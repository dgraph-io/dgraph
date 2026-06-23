/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

// Tests and benchmark for the proportional intra-predicate goroutine budget
// (mutations-pipeline-goroutines). The budget lets a hot/dominant predicate use
// more than one goroutine for its merge-light passes (data-write, index
// tokenization) while staying byte-identical to the legacy one-goroutine path.

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

// Timestamp bands for budget tests (kept clear of the existing pipeline_test.go
// bands documented there, which top out at 2599):
//   Byte-identical, budget=0:   5000-5099
//   Byte-identical, budget=30:  5100-5199
//   Auto byte-identical:        5200-5299
//   ProcessList byte-identical: 5300-5399
//   ProcessList cross-proposal: 5400-5499

// buildSkewedBatch produces a deterministic, predicate-skewed mutation batch:
// one dominant scalar predicate (nameAttr) with nameCount edges, plus two small
// predicates (deptAttr scalar, friendAttr uid). Entities live in disjoint uid
// ranges per predicate. name values repeat across a small set so the index has
// multi-uid buckets; friend targets repeat so reverse lists have multiple
// sources. Reused by both the byte-identical test and the benchmark.
// cloneEdges returns a fresh copy of each edge struct. The pipeline mutates an
// edge's ValueId in place (setting the scalar-value marker for scalar postings),
// which is fine in production where every transaction gets fresh edges from the
// proto, but breaks tests/benchmarks that feed the same slice to Process twice.
func cloneEdges(in []*pb.DirectedEdge) []*pb.DirectedEdge {
	out := make([]*pb.DirectedEdge, len(in))
	for i, e := range in {
		c := *e
		out[i] = &c
	}
	return out
}

func buildSkewedBatch(nameAttr, deptAttr, friendAttr string, nameCount int) []*pb.DirectedEdge {
	const (
		nameBase      = uint64(1_000_000)
		deptBase      = uint64(2_000_000)
		friendBase    = uint64(3_000_000)
		targetBase    = uint64(4_000_000)
		smallCount    = 10
		nameDistinct  = 50
		deptDistinct  = 3
		friendTargets = 4
	)
	edges := make([]*pb.DirectedEdge, 0, nameCount+2*smallCount)
	for i := 0; i < nameCount; i++ {
		edges = append(edges, &pb.DirectedEdge{
			Entity:    nameBase + uint64(i),
			Attr:      nameAttr,
			Value:     []byte(fmt.Sprintf("name%d", i%nameDistinct)),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		})
	}
	for i := 0; i < smallCount; i++ {
		edges = append(edges, &pb.DirectedEdge{
			Entity:    deptBase + uint64(i),
			Attr:      deptAttr,
			Value:     []byte(fmt.Sprintf("dept%d", i%deptDistinct)),
			ValueType: pb.Posting_STRING,
			Op:        pb.DirectedEdge_SET,
		})
	}
	for i := 0; i < smallCount; i++ {
		edges = append(edges, &pb.DirectedEdge{
			Entity:  friendBase + uint64(i),
			Attr:    friendAttr,
			ValueId: targetBase + uint64(i%friendTargets),
			Op:      pb.DirectedEdge_SET,
		})
	}
	return edges
}

// buildLangEdges produces langs language postings for each of n distinct
// entities of a `string @lang` predicate. Each (entity, lang) pair is a distinct
// posting (Uid = fingerprint(lang)) but all fold into the entity's single
// <pred,entity> forward data key — so the lang forward-write is one-to-one by
// source entity, the disjointness the ProcessList parallel split relies on.
func buildLangEdges(attr string, n, langs int) []*pb.DirectedEdge {
	const base = uint64(5_000_000)
	langCodes := []string{"en", "de", "fr", "es", "it"}
	if langs > len(langCodes) {
		langs = len(langCodes)
	}
	edges := make([]*pb.DirectedEdge, 0, n*langs)
	for i := 0; i < n; i++ {
		for l := 0; l < langs; l++ {
			edges = append(edges, &pb.DirectedEdge{
				Entity:    base + uint64(i),
				Attr:      attr,
				Value:     []byte(fmt.Sprintf("bio%d-%s", i, langCodes[l])),
				ValueType: pb.Posting_STRING,
				Lang:      langCodes[l],
				Op:        pb.DirectedEdge_SET,
			})
		}
	}
	return edges
}

func sortedUint64(in []uint64) []uint64 {
	out := append([]uint64(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// runBudgetBatch sets the global budget, runs the batch through a fresh pipeline
// txn, snapshots the conflict-key set produced (before commit, which does not
// clear it), commits to Badger, and returns the snapshot. The budget is restored
// on return so other tests still see the disabled (zero-value) default.
func runBudgetBatch(t *testing.T, budget int, startTs, commitTs uint64,
	edges []*pb.DirectedEdge) map[uint64]struct{} {
	t.Helper()
	old := x.WorkerConfig.MutationsPipelineGoroutines
	x.WorkerConfig.MutationsPipelineGoroutines = budget
	defer func() { x.WorkerConfig.MutationsPipelineGoroutines = old }()

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

// runTwoProposalBatch runs two mutation batches as two SEPARATE Process calls on
// the SAME registered transaction, modeling a multi-statement (CommitNow=false)
// transaction: worker/draft.go's RegisterStartTs reuses the *Txn for a start ts,
// and txn.Update between proposals does not clear txn.cache.deltas, so a list/lang
// predicate's forward deltas accumulate across proposals via AddDelta's
// addToList=true. The single-Process byte-identical tests never exercise this
// cross-proposal merge. Snapshots the conflict-key set, commits, restores the
// budget.
func runTwoProposalBatch(t *testing.T, budget int, startTs, commitTs uint64,
	batch1, batch2 []*pb.DirectedEdge) map[uint64]struct{} {
	t.Helper()
	old := x.WorkerConfig.MutationsPipelineGoroutines
	x.WorkerConfig.MutationsPipelineGoroutines = budget
	defer func() { x.WorkerConfig.MutationsPipelineGoroutines = old }()

	txn := Oracle().RegisterStartTs(startTs)

	mp1 := NewMutationPipeline(txn)
	require.NoError(t, mp1.Process(context.Background(), batch1))
	txn.Update() // mirrors draft.go's `defer txn.Update()` per applyMutations.

	mp2 := NewMutationPipeline(txn)
	require.NoError(t, mp2.Process(context.Background(), batch2))

	txn.Lock()
	conflicts := make(map[uint64]struct{}, len(txn.conflicts))
	for k := range txn.conflicts {
		conflicts[k] = struct{}{}
	}
	txn.Unlock()

	commitPipelineTxn(t, txn, commitTs)
	return conflicts
}

// TestProcessListCrossProposalByteIdentical is the regression test for the
// ProcessList parallel-write addToList bug: the parallel concStore must replicate
// AddDelta's addToList=true so a list/lang forward delta accumulated by an EARLIER
// proposal on the same txn is preserved by a LATER proposal. Two proposals SET the
// same source uids to a second target (and the same lang entities in a second
// language). The serial path appends across proposals ([A,B]); a parallel path
// that overwrote would drop A. Asserts the budget>1 result equals the serial
// baseline AND that the baseline itself carries both proposals' postings (so the
// test fails on the buggy overwrite, not merely on disagreement). Run under -race.
func TestProcessListCrossProposalByteIdentical(t *testing.T) {
	const (
		linkSrc     = 60 // distinct source uids touched in both proposals
		bioEntities = 40 // distinct @lang entities touched in both proposals
	)

	linkAttr := x.AttrInRootNamespace("link")
	bioAttr := x.AttrInRootNamespace("bio")

	const (
		srcBase = uint64(1_000_000)
		bioBase = uint64(5_000_000)
		targetA = uint64(9_000_001)
		targetB = uint64(9_000_002)
	)

	buildLink := func(target uint64) []*pb.DirectedEdge {
		edges := make([]*pb.DirectedEdge, 0, linkSrc)
		for i := 0; i < linkSrc; i++ {
			edges = append(edges, &pb.DirectedEdge{
				Entity:  srcBase + uint64(i),
				Attr:    linkAttr,
				ValueId: target,
				Op:      pb.DirectedEdge_SET,
			})
		}
		return edges
	}
	buildBio := func(lang string) []*pb.DirectedEdge {
		edges := make([]*pb.DirectedEdge, 0, bioEntities)
		for i := 0; i < bioEntities; i++ {
			edges = append(edges, &pb.DirectedEdge{
				Entity:    bioBase + uint64(i),
				Attr:      bioAttr,
				Value:     []byte(fmt.Sprintf("%s%d", lang, i)),
				ValueType: pb.Posting_STRING,
				Lang:      lang,
				Op:        pb.DirectedEdge_SET,
			})
		}
		return edges
	}

	batch1 := append(buildLink(targetA), buildBio("en")...)
	batch2 := append(buildLink(targetB), buildBio("de")...)

	// Sanity: budget=30 grants BOTH ProcessList predicates more than one worker in
	// each proposal (link=60, bio=40 per proposal), so the parallel concStore path
	// is genuinely taken on the second proposal where the cross-proposal merge runs.
	alloc := allocateWorkers(map[string]int{linkAttr: linkSrc, bioAttr: bioEntities}, 30)
	require.Greater(t, alloc[linkAttr], 1, "test must exercise multi-worker [uid] predicate")
	require.Greater(t, alloc[bioAttr], 1, "test must exercise multi-worker @lang predicate")

	schemaBytes := []byte(`
		link: [uid] @reverse .
		bio:  string @lang .
	`)

	langVals := func(t *testing.T, attr string, entity, readTs uint64) map[string]string {
		t.Helper()
		l, err := GetNoStore(x.DataKey(attr, entity), readTs)
		require.NoError(t, err)
		out := map[string]string{}
		require.NoError(t, l.Iterate(readTs, 0, func(p *pb.Posting) error {
			out[string(p.LangTag)] = string(p.Value)
			return nil
		}))
		return out
	}

	type result struct {
		fwd  map[uint64][]uint64
		rev  map[uint64][]uint64
		lang map[uint64]map[string]string
	}

	collect := func(readTs uint64) result {
		r := result{
			fwd:  map[uint64][]uint64{},
			rev:  map[uint64][]uint64{},
			lang: map[uint64]map[string]string{},
		}
		for i := 0; i < linkSrc; i++ {
			src := srcBase + uint64(i)
			r.fwd[src] = sortedUint64(forwardUids(t, linkAttr, src, readTs))
		}
		for _, tgt := range []uint64{targetA, targetB} {
			r.rev[tgt] = sortedUint64(reverseUids(t, linkAttr, tgt, readTs))
		}
		for i := 0; i < bioEntities; i++ {
			e := bioBase + uint64(i)
			r.lang[e] = langVals(t, bioAttr, e, readTs)
		}
		return r
	}

	// Run 1 — budget disabled (serial cross-proposal addToList merge).
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflicts0 := runTwoProposalBatch(t, 0, 5400, 5401, cloneEdges(batch1), cloneEdges(batch2))
	res0 := collect(5402)

	// Run 2 — budget=30 (parallel concStore must reproduce the merge).
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflicts30 := runTwoProposalBatch(t, 30, 5410, 5411, cloneEdges(batch1), cloneEdges(batch2))
	res30 := collect(5412)

	// The serial baseline must carry BOTH proposals' postings — otherwise the
	// equality assertions below could pass on a shared (wrong) overwrite result.
	wantFwd := sortedUint64([]uint64{targetA, targetB})
	for i := 0; i < linkSrc; i++ {
		require.Equal(t, wantFwd, res0.fwd[srcBase+uint64(i)],
			"serial forward list must append across proposals")
	}
	for i := 0; i < bioEntities; i++ {
		require.Equal(t, map[string]string{
			"en": fmt.Sprintf("en%d", i),
			"de": fmt.Sprintf("de%d", i),
		}, res0.lang[bioBase+uint64(i)], "serial lang values must carry both proposals")
	}

	require.Equal(t, res0.fwd, res30.fwd, "forward uid lists must match across budgets")
	require.Equal(t, res0.rev, res30.rev, "reverse lists must match across budgets")
	require.Equal(t, res0.lang, res30.lang, "lang values must match across budgets")
	require.Equal(t, conflicts0, conflicts30, "conflict-key sets must be identical")
}

// TestAllocateWorkers exercises the pure apportionment helper: floor of 1, the
// budget-disabled and saturated edge cases, the largest-remainder distribution,
// the floor-of-1 reclaim, and determinism — no cluster needed.
func TestAllocateWorkers(t *testing.T) {
	// Disabled budget (< 2) and empty input return nil → callers treat every
	// predicate as workers=0 (legacy path).
	require.Nil(t, allocateWorkers(map[string]int{"a": 5, "b": 3}, 0))
	require.Nil(t, allocateWorkers(map[string]int{"a": 5, "b": 3}, 1))
	require.Nil(t, allocateWorkers(map[string]int{}, 30))

	// P >= budget: one worker each, nothing left to split with.
	got := allocateWorkers(map[string]int{"a": 100, "b": 50, "c": 1}, 2)
	require.Equal(t, map[string]int{"a": 1, "b": 1, "c": 1}, got)

	// Even split: budget 4 across two equal predicates → 2 each.
	require.Equal(t, map[string]int{"a": 2, "b": 2},
		allocateWorkers(map[string]int{"a": 1, "b": 1}, 4))

	// Largest-remainder with deterministic tie-break: budget 3, two equal
	// predicates → quota 1.5 each, the single leftover goes to the
	// lexicographically smaller attr.
	require.Equal(t, map[string]int{"a": 2, "b": 1},
		allocateWorkers(map[string]int{"a": 1, "b": 1}, 3))

	// Skewed: one dominant predicate takes most of the budget; tiny ones get 1.
	// name=300, dept=10, friend=10, total=320, budget=30:
	//   name quota 28.125 -> 28, dept/friend quota 0.9375 -> 1 each; sum == 30.
	skewed := allocateWorkers(map[string]int{"name": 300, "dept": 10, "friend": 10}, 30)
	require.Equal(t, map[string]int{"name": 28, "dept": 1, "friend": 1}, skewed)

	// Floor-of-1 reclaim: a huge dominant plus 25 single-edge predicates
	// oversubscribes (28 ones bump the floors over budget); the overshoot is
	// reclaimed from the only predicate with >1 worker (the dominant).
	counts := map[string]int{"hot": 10000}
	for i := 0; i < 25; i++ {
		counts[fmt.Sprintf("tiny%02d", i)] = 1
	}
	reclaim := allocateWorkers(counts, 30)
	sum := 0
	for attr, w := range reclaim {
		require.GreaterOrEqual(t, w, 1, "every predicate must get >= 1 worker (%s)", attr)
		sum += w
	}
	require.Equal(t, 30, sum, "sum must equal budget when P < budget")
	require.Equal(t, 5, reclaim["hot"], "dominant keeps the remaining budget after the floor")
	for i := 0; i < 25; i++ {
		require.Equal(t, 1, reclaim[fmt.Sprintf("tiny%02d", i)])
	}

	// Determinism: the same input yields the same output across runs.
	require.Equal(t, skewed,
		allocateWorkers(map[string]int{"friend": 10, "name": 300, "dept": 10}, 30))
}

// TestPipelineBudgetByteIdentical proves the proportional budget produces results
// byte-identical to the legacy one-goroutine-per-predicate path: the same skewed
// batch run with budget=0 and budget=30 must yield identical committed data
// values, index buckets, forward/reverse lists, AND identical conflict-key sets.
// The dominant `name` predicate is granted >1 worker so both the parallel
// data-write and the parallel index tokenization (numGo=k) are exercised. Run it
// under -race to also cover I1/I2/I3.
func TestPipelineBudgetByteIdentical(t *testing.T) {
	const nameCount = 300

	nameAttr := x.AttrInRootNamespace("name")
	deptAttr := x.AttrInRootNamespace("dept")
	friendAttr := x.AttrInRootNamespace("friend")
	edges := buildSkewedBatch(nameAttr, deptAttr, friendAttr, nameCount)

	// Sanity: budget=30 actually grants the dominant predicate more than one
	// worker, so the parallel path under test is genuinely taken.
	alloc := allocateWorkers(map[string]int{nameAttr: nameCount, deptAttr: 10, friendAttr: 10}, 30)
	require.Greater(t, alloc[nameAttr], 1, "test must exercise multi-worker dominant predicate")

	schemaBytes := []byte(`
		name: string @index(exact) .
		dept: string @index(exact) .
		friend: uid @reverse .
	`)

	type result struct {
		nameVal map[uint64]string
		nameIdx map[string][]uint64
		deptIdx map[string][]uint64
		fwd     map[uint64][]uint64
		rev     map[uint64][]uint64
	}

	collect := func(readTs uint64) result {
		r := result{
			nameVal: map[uint64]string{},
			nameIdx: map[string][]uint64{},
			deptIdx: map[string][]uint64{},
			fwd:     map[uint64][]uint64{},
			rev:     map[uint64][]uint64{},
		}
		for i := 0; i < nameCount; i++ {
			e := uint64(1_000_000) + uint64(i)
			r.nameVal[e] = scalarValue(t, nameAttr, e, readTs)
		}
		for v := 0; v < 50; v++ {
			val := fmt.Sprintf("name%d", v)
			r.nameIdx[val] = sortedUint64(indexUidsForVal(t, nameAttr, val, readTs))
		}
		for v := 0; v < 3; v++ {
			val := fmt.Sprintf("dept%d", v)
			r.deptIdx[val] = sortedUint64(indexUidsForVal(t, deptAttr, val, readTs))
		}
		for i := 0; i < 10; i++ {
			src := uint64(3_000_000) + uint64(i)
			r.fwd[src] = sortedUint64(forwardUids(t, friendAttr, src, readTs))
		}
		for tgt := 0; tgt < 4; tgt++ {
			target := uint64(4_000_000) + uint64(tgt)
			r.rev[target] = sortedUint64(reverseUids(t, friendAttr, target, readTs))
		}
		return r
	}

	// Run 1 — budget disabled (legacy one-goroutine-per-predicate).
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflicts0 := runBudgetBatch(t, 0, 5000, 5001, cloneEdges(edges))
	res0 := collect(5002)

	// Run 2 — budget=30 (dominant predicate parallelized).
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflicts30 := runBudgetBatch(t, 30, 5100, 5101, cloneEdges(edges))
	res30 := collect(5102)

	require.Equal(t, res0.nameVal, res30.nameVal, "scalar data values must match")
	require.Equal(t, res0.nameIdx, res30.nameIdx, "name index buckets must match")
	require.Equal(t, res0.deptIdx, res30.deptIdx, "dept index buckets must match")
	require.Equal(t, res0.fwd, res30.fwd, "forward uid lists must match")
	require.Equal(t, res0.rev, res30.rev, "reverse lists must match")
	require.Equal(t, conflicts0, conflicts30, "conflict-key sets must be identical")
}

// TestProcessListBudgetByteIdentical is the ProcessList counterpart of
// TestPipelineBudgetByteIdentical. It proves the forward data-write split added to
// ProcessList is byte-identical to the serial path for the predicates that route
// THROUGH ProcessList — which the existing budget tests do NOT cover: their
// `friend: uid @reverse` is a scalar uid predicate (no @lang, not a list) so it
// goes to ProcessSingle. Here `link: [uid] @reverse` (info.isList → ProcessList,
// info.isUid → the sort/dedup concStore branch) is the dominant predicate, plus a
// small `bio: string @lang` predicate (su.Lang → ProcessList, non-uid branch) so
// the lang forward-write split is exercised too. The same batch run with budget=0
// (serial) and budget=30 (both predicates parallelized) must yield identical
// forward uid lists, reverse lists, lang values, AND conflict-key sets. Run under
// -race to also cover the disjoint-key/per-worker-buffer invariants.
func TestProcessListBudgetByteIdentical(t *testing.T) {
	const (
		linkCount   = 300 // distinct source uids → 5 hot reverse targets
		hotTargets  = 5
		bioEntities = 40
		bioLangs    = 3 // 40*3 = 120 lang postings
	)

	linkAttr := x.AttrInRootNamespace("link")
	bioAttr := x.AttrInRootNamespace("bio")

	edges := append(buildReverseEdges(linkAttr, linkCount, hotTargets),
		buildLangEdges(bioAttr, bioEntities, bioLangs)...)

	// Sanity: budget=30 grants BOTH predicates more than one worker, so the
	// multi-worker ProcessList path under test is genuinely taken for the [uid]
	// case AND the lang case.
	alloc := allocateWorkers(map[string]int{linkAttr: linkCount, bioAttr: bioEntities * bioLangs}, 30)
	require.Greater(t, alloc[linkAttr], 1, "test must exercise multi-worker [uid] @reverse predicate")
	require.Greater(t, alloc[bioAttr], 1, "test must exercise multi-worker @lang predicate")

	schemaBytes := []byte(`
		link: [uid] @reverse .
		bio:  string @lang .
	`)

	// langVals reads every language posting for a @lang entity's forward data key
	// as a langTag→value map, so both runs can be compared without depending on
	// posting order.
	langVals := func(t *testing.T, attr string, entity, readTs uint64) map[string]string {
		t.Helper()
		l, err := GetNoStore(x.DataKey(attr, entity), readTs)
		require.NoError(t, err)
		out := map[string]string{}
		require.NoError(t, l.Iterate(readTs, 0, func(p *pb.Posting) error {
			out[string(p.LangTag)] = string(p.Value)
			return nil
		}))
		return out
	}

	type result struct {
		fwd  map[uint64][]uint64
		rev  map[uint64][]uint64
		lang map[uint64]map[string]string
	}

	collect := func(readTs uint64) result {
		r := result{
			fwd:  map[uint64][]uint64{},
			rev:  map[uint64][]uint64{},
			lang: map[uint64]map[string]string{},
		}
		for i := 0; i < linkCount; i++ {
			src := uint64(1_000_000) + uint64(i)
			r.fwd[src] = sortedUint64(forwardUids(t, linkAttr, src, readTs))
		}
		for tgt := 0; tgt < hotTargets; tgt++ {
			target := uint64(9_000_000) + uint64(tgt)
			r.rev[target] = sortedUint64(reverseUids(t, linkAttr, target, readTs))
		}
		for i := 0; i < bioEntities; i++ {
			e := uint64(5_000_000) + uint64(i)
			r.lang[e] = langVals(t, bioAttr, e, readTs)
		}
		return r
	}

	// Run 1 — budget disabled (serial ProcessList forward write).
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflicts0 := runBudgetBatch(t, 0, 5300, 5301, cloneEdges(edges))
	res0 := collect(5302)

	// Run 2 — budget=30 (both ProcessList predicates parallelized).
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflicts30 := runBudgetBatch(t, 30, 5310, 5311, cloneEdges(edges))
	res30 := collect(5312)

	require.Equal(t, res0.fwd, res30.fwd, "forward uid lists must match")
	require.Equal(t, res0.rev, res30.rev, "reverse lists must match")
	require.Equal(t, res0.lang, res30.lang, "lang values must match")
	require.Equal(t, conflicts0, conflicts30, "conflict-key sets must be identical")
}

// TestAutoBudget exercises the pure AUTO-mode derivation directly, independent of
// the real GOMAXPROCS: machineCap vs workCap interaction, rounding, the budget<2
// fallback, the divide-by-zero guard on a misconfigured min-edges, and
// determinism — no cluster needed.
func TestAutoBudget(t *testing.T) {
	// machineCap dominates: round(96*0.5)=48 < 1_000_000/256=3906.
	require.Equal(t, 48, autoBudget(96, 1_000_000, 0.5, 256))

	// workCap dominates: 2560/256=10 < round(96*0.5)=48.
	require.Equal(t, 10, autoBudget(96, 2560, 0.5, 256))

	// Rounding is half away from zero: round(3*0.5)=round(1.5)=2.
	require.Equal(t, 2, autoBudget(3, 1_000_000, 0.5, 256))

	// budget<2 fallback: round(1*0.5)=1 and 100/256 clamps to 1, so min==1 → the
	// caller routes through allocateWorkers, which returns nil for budget<2 (legacy).
	require.Equal(t, 1, autoBudget(1, 100, 0.5, 256))

	// Divide-by-zero guard: a misconfigured minEdgesPerWorker of 0 must not panic
	// and clamps to 1, so workCap==totalEdges and machineCap (48) wins here.
	require.Equal(t, 48, autoBudget(96, 2560, 0.5, 0))

	// Determinism: identical inputs yield identical outputs.
	require.Equal(t, autoBudget(96, 2560, 0.5, 256), autoBudget(96, 2560, 0.5, 256))
}

// runAutoBudgetBatch is runBudgetBatch's AUTO-mode sibling: it enables AUTO
// (MutationsPipelineGoroutines=-1) with the given fraction / min-edges, runs the
// batch through a fresh pipeline txn, snapshots the conflict-key set, commits, and
// restores all three config fields on return.
func runAutoBudgetBatch(t *testing.T, fraction float64, minEdges int,
	startTs, commitTs uint64, edges []*pb.DirectedEdge) map[uint64]struct{} {
	t.Helper()
	oldBudget := x.WorkerConfig.MutationsPipelineGoroutines
	oldFraction := x.WorkerConfig.MutationsPipelineGoroutinesFraction
	oldMinEdges := x.WorkerConfig.MutationsPipelineMinEdgesPerWorker
	x.WorkerConfig.MutationsPipelineGoroutines = mutationsPipelineGoroutinesAuto
	x.WorkerConfig.MutationsPipelineGoroutinesFraction = fraction
	x.WorkerConfig.MutationsPipelineMinEdgesPerWorker = minEdges
	defer func() {
		x.WorkerConfig.MutationsPipelineGoroutines = oldBudget
		x.WorkerConfig.MutationsPipelineGoroutinesFraction = oldFraction
		x.WorkerConfig.MutationsPipelineMinEdgesPerWorker = oldMinEdges
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

// TestPipelineBudgetAutoByteIdentical proves AUTO mode is byte-identical to a
// fixed budget of the same computed value (and to the disabled legacy path). AUTO
// only chooses the integer fed into allocateWorkers, so for a deterministically
// forced internal budget the downstream behavior must match exactly. The forcing
// is host-independent: fraction=100 makes machineCap=round(GOMAXPROCS*100) >= 100
// on any host, so workCap=totalEdges/minEdges=320/32=10 always wins → AUTO derives
// budget=10. Asserts committed data values, index buckets, forward/reverse lists,
// AND conflict-key sets equal those of budget=0 and a fixed budget=10. Run under
// -race to also cover the parallel data-write/index paths.
func TestPipelineBudgetAutoByteIdentical(t *testing.T) {
	const nameCount = 300 // total batch = 300 name + 10 dept + 10 friend = 320 edges

	nameAttr := x.AttrInRootNamespace("name")
	deptAttr := x.AttrInRootNamespace("dept")
	friendAttr := x.AttrInRootNamespace("friend")
	edges := buildSkewedBatch(nameAttr, deptAttr, friendAttr, nameCount)

	// Sanity: the forced internal budget of 10 grants the dominant predicate more
	// than one worker, so the multi-worker path is genuinely exercised.
	alloc := allocateWorkers(map[string]int{nameAttr: nameCount, deptAttr: 10, friendAttr: 10}, 10)
	require.Greater(t, alloc[nameAttr], 1, "test must exercise multi-worker dominant predicate")
	// Sanity: AUTO with fraction=100 / minEdges=32 derives exactly 10 regardless of
	// the host's GOMAXPROCS (machineCap >= 100, workCap = 320/32 = 10).
	require.Equal(t, 10, autoBudget(1, 320, 100, 32))
	require.Equal(t, 10, autoBudget(256, 320, 100, 32))

	schemaBytes := []byte(`
		name: string @index(exact) .
		dept: string @index(exact) .
		friend: uid @reverse .
	`)

	type result struct {
		nameVal map[uint64]string
		nameIdx map[string][]uint64
		deptIdx map[string][]uint64
		fwd     map[uint64][]uint64
		rev     map[uint64][]uint64
	}

	collect := func(readTs uint64) result {
		r := result{
			nameVal: map[uint64]string{},
			nameIdx: map[string][]uint64{},
			deptIdx: map[string][]uint64{},
			fwd:     map[uint64][]uint64{},
			rev:     map[uint64][]uint64{},
		}
		for i := 0; i < nameCount; i++ {
			e := uint64(1_000_000) + uint64(i)
			r.nameVal[e] = scalarValue(t, nameAttr, e, readTs)
		}
		for v := 0; v < 50; v++ {
			val := fmt.Sprintf("name%d", v)
			r.nameIdx[val] = sortedUint64(indexUidsForVal(t, nameAttr, val, readTs))
		}
		for v := 0; v < 3; v++ {
			val := fmt.Sprintf("dept%d", v)
			r.deptIdx[val] = sortedUint64(indexUidsForVal(t, deptAttr, val, readTs))
		}
		for i := 0; i < 10; i++ {
			src := uint64(3_000_000) + uint64(i)
			r.fwd[src] = sortedUint64(forwardUids(t, friendAttr, src, readTs))
		}
		for tgt := 0; tgt < 4; tgt++ {
			target := uint64(4_000_000) + uint64(tgt)
			r.rev[target] = sortedUint64(reverseUids(t, friendAttr, target, readTs))
		}
		return r
	}

	// Run 1 — budget disabled (legacy one-goroutine-per-predicate).
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflicts0 := runBudgetBatch(t, 0, 5200, 5201, cloneEdges(edges))
	res0 := collect(5202)

	// Run 2 — fixed budget=10.
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflicts10 := runBudgetBatch(t, 10, 5210, 5211, cloneEdges(edges))
	res10 := collect(5212)

	// Run 3 — AUTO, forced to derive budget=10.
	require.NoError(t, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(t, schema.ParseBytes(schemaBytes, 1))
	conflictsAuto := runAutoBudgetBatch(t, 100, 32, 5220, 5221, cloneEdges(edges))
	resAuto := collect(5222)

	require.Equal(t, res0.nameVal, resAuto.nameVal, "scalar data values must match legacy")
	require.Equal(t, res10.nameVal, resAuto.nameVal, "scalar data values must match fixed budget")
	require.Equal(t, res10.nameIdx, resAuto.nameIdx, "name index buckets must match fixed budget")
	require.Equal(t, res10.deptIdx, resAuto.deptIdx, "dept index buckets must match fixed budget")
	require.Equal(t, res10.fwd, resAuto.fwd, "forward uid lists must match fixed budget")
	require.Equal(t, res10.rev, resAuto.rev, "reverse lists must match fixed budget")
	require.Equal(t, conflicts0, conflictsAuto, "conflict-key set must match legacy")
	require.Equal(t, conflicts10, conflictsAuto, "conflict-key set must match fixed budget")
}

// BenchmarkPipelineSkewedBatch measures Process throughput on a ~20,000-edge
// batch dominated by one non-indexed scalar predicate (~99% of edges), comparing
// the budget-off serial data-write against budget=30. The dominant predicate is
// non-indexed so the measurement isolates the data-write parallelization (the
// primary beneficiary). edges/s plus ns/op are reported per sub-benchmark.
func BenchmarkPipelineSkewedBatch(b *testing.B) {
	require.NoError(b, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(b, schema.ParseBytes([]byte(`
		name: string .
		dept: string @index(exact) .
		friend: uid @reverse .
	`), 1))

	nameAttr := x.AttrInRootNamespace("name")
	deptAttr := x.AttrInRootNamespace("dept")
	friendAttr := x.AttrInRootNamespace("friend")
	edges := buildSkewedBatch(nameAttr, deptAttr, friendAttr, 19980) // ~20,000 total

	run := func(b *testing.B, budget int) {
		old := x.WorkerConfig.MutationsPipelineGoroutines
		x.WorkerConfig.MutationsPipelineGoroutines = budget
		defer func() { x.WorkerConfig.MutationsPipelineGoroutines = old }()

		var ts uint64 = 100_000
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ts += 10
			// Fresh, unregistered txn per iteration: a private cache, no oracle
			// bookkeeping to leak across b.N.
			txn := NewTxn(ts)
			mp := NewMutationPipeline(txn)
			if err := mp.Process(context.Background(), cloneEdges(edges)); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		edgesPerSec := float64(len(edges)) * float64(b.N) / b.Elapsed().Seconds()
		b.ReportMetric(edgesPerSec, "edges/s")
	}

	b.Run("budget=0", func(b *testing.B) { run(b, 0) })
	b.Run("budget=30", func(b *testing.B) { run(b, 30) })
}

// BenchmarkPipelineBudgetSweep sweeps the goroutine budget over the same
// ~20,000-edge skewed batch as BenchmarkPipelineSkewedBatch, plus AUTO mode, so
// the throughput plateau can be located on the real (e.g. 96-core) box and the
// AUTO fraction tuned. Each sub-benchmark reports edges/s alongside the default
// ns/op. cloneEdges hands each Process call a fresh slice (Process mutates edge
// ValueId in place). Run with:
//
//	go test -run '^$' -bench '^BenchmarkPipelineBudgetSweep$' -benchmem ./posting/
func BenchmarkPipelineBudgetSweep(b *testing.B) {
	require.NoError(b, pstore.DropAll())
	MemLayerInstance.clear()
	require.NoError(b, schema.ParseBytes([]byte(`
		name: string .
		dept: string @index(exact) .
		friend: uid @reverse .
	`), 1))

	nameAttr := x.AttrInRootNamespace("name")
	deptAttr := x.AttrInRootNamespace("dept")
	friendAttr := x.AttrInRootNamespace("friend")
	edges := buildSkewedBatch(nameAttr, deptAttr, friendAttr, 19980) // ~20,000 total

	// run sets the budget plus the two AUTO tunables (inert unless budget==-1) and
	// restores all three on return. fraction/minEdges only matter for the auto case.
	run := func(b *testing.B, budget int, fraction float64, minEdges int) {
		oldBudget := x.WorkerConfig.MutationsPipelineGoroutines
		oldFraction := x.WorkerConfig.MutationsPipelineGoroutinesFraction
		oldMinEdges := x.WorkerConfig.MutationsPipelineMinEdgesPerWorker
		x.WorkerConfig.MutationsPipelineGoroutines = budget
		x.WorkerConfig.MutationsPipelineGoroutinesFraction = fraction
		x.WorkerConfig.MutationsPipelineMinEdgesPerWorker = minEdges
		defer func() {
			x.WorkerConfig.MutationsPipelineGoroutines = oldBudget
			x.WorkerConfig.MutationsPipelineGoroutinesFraction = oldFraction
			x.WorkerConfig.MutationsPipelineMinEdgesPerWorker = oldMinEdges
		}()

		var ts uint64 = 200_000
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ts += 10
			// Fresh, unregistered txn per iteration: a private cache, no oracle
			// bookkeeping to leak across b.N.
			txn := NewTxn(ts)
			mp := NewMutationPipeline(txn)
			if err := mp.Process(context.Background(), cloneEdges(edges)); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		edgesPerSec := float64(len(edges)) * float64(b.N) / b.Elapsed().Seconds()
		b.ReportMetric(edgesPerSec, "edges/s")
	}

	for _, budget := range []int{0, 8, 16, 24, 32, 48, 64, 96} {
		budget := budget
		b.Run(fmt.Sprintf("budget=%d", budget), func(b *testing.B) { run(b, budget, 0.5, 256) })
	}
	// AUTO uses the production default fraction (1.0); runtime.GOMAXPROCS(0) drives
	// machineCap = round(GOMAXPROCS * 1.0), capped by edges/minEdgesPerWorker.
	b.Run("budget=auto", func(b *testing.B) {
		run(b, mutationsPipelineGoroutinesAuto, 1.0, 256)
	})
}
