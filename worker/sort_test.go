/*
 * Copyright 2017-2023 Dgraph Labs, Inc. and Contributors
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

package worker

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/dgraph-io/badger/v4"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgraph/v24/posting"
	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/schema"
	"github.com/dgraph-io/dgraph/v24/x"
	"github.com/stretchr/testify/require"
)

func readPostingListFromDisk(key []byte, pstore *badger.DB, readTs uint64) (*posting.List, error) {
	txn := pstore.NewTransactionAt(readTs, false)
	defer txn.Discard()

	// When we do rollups, an older version would go to the top of the LSM tree, which can cause
	// issues during txn.Get. Therefore, always iterate.
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	iterOpts.PrefetchValues = false
	itr := txn.NewKeyIterator(key, iterOpts)
	defer itr.Close()
	itr.Seek(key)
	return posting.ReadPostingList(key, itr)
}

func rollup(t *testing.T, key []byte, pstore *badger.DB, readTs uint64) {
	ol, err := readPostingListFromDisk(key, pstore, readTs)
	require.NoError(t, err)
	kvs, err := ol.Rollup(nil, readTs+1)
	require.NoError(t, err)
	require.NoError(t, writePostingListToDisk(kvs))
}

func writePostingListToDisk(kvs []*bpb.KV) error {
	writer := posting.NewTxnWriter(pstore)
	for _, kv := range kvs {
		if err := writer.SetAt(kv.Key, kv.Value, kv.UserMeta[0], kv.Version); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func TestSingleUid(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	posting.Init(ps, 0)
	Init(ps)
	err = schema.ParseBytes([]byte("singleUidTest: string @index(exact) @unique ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.GalaxyAttr("singleUidTest")

	// Txn 1. Set 1 -> david 2 -> blush
	x.Check(runMutation(ctx, &pb.DirectedEdge{
		Value:  []byte("david"),
		Attr:   attr,
		Entity: 1,
		Op:     pb.DirectedEdge_SET,
	}, txn))

	x.Check(runMutation(ctx, &pb.DirectedEdge{
		Value:  []byte("blush"),
		Attr:   attr,
		Entity: 2,
		Op:     pb.DirectedEdge_SET,
	}, txn))

	txn.Update()
	writer := posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 7))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(7)

	// Txn 2. Set 2 -> david 1 -> blush
	txn = posting.Oracle().RegisterStartTs(9)

	x.Check(runMutation(ctx, &pb.DirectedEdge{
		Value:  []byte("david"),
		Attr:   attr,
		Entity: 2,
		Op:     pb.DirectedEdge_SET,
	}, txn))

	x.Check(runMutation(ctx, &pb.DirectedEdge{
		Value:  []byte("blush"),
		Attr:   attr,
		Entity: 1,
		Op:     pb.DirectedEdge_SET,
	}, txn))

	txn.Update()
	writer = posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 11))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(11)

	key := x.IndexKey(attr, string([]byte{2, 100, 97, 118, 105, 100}))

	// Reading the david index, we should see 2 inserted, 1 deleted
	txn = posting.Oracle().RegisterStartTs(15)
	l, err := txn.Get(key)
	require.NoError(t, err)

	found, mpost, err := l.FindPosting(15, 2)
	require.NoError(t, err)
	require.Equal(t, found, true)
	require.Equal(t, mpost.Op, uint32(0x1))

	found, _, err = l.FindPosting(15, 1)
	require.NoError(t, err)
	require.Equal(t, found, false)

	rollup(t, key, pstore, 16)

	txn = posting.Oracle().RegisterStartTs(18)
	l, err = txn.Get(key)
	require.NoError(t, err)

	found, mpost, err = l.FindPosting(18, 2)
	require.NoError(t, err)
	require.Equal(t, found, true)
	require.Equal(t, mpost.Op, uint32(0x0))

	found, _, err = l.FindPosting(18, 1)
	require.NoError(t, err)
	require.Equal(t, found, false)
}

func TestLangExact(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0)
	Init(ps)
	err = schema.ParseBytes([]byte("testLang: string @index(term) @lang ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.GalaxyAttr("testLang")

	edge := &pb.DirectedEdge{
		Value:  []byte("english"),
		Attr:   attr,
		Entity: 1,
		Op:     pb.DirectedEdge_SET,
		Lang:   "en",
	}

	x.Check(runMutation(ctx, edge, txn))

	edge = &pb.DirectedEdge{
		Value:  []byte("hindi"),
		Attr:   attr,
		Entity: 1,
		Op:     pb.DirectedEdge_SET,
		Lang:   "hi",
	}

	x.Check(runMutation(ctx, edge, txn))

	txn.Update()
	writer := posting.NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 2))
	require.NoError(t, writer.Flush())
	txn.UpdateCachedKeys(2)

	key := x.DataKey(attr, 1)
	rollup(t, key, pstore, 4)

	pl, err := readPostingListFromDisk(key, pstore, 6)
	require.NoError(t, err)

	val, err := pl.ValueForTag(6, "hi")
	require.NoError(t, err)
	require.Equal(t, val.Value, []byte("hindi"))
}

func BenchmarkAddMutationWithIndex(b *testing.B) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0)
	Init(ps)
	err = schema.ParseBytes([]byte("benchmarkadd: string @index(term) ."), 1)
	fmt.Println(err)
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.GalaxyAttr("benchmarkadd")

	for i := 0; i < b.N; i++ {
		edge := &pb.DirectedEdge{
			Value:  []byte("david"),
			Attr:   attr,
			Entity: 1,
			Op:     pb.DirectedEdge_SET,
		}

		x.Check(runMutation(ctx, edge, txn))
	}
}

func TestRemoveDuplicates(t *testing.T) {
	toSet := func(uids []uint64) map[uint64]struct{} {
		m := make(map[uint64]struct{})
		for _, uid := range uids {
			m[uid] = struct{}{}
		}
		return m
	}

	for _, test := range []struct {
		setIn   []uint64
		setOut  []uint64
		uidsIn  []uint64
		uidsOut []uint64
	}{
		{setIn: nil, setOut: nil, uidsIn: nil, uidsOut: nil},
		{setIn: nil, setOut: []uint64{2}, uidsIn: []uint64{2}, uidsOut: []uint64{2}},
		{setIn: []uint64{2}, setOut: []uint64{2}, uidsIn: []uint64{2}, uidsOut: []uint64{}},
		{setIn: []uint64{2}, setOut: []uint64{2}, uidsIn: []uint64{2, 2}, uidsOut: []uint64{}},
		{
			setIn:   []uint64{2, 3},
			setOut:  []uint64{2, 3, 4, 5},
			uidsIn:  []uint64{3, 4, 5},
			uidsOut: []uint64{4, 5},
		},
	} {
		set := toSet(test.setIn)
		uidsOut := removeDuplicates(test.uidsIn, set)
		require.Equal(t, uidsOut, test.uidsOut)
		require.Equal(t, set, toSet(test.setOut))
	}
}
