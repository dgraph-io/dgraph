/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"fmt"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
)

// Benchmarks comparing the serial single-posting read path (GetSinglePosting per key —
// what handleValuePostings did before this change) against the batched path
// (GetBatchSinglePosting / NewBatchedSinglePostingIterator). Populated once, read-only
// thereafter, so ns/op differences are pure read-path cost.

const benchNumKeys = 10000

var benchKeysOnce sync.Once
var benchKeys [][]byte

// benchSetup commits benchNumKeys scalar value postings once per process, batching all
// writes through a single TxnWriter flush.
func benchSetup(b *testing.B) [][]byte {
	benchKeysOnce.Do(func() {
		attr := x.AttrInRootNamespace("benchBatchRead")
		benchKeys = make([][]byte, benchNumKeys)
		writer := NewTxnWriter(pstore)
		ts := uint64(3)
		for i := 0; i < benchNumKeys; i++ {
			benchKeys[i] = x.DataKey(attr, uint64(i+1))
			pl := &pb.PostingList{
				Postings: []*pb.Posting{{
					Uid: 1, Value: fmt.Appendf(nil, "value-%06d", i),
					Op: Set, StartTs: ts, CommitTs: ts + 1,
				}},
			}
			delta, err := proto.Marshal(pl)
			x.Panic(err)
			txn := Oracle().RegisterStartTs(ts)
			txn.cache.deltas[string(benchKeys[i])] = delta
			x.Panic(txn.CommitToDisk(writer, ts+1))
			ts += 2
		}
		x.Panic(writer.Flush())
	})
	return benchKeys
}

// BenchmarkSinglePostingRead compares fetching `batch` adjacent posting lists via the
// serial path vs one GetBatchSinglePosting call. ns/op is per group of `batch` keys.
func BenchmarkSinglePostingRead(b *testing.B) {
	keys := benchSetup(b)
	readTs := uint64(1 << 30)

	for _, batch := range []int{5, 10, 32, 64} {
		window := len(keys) - batch
		b.Run(fmt.Sprintf("serial/batch=%d", batch), func(b *testing.B) {
			lc := NewLocalCache(readTs)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				off := (i * batch) % window
				for j := 0; j < batch; j++ {
					if _, err := lc.GetSinglePosting(keys[off+j]); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
		b.Run(fmt.Sprintf("batched/batch=%d", batch), func(b *testing.B) {
			lc := NewLocalCache(readTs)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				off := (i * batch) % window
				if _, err := lc.GetBatchSinglePosting(keys[off : off+batch]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkValuePostingScan is the headline number: reading benchNumKeys posting lists
// the way a query goroutine does — serial GetSinglePosting per uid (old path) vs the
// chunked iterator (new path, chunks of batchSinglePostingSize). ns/op is per full scan
// of benchNumKeys keys.
func BenchmarkValuePostingScan(b *testing.B) {
	keys := benchSetup(b)
	readTs := uint64(1 << 30)

	b.Run("serial", func(b *testing.B) {
		lc := NewLocalCache(readTs)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, key := range keys {
				if _, err := lc.GetSinglePosting(key); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("batched", func(b *testing.B) {
		lc := NewLocalCache(readTs)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			next := lc.NewBatchedSinglePostingIterator(len(keys), func(j int) []byte {
				return keys[j]
			})
			for range keys {
				if _, err := next(); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}
