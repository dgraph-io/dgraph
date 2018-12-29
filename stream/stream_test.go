/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package stream

import (
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

func openManaged(dir string) (*badger.DB, error) {
	opt := badger.DefaultOptions
	opt.Dir = dir
	opt.ValueDir = dir

	return badger.OpenManaged(opt)
}

func value(k int) []byte {
	return []byte(fmt.Sprintf("%08d", k))
}

type collector struct {
	kv []*bpb.KV
}

func (c *collector) Send(kvs *pb.KVS) error {
	c.kv = append(c.kv, kvs.Kv...)
	return nil
}

func TestOrchestrate(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	db, err := openManaged(dir)
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

	c := &collector{kv: make([]*bpb.KV, 0, 100)}
	sl := Lists{Stream: c, DB: db}
	sl.ItemToKVFunc = func(key []byte, itr *badger.Iterator) (*bpb.KV, error) {
		item := itr.Item()
		val, err := item.ValueCopy(nil)
		require.NoError(t, err)
		kv := &bpb.KV{Key: item.KeyCopy(nil), Value: val, Version: item.Version()}
		itr.Next() // Just for fun.
		return kv, nil
	}

	// Test case 1. Retrieve everything.
	err = sl.Orchestrate(context.Background(), "Testing", math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 300, len(c.kv), "Expected 300. Got: %d", len(c.kv))

	m := make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		expected := value(int(pk.Uid))
		require.Equal(t, expected, kv.Value)
		m[pk.Attr]++
	}
	require.Equal(t, 3, len(m))
	for pred, count := range m {
		require.Equal(t, 100, count, "Count mismatch for pred: %s", pred)
	}

	// Test case 2. Retrieve only 1 predicate.
	sl.Predicate = "p1"
	c.kv = c.kv[:0]
	err = sl.Orchestrate(context.Background(), "Testing", math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 100, len(c.kv), "Expected 100. Got: %d", len(c.kv))

	m = make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		expected := value(int(pk.Uid))
		require.Equal(t, expected, kv.Value)
		m[pk.Attr]++
	}
	require.Equal(t, 1, len(m))
	for pred, count := range m {
		require.Equal(t, 100, count, "Count mismatch for pred: %s", pred)
	}

	// Test case 3. Retrieve select keys within the predicate.
	c.kv = c.kv[:0]
	sl.ChooseKeyFunc = func(item *badger.Item) bool {
		pk := x.Parse(item.Key())
		return pk.Uid%2 == 0
	}
	err = sl.Orchestrate(context.Background(), "Testing", math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 50, len(c.kv), "Expected 50. Got: %d", len(c.kv))

	m = make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		expected := value(int(pk.Uid))
		require.Equal(t, expected, kv.Value)
		m[pk.Attr]++
	}
	require.Equal(t, 1, len(m))
	for pred, count := range m {
		require.Equal(t, 50, count, "Count mismatch for pred: %s", pred)
	}

	// Test case 4. Retrieve select keys from all predicates.
	c.kv = c.kv[:0]
	sl.Predicate = ""
	err = sl.Orchestrate(context.Background(), "Testing", math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, 150, len(c.kv), "Expected 150. Got: %d", len(c.kv))

	m = make(map[string]int)
	for _, kv := range c.kv {
		pk := x.Parse(kv.Key)
		expected := value(int(pk.Uid))
		require.Equal(t, expected, kv.Value)
		m[pk.Attr]++
	}
	require.Equal(t, 3, len(m))
	for pred, count := range m {
		require.Equal(t, 50, count, "Count mismatch for pred: %s", pred)
	}
}
