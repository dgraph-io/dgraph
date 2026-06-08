/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
	"google.golang.org/protobuf/proto"
)

// TestReadManyDataMatchesReadData verifies that the batched MultiGet-backed
// read path (MemoryLayer.ReadManyData -> ReadPostingListFromVersions) folds a
// key's version chain identically to the per-key path
// (NewKeyIterator -> ReadPostingList) over a real on-disk round-trip.
func TestReadManyDataMatchesReadData(t *testing.T) {
	mkpl := func(uid uint64) []byte {
		pl := &pb.PostingList{
			Postings: []*pb.Posting{{Uid: uid, Op: uint32(Set)}},
		}
		b, err := proto.Marshal(pl)
		require.NoError(t, err)
		return b
	}

	// Build keys with a few different version-chain shapes, each under a unique
	// predicate so the process-global cache starts cold.
	pred := x.AttrInRootNamespace("multiget-" + uuid.New().String())
	var keys [][]byte
	var kvs []*bpb.KV
	for i := 0; i < 12; i++ {
		key := x.DataKey(pred, uint64(i+1))
		keys = append(keys, key)
		switch i % 4 {
		case 0:
			// complete posting only
			kvs = append(kvs, &bpb.KV{Key: key, Value: mkpl(uint64(i + 1)),
				UserMeta: []byte{BitCompletePosting}, Version: 2})
		case 1:
			// complete + newer delta on top
			kvs = append(kvs, &bpb.KV{Key: key, Value: mkpl(uint64(i + 1)),
				UserMeta: []byte{BitCompletePosting}, Version: 2})
			kvs = append(kvs, &bpb.KV{Key: key, Value: mkpl(uint64(100 + i)),
				UserMeta: []byte{BitDeltaPosting}, Version: 4})
		case 2:
			// empty posting
			kvs = append(kvs, &bpb.KV{Key: key, Value: []byte{},
				UserMeta: []byte{BitEmptyPosting}, Version: 3})
		case 3:
			// absent key (write nothing)
		}
	}
	require.NoError(t, writePostingListToDisk(kvs))

	readTs := uint64(10)

	// Ground truth: per-key path (bypasses caches, reads raw from disk).
	want := make([][]uint64, len(keys))
	for i, key := range keys {
		l, err := readPostingListFromDisk(key, ps, readTs)
		require.NoError(t, err)
		want[i] = listToArray(t, 0, l, readTs)
	}

	// Batched path.
	got, err := MemLayerInstance.ReadManyData(keys, ps, readTs)
	require.NoError(t, err)
	require.Equal(t, len(keys), len(got))
	for i, key := range keys {
		require.NotNil(t, got[i], "list for key %x", key)
		require.Equal(t, want[i], listToArray(t, 0, got[i], readTs),
			"uid set mismatch for key index %d", i)
	}

	// Also exercise the viLocalCache value path end-to-end: per-key Get vs
	// batched MultiGet must agree value-for-value.
	refVals := make([][]byte, len(keys))
	refErrs := make([]bool, len(keys))
	for i, key := range keys {
		vc := &viLocalCache{delegate: NewLocalCache(readTs)}
		v, e := vc.Get(key)
		refVals[i] = v
		refErrs[i] = e != nil
	}
	vc := &viLocalCache{delegate: NewLocalCache(readTs)}
	gotVals, gotErrs := vc.MultiGet(keys)
	for i := range keys {
		require.Equal(t, refErrs[i], gotErrs[i] != nil,
			"error presence mismatch for key index %d (%v)", i, gotErrs[i])
		require.Equal(t, fmt.Sprintf("%x", refVals[i]), fmt.Sprintf("%x", gotVals[i]),
			"value mismatch for key index %d", i)
	}
}

// benchFrontierKeys writes nKeys vector-shaped posting lists (one complete
// posting whose Value holds the vector bytes) under a unique predicate and
// returns their keys. Mirrors how dgraph stores vectors that HNSW reads.
func benchFrontierKeys(b *testing.B, nKeys, dim int) [][]byte {
	b.Helper()
	pred := x.AttrInRootNamespace("frontier-bench-" + uuid.New().String())
	vec := make([]byte, dim*4) // dim float32s
	for i := range vec {
		vec[i] = byte(i)
	}
	pl := &pb.PostingList{Postings: []*pb.Posting{{Uid: 1, Op: uint32(Set), Value: vec}}}
	val, err := proto.Marshal(pl)
	if err != nil {
		b.Fatal(err)
	}
	keys := make([][]byte, nKeys)
	kvs := make([]*bpb.KV, 0, nKeys)
	for i := 0; i < nKeys; i++ {
		key := x.DataKey(pred, uint64(i+1))
		keys[i] = key
		kvs = append(kvs, &bpb.KV{Key: key, Value: val, UserMeta: []byte{BitCompletePosting}, Version: 1})
	}
	if err := writePostingListToDisk(kvs); err != nil {
		b.Fatal(err)
	}
	return keys
}

// BenchmarkHNSWFrontierRead compares the HNSW neighbor-vector read path against
// a real (cache-disabled) badger store: K serial per-key Get calls (today) vs a
// single batched MultiGet (this change), over a fresh per-iteration cache so
// every read is cold — the search-time reality.
func BenchmarkHNSWFrontierRead(b *testing.B) {
	const dim = 384
	readTs := uint64(10)
	for _, K := range []int{16, 64, 256} {
		keys := benchFrontierKeys(b, K, dim)
		b.Run(fmt.Sprintf("Get/K=%d", K), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				vc := &viLocalCache{delegate: NewLocalCache(readTs)}
				for _, key := range keys {
					if _, err := vc.Get(key); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
		b.Run(fmt.Sprintf("MultiGet/K=%d", K), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				vc := &viLocalCache{delegate: NewLocalCache(readTs)}
				_, errs := vc.MultiGet(keys)
				for _, e := range errs {
					if e != nil {
						b.Fatal(e)
					}
				}
			}
		})
	}
}
