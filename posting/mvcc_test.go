/*
 * Copyright 2023 Dgraph Labs, Inc. and Contributors
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

package posting

import (
	"context"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/dgraph/v24/protos/pb"
	"github.com/dgraph-io/dgraph/v24/x"
	"github.com/dgraph-io/ristretto/v2/z"
)

func TestIncrRollupGetsCancelledQuickly(t *testing.T) {
	attr := x.GalaxyAttr("rollup")
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
	attr := x.GalaxyAttr("cache")
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
	//lCache, _ = ristretto.NewCache[[]byte, *List](&ristretto.Config[[]byte, *List]{
	//	// Use 5% of cache memory for storing counters.
	//	NumCounters: int64(1000 * (1 << 20) * 0.05 * 2),
	//	MaxCost:     int64(1000 * (1 << 20) * 0.95),
	//	BufferItems: 64,
	//	Metrics:     true,
	//	Cost: func(val *List) int64 {
	//		return 0
	//	},
	//})

	attr := x.GalaxyAttr("cache")
	keys := make([][]byte, 0)
	N := 10000
	txn := Oracle().RegisterStartTs(1)

	for i := 1; i < N; i++ {
		key := x.DataKey(attr, uint64(i))
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
	err := txn.CommitToDisk(writer, 2)
	if err != nil {
		panic(err)
	}
	writer.Flush()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := keys[rand.Intn(N-1)]
			_, err = getNew(key, pstore, math.MaxUint64)
			if err != nil {
				panic(err)
			}
		}
	})

}

func TestRollupTimestamp(t *testing.T) {
	attr := x.GalaxyAttr("rollup")
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
	addMutation(t, l, edge, Del, 9, 10, false)

	nl, err := getNew(key, pstore, math.MaxUint64)
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

func TestPostingListRead(t *testing.T) {
	attr := x.GalaxyAttr("emptypl")
	key := x.DataKey(attr, 1)

	assertLength := func(readTs, sz int) {
		nl, err := getNew(key, pstore, math.MaxUint64)
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
	memoryLayer.Del(z.MemHash(key))
	assertLength(7, 0)

	addEdgeToUID(t, attr, 1, 4, 7, 8)
	assertLength(9, 1)

	var empty pb.PostingList
	data, err := proto.Marshal(&empty)
	require.NoError(t, err)

	writer = NewTxnWriter(pstore)
	require.NoError(t, writer.SetAt(key, data, BitCompletePosting, 10))
	require.NoError(t, writer.Flush())
	memoryLayer.Del(z.MemHash(key))
	assertLength(10, 0)

	addEdgeToUID(t, attr, 1, 5, 11, 12)
	addEdgeToUID(t, attr, 1, 6, 13, 14)
	addEdgeToUID(t, attr, 1, 7, 15, 16)
	assertLength(17, 3)
}
