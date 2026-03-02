/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package testutil

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgo/v250"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

// KeyFile is set to the path of the file containing the key. Used for testing purposes only.
var KeyFile string

func openDgraph(pdir string) (*badger.DB, error) {
	// Get key.
	config := viper.New()
	flags := &pflag.FlagSet{}
	x.RegisterEncFlag(flags)
	if err := config.BindPFlags(flags); err != nil {
		return nil, err
	}
	config.Set("encryption", x.BuildEncFlag(KeyFile))
	keys, err := x.GetEncAclKeys(config)
	if err != nil {
		return nil, err
	}

	opt := badger.DefaultOptions(pdir).
		WithBlockCacheSize(10 * (1 << 20)).
		WithIndexCacheSize(10 * (1 << 20)).
		WithEncryptionKey(keys.EncKey).
		WithNamespaceOffset(x.NamespaceOffset)
	return badger.OpenManaged(opt)
}

func WaitForRestore(t *testing.T, dg *dgo.Dgraph, HttpSocket string) {
	// Use a 10-minute overall deadline so the test fails quickly if the
	// cluster is in a permanently degraded state (e.g. OOM-killed containers)
	// rather than hanging until the Go test timeout.
	deadline := time.Now().Add(10 * time.Minute)

	restoreDone := false
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + HttpSocket + "/health")
		if err != nil {
			// The health endpoint may be transiently unreachable while the
			// server restarts during a restore.  Keep retrying.
			t.Logf("WaitForRestore: health check error (will retry): %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		buf, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Logf("WaitForRestore: error reading health body (will retry): %v", err)
			time.Sleep(1 * time.Second)
			continue
		}
		sbuf := string(buf)
		if !strings.Contains(sbuf, "opRestore") {
			restoreDone = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.True(t, restoreDone, "timed out waiting for restore operation to complete")

	// Wait for the client to exit draining mode. This is needed because the client might
	// be connected to a follower and might be behind the leader in applying the restore.
	// Waiting for three consecutive successful queries is done to prevent a situation in
	// which the query succeeds at the first attempt because the follower is behind and
	// has not started to apply the restore proposal.
	numSuccess := 0
	for time.Now().Before(deadline) {
		// This is a dummy query that returns no results.
		_, err := dg.NewTxn().Query(context.Background(), `{
	   q(func: has(invalid_pred)) {
		   invalid_pred
	   }}`)

		if err == nil {
			numSuccess++
		} else {
			// During restore the server may be in draining mode, or it may be
			// transiently unreachable (connection reset, TLS handshake failure,
			// gRPC Unavailable, etc.).  All of these are expected and retriable.
			errMsg := err.Error()
			transient := strings.Contains(errMsg, "the server is in draining mode") ||
				strings.Contains(errMsg, "Unavailable") ||
				strings.Contains(errMsg, "connection reset") ||
				strings.Contains(errMsg, "connection refused") ||
				strings.Contains(errMsg, "transport") ||
				strings.Contains(errMsg, "EOF") ||
				strings.Contains(errMsg, "overloaded") ||
				strings.Contains(errMsg, "context canceled") ||
				strings.Contains(errMsg, "Please retry")
			if !transient {
				require.Fail(t, "unexpected error while waiting for restore",
					"error: %v", err)
			}
			numSuccess = 0
		}

		// Apply restore works differently with race enabled.
		// We are seeing delays in apply proposals hence failure of queries.
		if numSuccess == 10 {
			// The server has been responsive ten times in a row.
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.GreaterOrEqual(t, numSuccess, 10,
		"timed out waiting for server to exit draining mode after restore")
}

// GetPredicateValues reads the specified p directory and returns the values for the given
// attribute in a map.
func GetPredicateValues(pdir, attr string, readTs uint64) (map[string]string, error) {
	db, err := openDgraph(pdir)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	values := make(map[string]string)

	txn := db.NewTransactionAt(readTs, false)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		pk, err := x.Parse(item.Key())
		x.Check(err)
		switch {
		case pk.Attr != attr:
			continue
		case !pk.IsData():
			continue
		}

		pl, err := posting.ReadPostingList(item.Key(), itr)
		if err != nil {
			return nil, err
		}

		err = pl.Iterate(readTs, 0, func(p *pb.Posting) error {
			vID := types.TypeID(p.ValType)
			src := types.ValueForType(vID)
			src.Value = p.Value
			str, err := types.Convert(src, types.StringID)
			if err != nil {
				return err
			}
			value := str.Value.(string)
			values[fmt.Sprintf("%#x", pk.Uid)] = value

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return values, err
}

type dataType int

const (
	schemaPredicate dataType = iota
	schemaType
)

func readSchema(pdir string, dType dataType) ([]string, error) {
	db, err := openDgraph(pdir)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	values := make([]string, 0)

	txn := db.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	for itr.Rewind(); itr.Valid(); itr.Next() {
		item := itr.Item()
		pk, err := x.Parse(item.Key())
		x.Check(err)

		switch {
		case item.UserMeta() != posting.BitSchemaPosting:
			continue
		case pk.IsSchema() && dType != schemaPredicate:
			continue
		case pk.IsType() && dType != schemaType:
			continue
		}

		values = append(values, x.ParseAttr(pk.Attr))
	}
	return values, nil
}

// GetPredicateNames returns the list of all the predicates stored in the restored pdir.
func GetPredicateNames(pdir string) ([]string, error) {
	return readSchema(pdir, schemaPredicate)
}

// GetTypeNames returns the list of all the types stored in the restored pdir.
func GetTypeNames(pdir string) ([]string, error) {
	return readSchema(pdir, schemaType)
}

// CheckSchema checks the names of the predicates and types in the schema against the given names.
func CheckSchema(t *testing.T, preds, types []string) {
	pdirs := []string{
		"./data/restore/p1",
		"./data/restore/p2",
		"./data/restore/p3",
	}

	restoredPreds := make([]string, 0)
	for _, pdir := range pdirs {
		groupPreds, err := GetPredicateNames(pdir)
		require.NoError(t, err)
		restoredPreds = append(restoredPreds, groupPreds...)

		restoredTypes, err := GetTypeNames(pdir)
		require.NoError(t, err)
		require.ElementsMatch(t, types, restoredTypes)
	}
	require.ElementsMatch(t, preds, restoredPreds)
}
