/*
 * Reverse-heavy benchmark: measures whether the auto/budget hybrid speedup
 * reaches @reverse [uid] predicates (it routes to ProcessList, which is serial).
 * Self-contained; package-internal (white-box).
 */

package posting

import (
	"context"
	"fmt"
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
