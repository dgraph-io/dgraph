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
// config. Mirrors runBudgetBatch but also sets the auto fraction/min-edges.
func runMatrixBatch(t *testing.T, budget int, fraction float64, minEdges int,
	startTs, commitTs uint64, edges []*pb.DirectedEdge) map[uint64]struct{} {
	t.Helper()
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

// matrixCases is the schema/index-type matrix.
func matrixCases() []schemaCase {
	const n = 600 // edges per scalar predicate; > any tested budget so the split fires
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
		budget   int
		fraction float64
		minEdges int
	}
	budgets := []budgetCfg{
		{"fixed8", 8, 1.0, 256},
		{"fixed32", 32, 1.0, 256},
		{"auto", mutationsPipelineGoroutinesAuto, 1.0, 64},
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
				// Baseline: legacy one-goroutine-per-predicate (budget 0).
				reset(t, sc.schema)
				s0, c0, r0 := next()
				conf0 := runMatrixBatch(t, 0, 1.0, 256, s0, c0, sc.edges(attrs))
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

				// Candidate: the budget under test.
				reset(t, sc.schema)
				s1, c1, r1 := next()
				confN := runMatrixBatch(t, bc.budget, bc.fraction, bc.minEdges, s1, c1, sc.edges(attrs))
				cand := snapshotPredicates(t, attrs, r1)

				require.Equal(t, base, cand,
					"committed state must be byte-identical (%s, %s)", sc.name, bc.name)
				require.Equal(t, conf0, confN,
					"conflict-key set must be identical (%s, %s)", sc.name, bc.name)
			})
		}
	}
}
