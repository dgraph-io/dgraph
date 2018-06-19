/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package worker

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func openBadger(dir string) (*badger.ManagedDB, error) {
	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir

	return badger.OpenManaged(opt)
}

func value(k int) []byte {
	return []byte(fmt.Sprintf("%08d", k))
}

type collector struct {
	kv []*intern.KV
}

func (c *collector) Send(kvs *intern.KVS) error {
	c.kv = append(c.kv, kvs.Kv...)
	return nil
}

func TestOrchestrate(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := openBadger(dir)
	require.NoError(t, err)

	var count int
	for _, pred := range []string{"p0", "p1", "p2"} {
		txn := db.NewTransactionAt(math.MaxUint64, true)
		for i := 1; i <= 100; i++ {
			key := x.DataKey(pred, uint64(i))
			require.NoError(t, txn.Set(key, value(i)))
			count++
		}
		require.NoError(t, txn.CommitAt(5, nil))
	}

	c := &collector{kv: make([]*intern.KV, 0, 100)}
	sl := streamLists{stream: c, db: db}
	sl.itemToKv = func(key []byte, itr *badger.Iterator) (*intern.KV, error) {
		item := itr.Item()
		val, err := item.ValueCopy(nil)
		require.NoError(t, err)
		kv := &intern.KV{Key: item.KeyCopy(nil), Val: val, Version: item.Version()}
		itr.Next() // Just for fun.
		return kv, nil
	}

	// Test case 1. Retrieve everything.
	err = sl.orchestrate(context.Background(), "Testing", math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 300, len(c.kv), "Expected 300. Got: %d", len(c.kv))

	m := make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		expected := value(int(pk.Uid))
		require.Equal(t, expected, kv.Val)
		m[pk.Attr]++
	}
	require.Equal(t, 3, len(m))
	for pred, count := range m {
		require.Equal(t, 100, count, "Count mismatch for pred: %s", pred)
	}

	// Test case 2. Retrieve only 1 predicate.
	sl.predicate = "p1"
	c.kv = c.kv[:0]
	err = sl.orchestrate(context.Background(), "Testing", math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 100, len(c.kv), "Expected 100. Got: %d", len(c.kv))

	m = make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		expected := value(int(pk.Uid))
		require.Equal(t, expected, kv.Val)
		m[pk.Attr]++
	}
	require.Equal(t, 1, len(m))
	for pred, count := range m {
		require.Equal(t, 100, count, "Count mismatch for pred: %s", pred)
	}

	// Test case 3. Retrieve select keys within the predicate.
	c.kv = c.kv[:0]
	sl.chooseKey = func(key []byte, version uint64) bool {
		pk := x.Parse(key)
		return pk.Uid%2 == 0
	}
	err = sl.orchestrate(context.Background(), "Testing", math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 50, len(c.kv), "Expected 50. Got: %d", len(c.kv))

	m = make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		expected := value(int(pk.Uid))
		require.Equal(t, expected, kv.Val)
		m[pk.Attr]++
	}
	require.Equal(t, 1, len(m))
	for pred, count := range m {
		require.Equal(t, 50, count, "Count mismatch for pred: %s", pred)
	}

	// Test case 4. Retrieve select keys from all predicates.
	c.kv = c.kv[:0]
	sl.predicate = ""
	err = sl.orchestrate(context.Background(), "Testing", math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 150, len(c.kv), "Expected 150. Got: %d", len(c.kv))

	m = make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		expected := value(int(pk.Uid))
		require.Equal(t, expected, kv.Val)
		m[pk.Attr]++
	}
	require.Equal(t, 3, len(m))
	for pred, count := range m {
		require.Equal(t, 50, count, "Count mismatch for pred: %s", pred)
	}
}
