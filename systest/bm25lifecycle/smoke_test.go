//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// TestBM25LifecycleSmoke proves the LocalCluster harness works for this package:
// single group, bm25 index, one query round-trip.
func TestBM25LifecycleSmoke(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().WithNumAlphas(1).WithNumZeros(1).WithReplicas(1)
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	dg, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	require.NoError(t, dg.DropAll())
	require.NoError(t, dg.SetupSchema(`smoke_text: string @index(bm25) .`))

	_, err = dg.Mutate(&api.Mutation{
		SetNquads: []byte(`
			_:a <smoke_text> "zeppelin zeppelin" .
			_:b <smoke_text> "zeppelin marzipan" .
		`),
		CommitNow: true,
	})
	require.NoError(t, err)

	resp, err := dg.Query(`{ q(func: bm25(smoke_text, "zeppelin")) { uid } }`)
	require.NoError(t, err)
	require.Contains(t, string(resp.GetJson()), "uid")
}
