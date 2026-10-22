//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
	"github.com/dgraph-io/dgraph/v25/x"

	"github.com/stretchr/testify/require"
)

const (
	placementSchema = `
		name: string @index(exact) .
		payload: string .
		friend: [uid] .
		dataless: string .
	`
	placementData = `
		_:a <name> "alice" .
		_:b <name> "bob" .
		_:a <payload> "blob-a" .
		_:b <payload> "blob-b" .
		_:a <friend> _:b .
		_:a <dgraph.type> "Person" .
	`
	// payload and friend carry data; dataless appears only in the schema and exercises
	// schema-key routing for pinned predicates without data.
	placementJson = `[
		{"predicate": "payload", "group": 2},
		{"predicate": "friend", "group": 3},
		{"predicate": "dataless", "group": 2}
	]`
)

// predicatesInShard opens the bulk output p directory read-only and returns the set of
// namespaced predicates having data keys and schema keys in it.
func predicatesInShard(t *testing.T, pdir string) (data, schema map[string]struct{}) {
	opts := badger.DefaultOptions(pdir).WithReadOnly(true)
	db, err := badger.OpenManaged(opts)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	data, schema = make(map[string]struct{}), make(map[string]struct{})
	txn := db.NewTransactionAt(^uint64(0), false)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()
	for itr.Rewind(); itr.Valid(); itr.Next() {
		pk, err := x.Parse(itr.Item().Key())
		if err != nil {
			continue // internal badger keys
		}
		switch {
		case pk.IsData():
			data[pk.Attr] = struct{}{}
		case pk.IsSchema():
			schema[pk.Attr] = struct{}{}
		}
	}
	return data, schema
}

func TestBulkLoaderTabletPlacement(t *testing.T) {
	bulkOutDir := t.TempDir()
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(3).WithNumZeros(1).
		WithReplicas(1).WithBulkLoadOutDir(bulkOutDir)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()

	require.NoError(t, c.StartZero(0))
	require.NoError(t, c.HealthCheck(true))

	baseDir := t.TempDir()
	schemaFile := filepath.Join(baseDir, "schema.txt")
	require.NoError(t, os.WriteFile(schemaFile, []byte(placementSchema), os.ModePerm))
	dataFile := filepath.Join(baseDir, "data.rdf")
	require.NoError(t, os.WriteFile(dataFile, []byte(placementData), os.ModePerm))
	placementFile := filepath.Join(baseDir, "placement.json")
	require.NoError(t, os.WriteFile(placementFile, []byte(placementJson), os.ModePerm))

	// Negative leg: a placement entry whose group has no reduce shard must fail the run
	// during startup validation, before any data is touched.
	badPlacementFile := filepath.Join(baseDir, "bad_placement.json")
	require.NoError(t, os.WriteFile(badPlacementFile,
		[]byte(`[{"predicate": "payload", "group": 5}]`), os.ModePerm))
	err = c.BulkLoad(dgraphtest.BulkOpts{
		DataFiles:           []string{dataFile},
		SchemaFiles:         []string{schemaFile},
		MapShards:           6,
		ReduceShards:        3,
		TabletPlacementFile: badPlacementFile,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reduce_shards(3)")
	require.Contains(t, err.Error(), "payload -> group 5")

	require.NoError(t, c.BulkLoad(dgraphtest.BulkOpts{
		DataFiles:           []string{dataFile},
		SchemaFiles:         []string{schemaFile},
		MapShards:           6,
		ReduceShards:        3,
		TabletPlacementFile: placementFile,
	}))

	// Offline leg: the out directory is on the host, so placement is verifiable straight
	// from the badger stores, before any alpha starts.
	pins := map[string]int{"0-payload": 1, "0-friend": 2, "0-dataless": 1}
	for shard := range 3 {
		pdir := filepath.Join(bulkOutDir, fmt.Sprintf("%d", shard), "p")

		groupId, err := x.ReadGroupIdFile(pdir)
		require.NoError(t, err)
		require.Equal(t, uint32(shard+1), groupId)

		data, schema := predicatesInShard(t, pdir)
		for pred, pinnedShard := range pins {
			if shard == pinnedShard {
				require.Contains(t, schema, pred,
					"schema key of pinned %s missing from its shard %d", pred, shard)
				if pred != "0-dataless" {
					require.Contains(t, data, pred,
						"data keys of pinned %s missing from its shard %d", pred, shard)
				}
			} else {
				require.NotContains(t, data, pred,
					"data keys of pinned %s leaked into shard %d", pred, shard)
				require.NotContains(t, schema, pred,
					"schema key of pinned %s leaked into shard %d", pred, shard)
			}
		}
		// Reserved predicates with data stay on group 1 regardless of placement. (Schema
		// keys of DATALESS reserved predicates are hash-distributed across groups by
		// writeSchema — pre-existing behavior, deliberately not asserted here.)
		dgraphType := x.NamespaceAttr(x.RootNamespace, "dgraph.type")
		if shard == 0 {
			require.Contains(t, data, dgraphType)
			require.Contains(t, schema, dgraphType)
		} else {
			require.NotContains(t, data, dgraphType)
		}
	}

	// Cluster leg: boot the alphas and confirm Zero serves each pinned tablet from its
	// pinned group, then query through the cluster.
	require.NoError(t, c.Start())

	require.Eventually(t, func() bool {
		state, err := c.GetZeroState(0)
		if err != nil {
			return false
		}
		for pred, pinnedShard := range pins {
			group, ok := state.TabletGroup(pred)
			if !ok || group != uint32(pinnedShard+1) {
				return false
			}
		}
		return true
	}, 2*time.Minute, 2*time.Second, "pinned tablets not served by their pinned groups")

	dg, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	resp, err := dg.NewReadOnlyTxn().Query(ctx, `{
		payloads(func: has(payload)) { count(uid) }
		friends(func: has(friend)) { count(uid) }
	}`)
	require.NoError(t, err)
	require.JSONEq(t,
		`{"payloads": [{"count": 2}], "friends": [{"count": 1}]}`,
		string(resp.Json))
}
