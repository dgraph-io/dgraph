/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

// Benchmarks comparing the legacy serial mutation path (runMutation per edge)
// with the new per-predicate mutation pipeline (newRunMutations).
//
// What the pipeline ought to win on:
//   - many predicates per transaction      → one goroutine per predicate
//   - many indexed edges per predicate     → 10-way intra-predicate
//                                            parallelism on tokenization
//
// What it shouldn't help (and may regret):
//   - tiny mutations (1-2 edges, 1 predicate) where goroutine spin-up cost
//     dominates the mutation work
//
// Each iteration is a single transaction: build a fresh batch of edges,
// run mutations, txn.Update(), CommitToDisk. We do NOT include the b.ResetTimer()
// before edge construction because edge construction is part of the
// per-transaction cost the pipeline is supposed to amortize.

func benchSetup(b *testing.B, schemaTxt string) *badger.DB {
	b.Helper()
	dir, err := os.MkdirTemp("", "pipeline_bench_")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = os.RemoveAll(dir) })

	ps, err := badger.OpenManaged(badger.DefaultOptions(dir).WithLoggingLevel(badger.ERROR))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = ps.Close() })

	posting.Init(ps, 0, false)
	Init(ps)
	posting.Oracle().ResetTxns()
	if err := schema.ParseBytes([]byte(schemaTxt), 1); err != nil {
		b.Fatal(err)
	}
	return ps
}

// buildEdges constructs numPreds*edgesPerPred edges across distinct predicates,
// indexed-string-valued. The same generator drives both legacy and pipeline
// runs so the input is identical.
func buildEdges(numPreds, edgesPerPred int, baseUid uint64) []*pb.DirectedEdge {
	edges := make([]*pb.DirectedEdge, 0, numPreds*edgesPerPred)
	for p := 0; p < numPreds; p++ {
		attr := x.AttrInRootNamespace(fmt.Sprintf("p%d", p))
		for e := 0; e < edgesPerPred; e++ {
			edges = append(edges, &pb.DirectedEdge{
				Entity:    baseUid + uint64(e),
				Attr:      attr,
				Value:     []byte(fmt.Sprintf("v%d_%d", p, e)),
				ValueType: pb.Posting_STRING,
				Op:        pb.DirectedEdge_SET,
			})
		}
	}
	return edges
}

// schemaForPreds emits "p0: string @index(exact) ., p1: ..., ..." (or no
// index, depending on indexed). Each predicate is a distinct list-or-scalar.
func schemaForPreds(numPreds int, indexed bool, list bool) string {
	var b []byte
	for p := 0; p < numPreds; p++ {
		ty := "string"
		if list {
			ty = "[string]"
		}
		idx := ""
		if indexed {
			idx = " @index(exact)"
		}
		b = append(b, []byte(fmt.Sprintf("p%d: %s%s .\n", p, ty, idx))...)
	}
	return string(b)
}

// runOne executes one transaction's mutations through the chosen path.
// startTs/commitTs must be unique per call.
func runOnePipeline(b *testing.B, ps *badger.DB, edges []*pb.DirectedEdge, startTs, commitTs uint64) {
	b.Helper()
	txn := posting.Oracle().RegisterStartTs(startTs)
	if err := newRunMutations(context.Background(), edges, txn); err != nil {
		b.Fatal(err)
	}
	txn.Update()
	w := posting.NewTxnWriter(ps)
	if err := txn.CommitToDisk(w, commitTs); err != nil {
		b.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		b.Fatal(err)
	}
	txn.UpdateCachedKeys(commitTs)
}

func runOneLegacy(b *testing.B, ps *badger.DB, edges []*pb.DirectedEdge, startTs, commitTs uint64) {
	b.Helper()
	txn := posting.Oracle().RegisterStartTs(startTs)
	for _, e := range edges {
		if err := runMutation(context.Background(), e, txn); err != nil {
			b.Fatal(err)
		}
	}
	txn.Update()
	w := posting.NewTxnWriter(ps)
	if err := txn.CommitToDisk(w, commitTs); err != nil {
		b.Fatal(err)
	}
	if err := w.Flush(); err != nil {
		b.Fatal(err)
	}
	txn.UpdateCachedKeys(commitTs)
}

// runBench runs sub-benchmarks (legacy vs pipeline) for a single
// (numPreds, edgesPerPred, indexed, list) configuration.
func runBench(b *testing.B, numPreds, edgesPerPred int, indexed, list bool) {
	for _, mode := range []struct {
		name string
		fn   func(*testing.B, *badger.DB, []*pb.DirectedEdge, uint64, uint64)
	}{
		{"legacy", runOneLegacy},
		{"pipeline", runOnePipeline},
	} {
		b.Run(mode.name, func(b *testing.B) {
			ps := benchSetup(b, schemaForPreds(numPreds, indexed, list))
			b.ReportAllocs()
			b.ResetTimer()
			ts := uint64(10)
			for i := 0; i < b.N; i++ {
				edges := buildEdges(numPreds, edgesPerPred, uint64(i)*1_000_000+1)
				mode.fn(b, ps, edges, ts, ts+1)
				ts += 2
			}
		})
	}
}

// 1 predicate, 1 edge — smallest possible mutation. Pipeline overhead
// is most visible here.
func BenchmarkMutate_1pred_1edge_indexed(b *testing.B) {
	runBench(b, 1, 1, true, false)
}

// 1 predicate, 100 indexed edges — exercises intra-predicate
// tokenization parallelism.
func BenchmarkMutate_1pred_100edges_indexed(b *testing.B) {
	runBench(b, 1, 100, true, false)
}

// 10 predicates, 1 edge each — per-predicate parallelism with light work
// per predicate.
func BenchmarkMutate_10preds_1edge_indexed(b *testing.B) {
	runBench(b, 10, 1, true, false)
}

// 10 predicates, 100 edges each — full benefit case: per-predicate AND
// intra-predicate parallelism on indexed work.
func BenchmarkMutate_10preds_100edges_indexed(b *testing.B) {
	runBench(b, 10, 100, true, false)
}

// 1 predicate, 1000 indexed edges — heavy intra-predicate.
func BenchmarkMutate_1pred_1000edges_indexed(b *testing.B) {
	runBench(b, 1, 1000, true, false)
}

// 10 predicates, 1000 edges each — large mutation, indexed.
func BenchmarkMutate_10preds_1000edges_indexed(b *testing.B) {
	runBench(b, 10, 1000, true, false)
}

// Non-indexed counterparts isolate per-predicate parallelism from the
// tokenization parallelism.
func BenchmarkMutate_10preds_1000edges_noindex(b *testing.B) {
	runBench(b, 10, 1000, false, false)
}

// Very large indexed mutation: 50 predicates × 1000 edges each = 50k edges.
// Where the pipeline should shine most.
func BenchmarkMutate_50preds_1000edges_indexed(b *testing.B) {
	runBench(b, 50, 1000, true, false)
}

// 50 predicates, 100 edges each (5k edges) — typical-ish bulk write shape.
func BenchmarkMutate_50preds_100edges_indexed(b *testing.B) {
	runBench(b, 50, 100, true, false)
}
