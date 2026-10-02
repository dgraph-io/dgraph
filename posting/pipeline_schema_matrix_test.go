/*
 * Schema/index-type matrix: confirms the intra-predicate goroutine budget
 * (proportional, auto, and the ProcessList parallel forward write) produces
 * byte-identical committed state and identical conflict-key sets vs. the legacy
 * one-goroutine-per-predicate path, across a wide range of Dgraph value types
 * and index tokenizers.
 *
 * The comparison is fully generic: for every predicate it scans ALL committed
 * Badger keys (data, every index token, reverse, count) at a read timestamp and
 * dumps each posting list canonically, then asserts the budget>1 dump equals the
 * budget=0 dump. This covers any tokenizer without hand-coding token readers.
 */

package posting

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

// schemaCase is one row of the matrix: a schema, the root-namespace predicate
// attrs it declares, and a builder for a representative mutation batch.
type schemaCase struct {
	name   string
	schema string
	attrs  []string
	edges  func(attrs []string) []*pb.DirectedEdge
}

// strEdge builds a scalar edge carrying a textual value; ValidateAndConvert
// converts it to the schema's scalar type (the RDF ingest path).
func strEdge(attr string, entity uint64, val string) *pb.DirectedEdge {
	return &pb.DirectedEdge{
		Entity: entity, Attr: attr,
		Value: []byte(val), ValueType: pb.Posting_STRING, Op: pb.DirectedEdge_SET,
	}
}

// uidEdge builds a uid edge (forward target in ValueId).
func uidEdge(attr string, entity, target uint64) *pb.DirectedEdge {
	return &pb.DirectedEdge{
		Entity: entity, Attr: attr, ValueId: target, Op: pb.DirectedEdge_SET,
	}
}

// snapshotPredicates scans every committed key under each predicate's prefix at
// readTs and returns hex(key) -> canonical posting-list dump. Two runs that
// commit identical logical state produce identical maps.
func snapshotPredicates(t *testing.T, attrs []string, readTs uint64) map[string]string {
	t.Helper()
	out := map[string]string{}
	txn := pstore.NewTransactionAt(readTs, false)
	defer txn.Discard()
	for _, attr := range attrs {
		prefix := x.PredicatePrefix(attr)
		iopt := badger.DefaultIteratorOptions
		iopt.AllVersions = false
		iopt.PrefetchValues = false
		iopt.Prefix = prefix
		it := txn.NewIterator(iopt)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().KeyCopy(nil)
			l, err := GetNoStore(key, readTs)
			require.NoError(t, err)
			var postings []string
			err = l.Iterate(readTs, 0, func(p *pb.Posting) error {
				postings = append(postings, fmt.Sprintf(
					"u=%d|v=%s|pt=%d|lang=%s|facets=%v",
					p.Uid, hex.EncodeToString(p.Value), p.PostingType,
					hex.EncodeToString(p.LangTag), p.Facets))
				return nil
			})
			require.NoError(t, err)
			// postings already in uid order from Iterate; make deterministic anyway.
			sort.Strings(postings)
			out[hex.EncodeToString(key)] = fmt.Sprintf("%v", postings)
		}
		it.Close()
	}
	return out
}

// keyKinds parses the snapshot's keys and counts how many are data / index /
// reverse / count. Used to prove the comparison actually covers the secondary
// key types a schema declares (so the test can't pass by comparing data only).
func keyKinds(t *testing.T, snap map[string]string) (data, index, reverse, count int) {
	t.Helper()
	for hk := range snap {
		raw, err := hex.DecodeString(hk)
		require.NoError(t, err)
		pk, err := x.Parse(raw)
		require.NoError(t, err)
		switch {
		case pk.IsCountOrCountRev():
			count++
		case pk.IsReverse():
			reverse++
		case pk.IsIndex():
			index++
		case pk.IsData():
			data++
		}
	}
	return
}

// runMatrixBatch applies edges through a fresh pipeline txn at the given budget
// (and auto tunables), snapshots the conflict-key set, commits, and restores the
// config. Mirrors runBudgetBatch but also sets the edges-per-worker cap.
func runMatrixBatch(t *testing.T, par x.IntraMutationParallelism, minEdges int,
	startTs, commitTs uint64, edges []*pb.DirectedEdge) map[uint64]struct{} {
	t.Helper()
	ob := x.WorkerConfig.IntraMutationParallelism
	om := x.WorkerConfig.IntraMutationEdgesPerWorker
	x.WorkerConfig.IntraMutationParallelism = par
	x.WorkerConfig.IntraMutationEdgesPerWorker = minEdges
	defer func() {
		x.WorkerConfig.IntraMutationParallelism = ob
		x.WorkerConfig.IntraMutationEdgesPerWorker = om
	}()

	txn := Oracle().RegisterStartTs(startTs)
	mp := NewMutationPipeline(txn)
	require.NoError(t, mp.Process(context.Background(), edges))

	txn.Lock()
	conflicts := make(map[uint64]struct{}, len(txn.conflicts))
	for k := range txn.conflicts {
		conflicts[k] = struct{}{}
	}
	txn.Unlock()

	commitPipelineTxn(t, txn, commitTs)
	return conflicts
}

// nonEmpty drops keys whose posting list is empty, so a snapshot comparison
// asserts agreement on real content rather than on leftover emptied buckets.
func nonEmpty(snap map[string]string) map[string]string {
	out := make(map[string]string, len(snap))
	for k, v := range snap {
		if v != "[]" {
			out[k] = v
		}
	}
	return out
}

// runMatrixLegacy is runMatrixBatch's legacy counterpart: it applies the edges
// through a serial runMutation loop — the path draft.go takes when a mutation
// bypasses the pipeline — then snapshots the conflict-key set and commits.
//
// Edges are stable-sorted by (Attr, Entity) first, exactly as applyMutations
// does before its DivideAndRule fan-out. That matters for byte-identity: the
// sort preserves relative order within each (attr, entity) group, and the
// pipeline preserves order within a predicate, so both paths observe the same
// sequence of operations per key.
func runMatrixLegacy(t *testing.T, startTs, commitTs uint64,
	edges []*pb.DirectedEdge) map[uint64]struct{} {
	t.Helper()
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].GetAttr() != edges[j].GetAttr() {
			return edges[i].GetAttr() < edges[j].GetAttr()
		}
		return edges[i].GetEntity() < edges[j].GetEntity()
	})

	ctx := schema.GetWriteContext(context.Background())
	txn := Oracle().RegisterStartTs(startTs)
	for _, edge := range edges {
		// worker.runMutation validates and converts before delegating to the
		// posting-level runMutation, which does not. Skipping this step leaves
		// scalar values as raw RDF strings ("10" instead of an encoded int), so
		// the baseline would write different keys and every scalar case would
		// "fail" for a harness reason rather than a real divergence.
		su, ok := schema.State().Get(ctx, edge.Attr)
		if edge.Op != pb.DirectedEdge_DEL {
			require.Truef(t, ok, "no schema for %s", edge.Attr)
		}
		require.NoError(t, ValidateAndConvert(edge, &su))
		require.NoError(t, runMutation(ctx, edge, txn))
	}

	txn.Lock()
	conflicts := make(map[uint64]struct{}, len(txn.conflicts))
	for k := range txn.conflicts {
		conflicts[k] = struct{}{}
	}
	txn.Unlock()

	commitPipelineTxn(t, txn, commitTs)
	return conflicts
}

// matrixCases is the schema/index-type matrix (default batch size).
func matrixCases() []schemaCase { return matrixCasesN(600) }

// matrixCasesN is the schema/index-type matrix with n edges per scalar predicate.
func matrixCasesN(n int) []schemaCase {
	scalar := func(valOf func(i int) string) func([]string) []*pb.DirectedEdge {
		return func(attrs []string) []*pb.DirectedEdge {
			e := make([]*pb.DirectedEdge, 0, n)
			for i := 0; i < n; i++ {
				e = append(e, strEdge(attrs[0], uint64(1_000_000+i), valOf(i)))
			}
			return e
		}
	}
	return []schemaCase{
		{
			name:   "string_multi_index", // exact+hash+term+fulltext+trigram on one pred
			schema: `p: string @index(exact, hash, term, fulltext, trigram) .`,
			attrs:  []string{"p"},
			edges:  scalar(func(i int) string { return fmt.Sprintf("the quick brown fox %d", i%40) }),
		},
		{
			name:   "int_count",
			schema: `p: int @index(int) @count .`,
			attrs:  []string{"p"},
			edges:  scalar(func(i int) string { return fmt.Sprintf("%d", i%50) }),
		},
		{
			name:   "float_index",
			schema: `p: float @index(float) .`,
			attrs:  []string{"p"},
			edges:  scalar(func(i int) string { return fmt.Sprintf("%d.25", i%50) }),
		},
		{
			name:   "datetime_index",
			schema: `p: dateTime @index(day) .`,
			attrs:  []string{"p"},
			edges:  scalar(func(i int) string { return fmt.Sprintf("2021-%02d-15T10:30:00Z", (i%12)+1) }),
		},
		{
			name:   "bool_index",
			schema: `p: bool @index(bool) .`,
			attrs:  []string{"p"},
			edges:  scalar(func(i int) string { return fmt.Sprintf("%t", i%2 == 0) }),
		},
		{
			name:   "geo_index",
			schema: `p: geo @index(geo) .`,
			attrs:  []string{"p"},
			edges: scalar(func(i int) string {
				return fmt.Sprintf(`{"type":"Point","coordinates":[%d.0,%d.0]}`, i%7, (i*2)%7)
			}),
		},
		{
			name:   "list_uid_reverse_count",
			schema: `p: [uid] @reverse @count .`,
			attrs:  []string{"p"},
			edges: func(attrs []string) []*pb.DirectedEdge {
				e := make([]*pb.DirectedEdge, 0, n)
				for i := 0; i < n; i++ { // many sources -> 5 hot reverse targets
					e = append(e, uidEdge(attrs[0], uint64(1_000_000+i), uint64(9_000_000+i%5)))
				}
				return e
			},
		},
		{
			// One distinct reverse target per source, so len(reverseredMap) == n
			// exceeds reverseParallelMinTargets and ProcessReverse takes its
			// PARALLEL write path. Every other @reverse case here lands on 5 hot
			// targets and stays serial, so without this row the parallel reverse
			// write would be uncovered by the byte-identity matrix.
			name:   "list_uid_reverse_highcard",
			schema: `p: [uid] @reverse .`,
			attrs:  []string{"p"},
			edges: func(attrs []string) []*pb.DirectedEdge {
				e := make([]*pb.DirectedEdge, 0, n)
				for i := 0; i < n; i++ {
					e = append(e, uidEdge(attrs[0], uint64(1_000_000+i), uint64(9_000_000+i)))
				}
				return e
			},
		},
		{
			name:   "uid_reverse_singular",
			schema: `p: uid @reverse .`,
			attrs:  []string{"p"},
			edges: func(attrs []string) []*pb.DirectedEdge {
				e := make([]*pb.DirectedEdge, 0, n)
				for i := 0; i < n; i++ {
					e = append(e, uidEdge(attrs[0], uint64(1_000_000+i), uint64(9_000_000+i%5)))
				}
				return e
			},
		},
		{
			// The singular-uid leg reaches ProcessReverse via ProcessSingle, not
			// ProcessList, and handleOldDeleteForSingle appends a synthetic DEL
			// carrying the OLD target — so one source uid can produce TWO distinct
			// reverse keys. uid_reverse_singular above lands on 5 hot targets and
			// stays serial, leaving that leg's parallel path uncovered; this row
			// gives it one distinct target per source so it exceeds
			// reverseParallelMinTargets.
			name:   "uid_reverse_singular_highcard",
			schema: `p: uid @reverse .`,
			attrs:  []string{"p"},
			edges: func(attrs []string) []*pb.DirectedEdge {
				e := make([]*pb.DirectedEdge, 0, n)
				for i := 0; i < n; i++ {
					e = append(e, uidEdge(attrs[0], uint64(1_000_000+i), uint64(9_000_000+i)))
				}
				return e
			},
		},
		{
			name:   "list_string_index",
			schema: `p: [string] @index(exact, term) .`,
			attrs:  []string{"p"},
			edges: func(attrs []string) []*pb.DirectedEdge {
				e := make([]*pb.DirectedEdge, 0, 2*n)
				for i := 0; i < n; i++ { // 2 values per entity -> list postings + shared tokens
					ent := uint64(1_000_000 + i)
					e = append(e, strEdge(attrs[0], ent, fmt.Sprintf("tag%d", i%30)))
					e = append(e, strEdge(attrs[0], ent, fmt.Sprintf("tag%d", (i+1)%30)))
				}
				return e
			},
		},
		{
			name:   "upsert_and_noconflict",
			schema: "up: string @index(exact) @upsert .\n nc: string @index(hash) @noconflict .",
			attrs:  []string{"up", "nc"},
			edges: func(attrs []string) []*pb.DirectedEdge {
				e := make([]*pb.DirectedEdge, 0, 2*n)
				for i := 0; i < n; i++ {
					e = append(e, strEdge(attrs[0], uint64(1_000_000+i), fmt.Sprintf("u%d", i%50)))
					e = append(e, strEdge(attrs[1], uint64(1_000_000+i), fmt.Sprintf("c%d", i%50)))
				}
				return e
			},
		},
		{
			name:   "lang_fulltext",
			schema: `p: string @index(fulltext) @lang .`,
			attrs:  []string{"p"},
			edges: func(attrs []string) []*pb.DirectedEdge {
				// buildLangEdges (existing helper): n entities across several langs.
				return buildLangEdges(attrs[0], n, 4)
			},
		},
		{
			name:   "mixed_hot_predicate", // skewed multi-predicate, dominant scalar + reverse + index
			schema: "name: string @index(exact) .\n friend: [uid] @reverse .\n age: int @index(int) .",
			attrs:  []string{"name", "friend", "age"},
			edges: func(attrs []string) []*pb.DirectedEdge {
				e := make([]*pb.DirectedEdge, 0, n+200)
				for i := 0; i < n; i++ { // dominant: name
					e = append(e, strEdge(attrs[0], uint64(1_000_000+i), fmt.Sprintf("name%d", i%50)))
				}
				for i := 0; i < 100; i++ {
					e = append(e, uidEdge(attrs[1], uint64(2_000_000+i), uint64(9_000_000+i%5)))
					e = append(e, strEdge(attrs[2], uint64(3_000_000+i), fmt.Sprintf("%d", 20+i%40)))
				}
				return e
			},
		},
	}
}

// TestSchemaMatrixByteIdentical is the headline robustness test: for every
// schema/index combination, the proportional budget (fixed 8, fixed 32, and
// auto) must produce byte-identical committed state and identical conflict keys
// vs. the legacy (budget=0) path. Run under -race to also cover the concurrent
// data-write / index-tokenization / ProcessList paths.
func TestSchemaMatrixByteIdentical(t *testing.T) {
	type budgetCfg struct {
		name     string
		par      x.IntraMutationParallelism
		minEdges int
	}
	budgets := []budgetCfg{
		{"fixed8", x.IntraMutationParallelism{Workers: 8}, 256},
		{"fixed32", x.IntraMutationParallelism{Workers: 32}, 256},
		{"auto", x.IntraMutationParallelism{PerCPU: 1.0}, 64},
	}

	var ts uint64 = 1_000_000
	next := func() (uint64, uint64, uint64) { ts += 100; return ts, ts + 1, ts + 2 }

	reset := func(t *testing.T, schemaText string) {
		require.NoError(t, pstore.DropAll())
		MemLayerInstance.clear()
		require.NoError(t, schema.ParseBytes([]byte(schemaText), 1))
	}

	for _, sc := range matrixCases() {
		attrs := make([]string, len(sc.attrs))
		for i, a := range sc.attrs {
			attrs[i] = x.AttrInRootNamespace(a)
		}
		for _, bc := range budgets {
			sc, bc, attrs := sc, bc, attrs
			t.Run(sc.name+"/"+bc.name, func(t *testing.T) {
				// Baseline: the REAL legacy path — a serial runMutation loop, the
				// same per-edge call draft.go makes when
				// intra-mutation-min-edges=0 sends a mutation around the pipeline.
				//
				// This used to be "the pipeline with the budget off", which was an
				// in-pipeline replica of legacy semantics (locked AddDelta). Once
				// the lock-free store stopped being tied to the worker grant that
				// replica disappeared, and with it the only in-package proof that
				// AddDeltaConcurrent agrees with AddDelta. Comparing against
				// runMutation is strictly stronger: it is the alternative an
				// operator can actually select, not a stand-in for it.
				reset(t, sc.schema)
				s0, c0, r0 := next()
				// Conflict keys from the legacy run are intentionally unused — see
				// the comparison notes below for why they are not asserted against.
				_ = runMatrixLegacy(t, s0, c0, sc.edges(attrs))
				base := snapshotPredicates(t, attrs, r0)
				require.NotEmpty(t, base, "baseline wrote no keys for %s", sc.name)

				// Harden: prove the snapshot actually covers the secondary key
				// types this schema declares, so the equality check is meaningful
				// (not just comparing data keys).
				data, index, reverse, count := keyKinds(t, base)
				require.Positive(t, data, "expected data keys (%s)", sc.name)
				if strings.Contains(sc.schema, "@index") {
					require.Positive(t, index, "expected index keys (%s)", sc.name)
				}
				if strings.Contains(sc.schema, "@reverse") {
					require.Positive(t, reverse, "expected reverse keys (%s)", sc.name)
				}
				if strings.Contains(sc.schema, "@count") {
					require.Positive(t, count, "expected count keys (%s)", sc.name)
				}

				// The pipeline with no fan-out: the conflict-key reference. Conflict
				// keys are compared pipeline-to-pipeline, NOT against legacy — see
				// the subset assertion below for why.
				reset(t, sc.schema)
				sOff, cOff, _ := next()
				confOff := runMatrixBatch(t, x.IntraMutationParallelism{}, 256,
					sOff, cOff, sc.edges(attrs))

				// Candidate: the parallelism setting under test.
				reset(t, sc.schema)
				s1, c1, r1 := next()
				confN := runMatrixBatch(t, bc.par, bc.minEdges, s1, c1, sc.edges(attrs))
				cand := snapshotPredicates(t, attrs, r1)

				// The strong check: every posting list the pipeline commits is
				// byte-identical to what the legacy per-edge path commits, for every
				// schema and every tokenizer.
				//
				// Empty lists are excluded because legacy leaves behind emptied
				// count buckets that the pipeline never creates — for
				// `[uid] @reverse @count` legacy writes 726 keys to the pipeline's
				// 607, and all 119 extras are empty with zero differing values. That
				// predates this work (measured identically on the pre-change tree)
				// and the pipeline's output is the cleaner of the two, so comparing
				// non-empty state asserts the real invariant without pinning a
				// difference this change did not introduce.
				require.Equal(t, nonEmpty(base), nonEmpty(cand),
					"committed state must be byte-identical to legacy (%s, %s)",
					sc.name, bc.name)

				// Conflict keys are compared pipeline-to-pipeline: parallel must
				// agree with serial. This is the guarantee this branch's work has to
				// preserve, and it is what the production abort-count parity rests
				// on.
				//
				// They are deliberately NOT compared against legacy. The pipeline
				// diverges from legacy in both directions there and has since before
				// this work: `uid @reverse` gives 1200 pipeline keys to legacy's 600,
				// and `@lang @index(fulltext)` gives 4800 to legacy's 3000 while
				// dropping 600 that legacy emits. Both were measured on the
				// pre-change tree. Worth its own investigation — see pipeline-todo.md
				// — but pinning it here would assert a behavior this change neither
				// caused nor fixed.
				require.Equal(t, confOff, confN,
					"conflict-key set must not depend on parallelism (%s, %s)",
					sc.name, bc.name)
			})
		}
	}
}
