/*
 * Reverse-heavy benchmark: measures whether the auto/budget hybrid speedup
 * reaches @reverse [uid] predicates (it routes to ProcessList, which is serial).
 * Self-contained; package-internal (white-box).
 */

package posting

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

// buildReverseEdges builds `n` forward [uid] edges from distinct sources to a
// small set of `hotTargets` target uids — modeling the "many sources, 2-5 hot
// target nodes" reverse fan-in of the production workload.
func buildReverseEdges(attr string, n, hotTargets int) []*pb.DirectedEdge {
	const srcBase = uint64(1_000_000)
	const tgtBase = uint64(9_000_000)
	edges := make([]*pb.DirectedEdge, 0, n)
	for i := 0; i < n; i++ {
		edges = append(edges, &pb.DirectedEdge{
			Entity:  srcBase + uint64(i),
			Attr:    attr,
			ValueId: tgtBase + uint64(i%hotTargets), // few hot targets
			Op:      pb.DirectedEdge_SET,
		})
	}
	return edges
}

// buildScalarIndexedEdges builds `n` scalar string edges (distinct entities,
// `distinct` distinct values) — these route to ProcessSingle and DO benefit
// from the intra-predicate split.
func buildScalarIndexedEdges(attr string, n, distinct int) []*pb.DirectedEdge {
	const base = uint64(2_000_000)
	edges := make([]*pb.DirectedEdge, 0, n)
	for i := 0; i < n; i++ {
		edges = append(edges, &pb.DirectedEdge{
			Entity:    base + uint64(i),
			Attr:      attr,
			Value:     []byte(fmt.Sprintf("v%d", i%distinct)),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		})
	}
	return edges
}

func runBudget(b *testing.B, edges []*pb.DirectedEdge, budget int, fraction float64, minEdges int) {
	ob := x.WorkerConfig.MutationsPipelineGoroutines
	of := x.WorkerConfig.MutationsPipelineGoroutinesFraction
	om := x.WorkerConfig.MutationsPipelineMinEdgesPerWorker
	x.WorkerConfig.MutationsPipelineGoroutines = budget
	x.WorkerConfig.MutationsPipelineGoroutinesFraction = fraction
	x.WorkerConfig.MutationsPipelineMinEdgesPerWorker = minEdges
	defer func() {
		x.WorkerConfig.MutationsPipelineGoroutines = ob
		x.WorkerConfig.MutationsPipelineGoroutinesFraction = of
		x.WorkerConfig.MutationsPipelineMinEdgesPerWorker = om
	}()
	var ts uint64 = 300_000
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts += 10
		txn := NewTxn(ts)
		mp := NewMutationPipeline(txn)
		if err := mp.Process(context.Background(), cloneEdges(edges)); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(edges))*float64(b.N)/b.Elapsed().Seconds(), "edges/s")
}

// BenchmarkReverseDominant: ~20k edges, a SINGLE dominant `[uid] @reverse`
// predicate (5 hot targets). This is the "is the speedup OBE for @reverse"
// case — expect budget=auto ~= budget=0 (ProcessList + ProcessReverse serial).
func BenchmarkReverseDominant(b *testing.B) {
	require.NoError(b, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(b, schema.ParseBytes([]byte(`link: [uid] @reverse .`), 1))
	link := x.AttrInRootNamespace("link")
	edges := buildReverseEdges(link, 19980, 5)

	for _, bud := range []int{0, 8, 32} {
		bud := bud
		b.Run(fmt.Sprintf("budget=%d", bud), func(b *testing.B) { runBudget(b, edges, bud, 1.0, 256) })
	}
	b.Run("budget=auto", func(b *testing.B) { runBudget(b, edges, mutationsPipelineGoroutinesAuto, 1.0, 256) })
}

// BenchmarkReverseHighCardinality: ~20k edges on a single `[uid] @reverse`
// predicate spread over 8000 DISTINCT target uids (~2.5 sources each). This is
// the fan-in shape the parallel reverse write targets: the reverse map holds
// thousands of independent <~pred,targetUid> keys, so partitioning it across
// workers has real work to split. Contrast with BenchmarkReverseDominant, whose
// 5 hot targets cap parallelism at 5 and leave the serial path competitive.
func BenchmarkReverseHighCardinality(b *testing.B) {
	require.NoError(b, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(b, schema.ParseBytes([]byte(`link: [uid] @reverse .`), 1))
	link := x.AttrInRootNamespace("link")
	edges := buildReverseEdges(link, 19980, 8000)

	for _, bud := range []int{0, 8, 32} {
		bud := bud
		b.Run(fmt.Sprintf("budget=%d", bud), func(b *testing.B) { runBudget(b, edges, bud, 1.0, 256) })
	}
	b.Run("budget=auto", func(b *testing.B) { runBudget(b, edges, mutationsPipelineGoroutinesAuto, 1.0, 256) })
}

// BenchmarkReverseMultiPredicate: ~20k edges spread over N CONCURRENT
// high-cardinality `[uid] @reverse` predicates. Each predicate gets its own L1
// goroutine, so all N ProcessReverse loops run at once and contend the SAME
// global txn.cache.Lock() that serial AddDelta takes per target.
//
// This is the effect a single-predicate benchmark cannot reproduce, and it is
// the leading explanation for why a production profile (25 predicates, ~70%
// @reverse, 20 concurrent writer threads) attributes far more time to the
// reverse write than a one-predicate microbenchmark does. If the parallel write
// — which moves these writes onto sharded per-key locks via AddDeltaConcurrent —
// helps MORE as predicate count rises, cross-predicate lock relief is real and
// the single-predicate number understates the production gain.
func BenchmarkReverseMultiPredicate(b *testing.B) {
	const totalEdges = 19980
	// 25 predicates + budget=30 is the shipped production shape: worker/server_state.go
	// defaults mutations-pipeline-goroutines=30, and allocateWorkers hands 25
	// predicates a {1:20, 2:5} grant — i.e. most predicates get a ONE-worker grant.
	for _, nPred := range []int{1, 4, 8, 16, 25} {
		nPred := nPred
		b.Run(fmt.Sprintf("preds=%d", nPred), func(b *testing.B) {
			require.NoError(b, pstore.DropAll())
			MemLayerInstance.clear()
			var sb strings.Builder
			for p := 0; p < nPred; p++ {
				fmt.Fprintf(&sb, "link%d: [uid] @reverse .\n", p)
			}
			require.NoError(b, schema.ParseBytes([]byte(sb.String()), 1))

			perPred := totalEdges / nPred
			edges := make([]*pb.DirectedEdge, 0, totalEdges)
			for p := 0; p < nPred; p++ {
				attr := x.AttrInRootNamespace(fmt.Sprintf("link%d", p))
				// Distinct target per source keeps every predicate above
				// reverseParallelMinTargets so each takes the parallel path.
				for i := 0; i < perPred; i++ {
					edges = append(edges, &pb.DirectedEdge{
						Entity:  uint64(1_000_000 + p*1_000_000 + i),
						Attr:    attr,
						ValueId: uint64(9_000_000 + p*1_000_000 + i),
						Op:      pb.DirectedEdge_SET,
					})
				}
			}
			for _, bud := range []int{0, 30, 32} {
				bud := bud
				b.Run(fmt.Sprintf("budget=%d", bud), func(b *testing.B) { runBudget(b, edges, bud, 1.0, 256) })
			}
		})
	}
}

// BenchmarkReverseHighCardConcurrent: the production high-cardinality shape.
// EVERY predicate has 8,000 distinct reverse targets (fan-in 2 => 16k edges
// each), so at 4 predicates this is a ~64k-edge batch — the real batch size —
// in which every predicate generates 8,000 independent <~pred,targetUid> write
// keys. Unlike BenchmarkReverseMultiPredicate (which divides a fixed edge budget
// across predicates and so thins the per-predicate cardinality), this holds
// cardinality FIXED at 8,000 and scales the batch, isolating the effect of
// several high-cardinality reverse predicates contending the global cache lock
// at once.
func BenchmarkReverseHighCardConcurrent(b *testing.B) {
	const targetsPerPred = 8000
	const fanIn = 2
	for _, nPred := range []int{1, 2, 4} {
		nPred := nPred
		b.Run(fmt.Sprintf("preds=%d", nPred), func(b *testing.B) {
			require.NoError(b, pstore.DropAll())
			MemLayerInstance.clear()
			var sb strings.Builder
			for p := 0; p < nPred; p++ {
				fmt.Fprintf(&sb, "hc%d: [uid] @reverse .\n", p)
			}
			require.NoError(b, schema.ParseBytes([]byte(sb.String()), 1))

			edges := make([]*pb.DirectedEdge, 0, nPred*targetsPerPred*fanIn)
			for p := 0; p < nPred; p++ {
				attr := x.AttrInRootNamespace(fmt.Sprintf("hc%d", p))
				for i := 0; i < targetsPerPred*fanIn; i++ {
					edges = append(edges, &pb.DirectedEdge{
						Entity:  uint64(1_000_000 + p*1_000_000 + i),
						Attr:    attr,
						ValueId: uint64(9_000_000 + p*1_000_000 + i%targetsPerPred),
						Op:      pb.DirectedEdge_SET,
					})
				}
			}
			for _, bud := range []int{0, 30} {
				bud := bud
				b.Run(fmt.Sprintf("budget=%d", bud), func(b *testing.B) { runBudget(b, edges, bud, 1.0, 256) })
			}
		})
	}
}

// BenchmarkReverseFiftyFifty: ~20k edges, 50% on a `[uid] @reverse` predicate
// (no speedup) and 50% on a scalar `@index` predicate (full speedup) — the
// "50% of incoming statements have an @reverse index" assumption. Shows the
// diluted, real-workload speedup.
func BenchmarkReverseFiftyFifty(b *testing.B) {
	require.NoError(b, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(b, schema.ParseBytes([]byte(`
		sname: string @index(exact) .
		link:  [uid] @reverse .
	`), 1))
	sname := x.AttrInRootNamespace("sname")
	link := x.AttrInRootNamespace("link")
	edges := append(buildScalarIndexedEdges(sname, 10000, 50),
		buildReverseEdges(link, 10000, 5)...)

	for _, bud := range []int{0, 8, 32} {
		bud := bud
		b.Run(fmt.Sprintf("budget=%d", bud), func(b *testing.B) { runBudget(b, edges, bud, 1.0, 256) })
	}
	b.Run("budget=auto", func(b *testing.B) { runBudget(b, edges, mutationsPipelineGoroutinesAuto, 1.0, 256) })
}

// BenchmarkScalarDominant: control — ~20k edges, dominant scalar `@index`
// predicate (matches the original sweep's best case) to contrast against the
// reverse-dominant number.
func BenchmarkScalarDominant(b *testing.B) {
	require.NoError(b, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(b, schema.ParseBytes([]byte(`sname: string @index(exact) .`), 1))
	sname := x.AttrInRootNamespace("sname")
	edges := buildScalarIndexedEdges(sname, 19980, 50)

	for _, bud := range []int{0, 8, 32} {
		bud := bud
		b.Run(fmt.Sprintf("budget=%d", bud), func(b *testing.B) { runBudget(b, edges, bud, 1.0, 256) })
	}
	b.Run("budget=auto", func(b *testing.B) { runBudget(b, edges, mutationsPipelineGoroutinesAuto, 1.0, 256) })
}
