/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"context"
	"math/rand"
	"sync"
	"testing"

	"github.com/dgraph-io/dgraph/v25/x"
)

// Benchmarks for the fixed per-query BM25 overheads (campaign Q35).
//
// Every bm25() query unconditionally calls ReadBM25Stats (worker/task.go,
// handleBM25Search), which performs NumBM25StatsBuckets (32) posting-list
// reads regardless of query selectivity — a df=10 query pays the same floor
// as a df=1M one. The ReadBM25Stats benchmarks measure that floor against the
// package's badger-backed pstore three ways: straight store reads (no cache),
// a fresh LocalCache per call (what a real query does), and a warm LocalCache
// (repeat reads within one query).
//
// The codec round-trip benches pin the cost of the varint encodings that sit
// on every posting materialization (tf, docLen) and every stats read/write
// (docCount, totalTerms); decode of the posting value is the per-posting hot
// path inside ReadBM25TermPostings.
//
// The companion Q34 benchmark (scored HNSW search overhead) is deliberately
// deferred: it requires a populated HNSW index, which is not available at
// this package's unit level.

var bm25BenchOnce sync.Once

// bm25BenchStatsSetup commits one transaction that writes deterministic corpus
// statistics into every one of the 32 stats buckets for the benchmark
// predicate, mirroring the commit path of TestBM25StatsAccumulateAcrossTxns.
// It returns the namespaced attribute and a timestamp at which the committed
// stats are visible.
func bm25BenchStatsSetup(tb testing.TB) (attr string, readTs uint64) {
	attr = x.AttrInRootNamespace("bm25benchstats")
	const startTs, commitTs = 50001, 50002
	readTs = commitTs + 1
	bm25BenchOnce.Do(func() {
		ctx := context.Background()
		rng := rand.New(rand.NewSource(42)) // deterministic corpus
		txn := Oracle().RegisterStartTs(startTs)
		txn.cache = NewLocalCache(startTs)
		// 320 documents with uids 1..320: every bucket (uid%32) receives
		// exactly ten documents, so all 32 bucket keys hold a value posting.
		for uid := uint64(1); uid <= 320; uid++ {
			docLen := int64(rng.Intn(100) + 1)
			if err := txn.updateBM25Stats(ctx, attr, uid, 1, docLen); err != nil {
				tb.Fatal(err)
			}
		}
		txn.Update()
		txn.UpdateCachedKeys(commitTs)
		writer := NewTxnWriter(pstore)
		if err := txn.CommitToDisk(writer, commitTs); err != nil {
			tb.Fatal(err)
		}
		if err := writer.Flush(); err != nil {
			tb.Fatal(err)
		}
	})
	return attr, readTs
}

// checkBM25BenchStats guards against the benchmark timing an accidental no-op:
// the corpus is 320 docs, and totalTerms must be non-zero.
func checkBM25BenchStats(b *testing.B, docCount, totalTerms uint64) {
	b.Helper()
	if docCount != 320 || totalTerms == 0 {
		b.Fatalf("unexpected stats: docCount=%d totalTerms=%d", docCount, totalTerms)
	}
}

// BenchmarkReadBM25StatsStore measures the uncached stats floor: each op reads
// all 32 bucket posting lists straight from the badger-backed store.
func BenchmarkReadBM25StatsStore(b *testing.B) {
	attr, readTs := bm25BenchStatsSetup(b)
	get := func(k []byte) (*List, error) { return GetNoStore(k, readTs) }
	var dc, tt uint64
	var err error
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc, tt, err = ReadBM25Stats(get, attr, readTs)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	checkBM25BenchStats(b, dc, tt)
	b.ReportMetric(numBM25StatsBuckets, "buckets_read/op")
}

// BenchmarkReadBM25StatsFreshLocalCache is the production-faithful shape: a
// query starts with an empty LocalCache, so all 32 bucket reads miss and fall
// through to the store, plus the cache bookkeeping.
func BenchmarkReadBM25StatsFreshLocalCache(b *testing.B) {
	attr, readTs := bm25BenchStatsSetup(b)
	var dc, tt uint64
	var err error
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lc := NewLocalCache(readTs)
		dc, tt, err = ReadBM25Stats(lc.Get, attr, readTs)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	checkBM25BenchStats(b, dc, tt)
	b.ReportMetric(numBM25StatsBuckets, "buckets_read/op")
}

// BenchmarkReadBM25StatsWarmLocalCache measures the repeat-read floor when all
// 32 bucket lists are already materialized in the query's LocalCache.
func BenchmarkReadBM25StatsWarmLocalCache(b *testing.B) {
	attr, readTs := bm25BenchStatsSetup(b)
	lc := NewLocalCache(readTs)
	// Prime the cache so the timed loop measures only warm reads.
	if _, _, err := ReadBM25Stats(lc.Get, attr, readTs); err != nil {
		b.Fatal(err)
	}
	var dc, tt uint64
	var err error
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc, tt, err = ReadBM25Stats(lc.Get, attr, readTs)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	checkBM25BenchStats(b, dc, tt)
	b.ReportMetric(numBM25StatsBuckets, "buckets_read/op")
}

// BenchmarkBM25ValueRoundTrip measures the (tf, docLen) posting-value codec:
// one encode plus one decode per op, over varint widths from 1 to 5 bytes.
func BenchmarkBM25ValueRoundTrip(b *testing.B) {
	var sink uint32
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tf := uint32(i%127 + 1)       // 1-byte varint
		dl := uint32(i%(1<<20)) + 128 // 2..3-byte varint
		buf := encodeBM25Value(tf, dl)
		gtf, gdl, ok := decodeBM25Value(buf)
		if !ok || gtf != tf || gdl != dl {
			b.Fatalf("round trip mismatch: (%d,%d) -> (%d,%d,%v)", tf, dl, gtf, gdl, ok)
		}
		sink += gtf + gdl
	}
	if sink == 0 {
		b.Fatal("sink is zero; codec produced no data")
	}
}

// BenchmarkBM25ValueDecode isolates the decode half — the per-posting hot path
// of ReadBM25TermPostings, executed once per materialized posting.
func BenchmarkBM25ValueDecode(b *testing.B) {
	// Deterministic pre-encoded values covering the varint width range.
	rng := rand.New(rand.NewSource(42))
	const n = 1024
	encoded := make([][]byte, n)
	for i := range encoded {
		encoded[i] = encodeBM25Value(uint32(rng.Intn(1<<16)), uint32(rng.Intn(1<<24)))
	}
	var sink uint32
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tf, dl, ok := decodeBM25Value(encoded[i%n])
		if !ok {
			b.Fatal("decode failed on valid input")
		}
		sink += tf + dl
	}
	if sink == 0 {
		b.Fatal("sink is zero; decode produced no data")
	}
	b.ReportMetric(1, "postings_decoded/op")
}

// BenchmarkBM25StatsRoundTrip measures the (docCount, totalTerms) stats codec:
// one encode plus one decode per op — the payload of every bucket read/write.
func BenchmarkBM25StatsRoundTrip(b *testing.B) {
	var sink uint64
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc := uint64(i%1_000_000 + 1)
		tt := dc * 37 // plausible avgDL ~37
		buf := encodeBM25Stats(dc, tt)
		gdc, gtt := decodeBM25Stats(buf)
		if gdc != dc || gtt != tt {
			b.Fatalf("round trip mismatch: (%d,%d) -> (%d,%d)", dc, tt, gdc, gtt)
		}
		sink += gdc + gtt
	}
	if sink == 0 {
		b.Fatal("sink is zero; codec produced no data")
	}
}
