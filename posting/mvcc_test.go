/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"context"
	"math"
	"math/rand"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/dgraph-io/ristretto/v2/z"
)

func TestIncrRollupGetsCancelledQuickly(t *testing.T) {
	attr := x.AttrInRootNamespace("rollup")
	key := x.DataKey(attr, 1)
	closer = z.NewCloser(1)

	writer := NewTxnWriter(pstore)

	incrRollup := &incrRollupi{
		getNewTs: func(b bool) uint64 {
			return 100
		},
		closer: closer,
	}

	finished := make(chan struct{})

	go func() {
		require.Error(t, incrRollup.rollUpKey(writer, key))
		finished <- struct{}{}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	closer.Signal()

	select {
	case <-ctx.Done():
		t.Fatalf("Cancelling rollup took more than 1 second")
	case <-finished:
	}
}

func TestCacheAfterDeltaUpdateRecieved(t *testing.T) {
	attr := x.AttrInRootNamespace("cache")
	key := x.IndexKey(attr, "temp")

	// Create a delta from 5->15. Mimick how a follower recieves a delta.
	p := new(pb.PostingList)
	p.Postings = []*pb.Posting{{
		Uid:      1,
		StartTs:  5,
		CommitTs: 15,
		Op:       1,
	}}
	delta, err := proto.Marshal(p)
	require.NoError(t, err)

	// Write delta to disk and call update
	txn := Oracle().RegisterStartTs(5)
	txn.cache.deltas[string(key)] = delta

	writer := NewTxnWriter(pstore)
	require.NoError(t, txn.CommitToDisk(writer, 15))
	require.NoError(t, writer.Flush())

	txn.UpdateCachedKeys(15)

	// Read key at timestamp 10. Make sure cache is not updated by this, as there is a later read.
	l, err := GetNoStore(key, 10)
	require.NoError(t, err)
	require.Equal(t, l.mutationMap.listLen(10), 0)

	// Read at 20 should show the value
	l1, err := GetNoStore(key, 20)
	require.NoError(t, err)
	require.Equal(t, l1.mutationMap.listLen(20), 1)
}

func BenchmarkTestCache(b *testing.B) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Panic(err)
	defer os.RemoveAll(dir)

	ps, err = badger.OpenManaged(badger.DefaultOptions(dir))
	x.Panic(err)
	Init(ps, 10000000, true)
	schema.Init(ps)

	attr := x.AttrInRootNamespace("cache")
	keys := make([][]byte, 0)
	N := uint64(10000)
	NInt := 10000
	txn := Oracle().RegisterStartTs(1)

	for i := uint64(1); i < N; i++ {
		key := x.DataKey(attr, i)
		keys = append(keys, key)
		edge := &pb.DirectedEdge{
			ValueId: 2,
			Attr:    attr,
			Entity:  1,
			Op:      pb.DirectedEdge_SET,
		}
		l, _ := GetNoStore(key, 1)
		// No index entries added here as we do not call AddMutationWithIndex.
		txn.cache.SetIfAbsent(string(l.key), l)
		err := l.addMutation(context.Background(), txn, edge)
		if err != nil {
			panic(err)
		}
	}
	txn.Update()
	writer := NewTxnWriter(pstore)
	err = txn.CommitToDisk(writer, 2)
	if err != nil {
		panic(err)
	}
	writer.Flush()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := keys[rand.Intn(NInt-1)]
			_, err = getNew(key, pstore, math.MaxUint64, false)
			if err != nil {
				panic(err)
			}
		}
	})
}

func TestRollupTimestamp(t *testing.T) {
	attr := x.AttrInRootNamespace("rollup")
	key := x.DataKey(attr, 1)
	// 3 Delta commits.
	addEdgeToUID(t, attr, 1, 2, 1, 2)
	addEdgeToUID(t, attr, 1, 3, 3, 4)
	addEdgeToUID(t, attr, 1, 4, 5, 6)

	l, err := GetNoStore(key, math.MaxUint64)
	require.NoError(t, err)

	uidList, err := l.Uids(ListOptions{ReadTs: 7})
	require.NoError(t, err)
	require.Equal(t, 3, len(uidList.Uids))

	edge := &pb.DirectedEdge{
		Entity: 1,
		Attr:   attr,

		Value: []byte(x.Star),
		Op:    pb.DirectedEdge_DEL,
	}
	l.mutationMap.setTs(9)
	addMutation(t, l, edge, Del, 9, 10, false)

	nl, err := getNew(key, pstore, math.MaxUint64, false)
	require.NoError(t, err)

	uidList, err = nl.Uids(ListOptions{ReadTs: 11})
	require.NoError(t, err)
	require.Equal(t, 0, len(uidList.Uids))

	// Now check that we don't lost the highest version during a rollup operation, despite the STAR
	// delete marker being the most recent update.
	kvs, err := nl.Rollup(nil, math.MaxUint64)
	require.NoError(t, err)
	require.Equal(t, uint64(10), kvs[0].Version)
}

// TestCacheStaleWhenMaxTsLessThanReadTs tests that readFromCache
// returns nil (cache miss) when cacheMaxTs < readTs, forcing a disk read.
// Issue #9597: Without the maxTs >= readTs check, stale cache data is returned.
func TestCacheStaleWhenMaxTsLessThanReadTs(t *testing.T) {
	require.NoError(t, pstore.DropAll())

	// Re-initialize MemoryLayer with cache enabled (10MB) for this test
	// The default test setup uses cache size 0 which disables caching
	origMemLayer := MemLayerInstance
	MemLayerInstance = initMemoryLayer(10<<20, false)
	t.Cleanup(func() {
		MemLayerInstance = origMemLayer
	})

	attr := x.AttrInRootNamespace("issue9597")
	key := x.IndexKey(attr, "test")

	// Step 1: Write UID 1 at commitTs=10 and populate cache via UpdateCachedKeys
	p1 := new(pb.PostingList)
	p1.Postings = []*pb.Posting{{
		Uid:      1,
		StartTs:  5,
		CommitTs: 10,
		Op:       1,
	}}
	delta1, err := proto.Marshal(p1)
	require.NoError(t, err)

	txn1 := Oracle().RegisterStartTs(5)
	txn1.cache.deltas[string(key)] = delta1

	writer1 := NewTxnWriter(pstore)
	require.NoError(t, txn1.CommitToDisk(writer1, 10))
	require.NoError(t, writer1.Flush())

	// Read at ts=10 to populate the cache (this triggers saveInCache in ReadData)
	l1, err := GetNoStore(key, 10)
	require.NoError(t, err)
	require.Equal(t, 1, l1.mutationMap.listLen(10))

	// Wait for ristretto to process the cache set
	MemLayerInstance.wait()

	// Verify cache is populated
	cacheItem, cacheOk := MemLayerInstance.cache.get(key)
	require.True(t, cacheOk, "Cache should have entry after first read")
	require.NotNil(t, cacheItem.list)
	cacheMaxTsBefore := cacheItem.list.maxTs

	// Step 2: Write UID 2 at commitTs=20 to disk, but DON'T call UpdateCachedKeys
	// This simulates the race where disk has newer data than cache
	p2 := new(pb.PostingList)
	p2.Postings = []*pb.Posting{{
		Uid:      2,
		StartTs:  15,
		CommitTs: 20,
		Op:       1,
	}}
	delta2, err := proto.Marshal(p2)
	require.NoError(t, err)

	txn2 := Oracle().RegisterStartTs(15)
	txn2.cache.deltas[string(key)] = delta2

	writer2 := NewTxnWriter(pstore)
	require.NoError(t, txn2.CommitToDisk(writer2, 20))
	require.NoError(t, writer2.Flush())
	// NOTE: We intentionally skip UpdateCachedKeys to keep cache stale

	// Verify cache is still stale (maxTs unchanged)
	cacheItem2, _ := MemLayerInstance.cache.get(key)
	require.Equal(t, cacheMaxTsBefore, cacheItem2.list.maxTs, "Cache should still be stale")

	// Step 3: Read at readTs=25 (greater than cache maxTs)
	// With the fix: cache miss (maxTs < readTs), reads from disk, gets both UIDs
	// Without fix: cache hit (minTs <= readTs), returns stale data without UID 2
	l2, err := GetNoStore(key, 25)
	require.NoError(t, err)

	uidList, err := l2.Uids(ListOptions{ReadTs: 25})
	require.NoError(t, err)

	// Should see UID 2 (from disk read)
	hasUid2 := slices.Contains(uidList.Uids, 2)
	if !hasUid2 {
		t.Fatalf("Expected UID 2 in result, got UIDs: %v", uidList.Uids)
	}
	require.True(t, hasUid2, "UID 2 missing - cache returned stale data (maxTs < readTs)")
}

func TestPostingListRead(t *testing.T) {
	attr := x.AttrInRootNamespace("emptypl")
	key := x.DataKey(attr, 1)

	assertLength := func(readTs, sz int) {
		nl, err := getNew(key, pstore, math.MaxUint64, false)
		require.NoError(t, err)
		uidList, err := nl.Uids(ListOptions{ReadTs: uint64(readTs)})
		require.NoError(t, err)
		require.Equal(t, sz, len(uidList.Uids))
	}

	addEdgeToUID(t, attr, 1, 2, 1, 2)
	addEdgeToUID(t, attr, 1, 3, 3, 4)

	writer := NewTxnWriter(pstore)
	require.NoError(t, writer.SetAt(key, []byte{}, BitEmptyPosting, 6))
	require.NoError(t, writer.Flush())
	// Delete the key from cache as we have just updated it
	MemLayerInstance.del(key)
	assertLength(7, 0)

	addEdgeToUID(t, attr, 1, 4, 7, 8)
	assertLength(9, 1)

	var empty pb.PostingList
	data, err := proto.Marshal(&empty)
	require.NoError(t, err)

	writer = NewTxnWriter(pstore)
	require.NoError(t, writer.SetAt(key, data, BitCompletePosting, 10))
	require.NoError(t, writer.Flush())
	MemLayerInstance.del(key)
	assertLength(10, 0)

	addEdgeToUID(t, attr, 1, 5, 11, 12)
	addEdgeToUID(t, attr, 1, 6, 13, 14)
	addEdgeToUID(t, attr, 1, 7, 15, 16)
	assertLength(17, 3)
}
