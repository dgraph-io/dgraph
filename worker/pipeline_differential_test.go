/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/types/facets"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Pipeline-vs-legacy differential.
//
// Apply the SAME edges through the legacy ordered path and the mutation
// pipeline (forced fan-out) on a freshly wiped store, then compare the entire
// committed key-space. requireSame asserts the two paths agree.
//
// Below the harness: a broad fuzz gate (TestPipelineLegacyDifferential) plus
// one deterministic, minimal case per divergence class the fuzz surfaced. These
// are permanent correctness tests that must stay in the suite; each failing
// case must go green once its class is fixed, and the pipeline must not be made
// default-on while any of them is red.
// ---------------------------------------------------------------------------

type diffH struct {
	t  *testing.T
	ps *badger.DB
	ts uint64
}

// snapshot is one path's committed state. data holds every data/index/reverse/
// count posting (facets included). vec reduces the HNSW graph keys to "which
// uids are in the vector index": the entry point and per-level adjacency depend
// on insertion order and random level draws, so byte-comparing them would
// produce false diffs between two correct runs.
type snapshot struct {
	data map[string]string
	vec  map[string]bool
}

func newDiffH(t *testing.T) *diffH {
	dir, err := os.MkdirTemp("", "pipediff_")
	require.NoError(t, err)
	ps, err := badger.OpenManaged(badger.DefaultOptions(dir).WithLoggingLevel(badger.ERROR))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close(); _ = os.RemoveAll(dir) })

	posting.Init(ps, 0, false)
	Init(ps)
	require.NoError(t, schema.ParseBytes([]byte(`
		sv:  string        @index(exact) .
		lv:  [uid]         @reverse .
		lc:  [uid]         @reverse @count .
		lp:  string        @lang .
		li:  string        @lang @index(exact) .
		ls:  [string]      @index(term) .
		lf:  [string]      @index(fulltext) .
		lt:  [string]      @index(trigram) .
		geo: geo           @index(geo) .
		bf:  bigfloat      @index(bigfloat) .
		bo:  bool          @index(bool) .
		fl:  float         @index(float) .
		pw:  password .
		vec: float32vector @index(hnsw(metric:"euclidean")) .
		su:  uid           @reverse .
		fs:  string        @index(exact) .
		fu:  [uid] .
		fr:  [uid]         @reverse .
	`), 1))
	prevPar := x.WorkerConfig.IntraMutationParallelism
	prevEPW := x.WorkerConfig.IntraMutationEdgesPerWorker
	t.Cleanup(func() {
		x.WorkerConfig.IntraMutationParallelism = prevPar
		x.WorkerConfig.IntraMutationEdgesPerWorker = prevEPW
	})
	x.WorkerConfig.IntraMutationParallelism = x.IntraMutationParallelism{Workers: 8}
	x.WorkerConfig.IntraMutationEdgesPerWorker = 1
	return &diffH{t: t, ps: ps, ts: 10}
}

func (h *diffH) A(p string) string { return x.AttrInRootNamespace(p) }

func (h *diffH) dump() snapshot {
	s := snapshot{data: map[string]string{}, vec: map[string]bool{}}
	pwAttr := h.A("pw")
	txn := h.ps.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Rewind(); it.Valid(); it.Next() {
		key := it.Item().KeyCopy(nil)
		pk, err := x.Parse(key)
		if err != nil || (!pk.IsData() && !pk.IsIndex() && !pk.IsReverse() && !pk.IsCountOrCountRev()) {
			continue
		}
		if strings.Contains(pk.Attr, hnsw.VecKeyword) {
			if strings.HasSuffix(pk.Attr, hnsw.VecEntry) {
				continue
			}
			if pk.IsData() {
				s.vec[fmt.Sprintf("%d", pk.Uid)] = true
			}
			continue
		}
		pl, err := posting.GetNoStore(key, math.MaxUint64)
		require.NoError(h.t, err)
		var parts []string
		require.NoError(h.t, pl.Iterate(math.MaxUint64, 0, func(p *pb.Posting) error {
			v := fmt.Sprintf("%x", p.Value)
			if pk.Attr == pwAttr {
				v = "PWHASH" // bcrypt is salted; the plaintext is verified separately
			}
			parts = append(parts, fmt.Sprintf("u%d:v%s:l%s:t%d:f%s",
				p.Uid, v, p.LangTag, p.PostingType, fmtFacets(p.Facets)))
			return nil
		}))
		sort.Strings(parts)
		if len(parts) > 0 {
			s.data[fmt.Sprintf("%x", key)] = fmt.Sprintf("%v", parts)
		}
	}
	return s
}

func fmtFacets(fs []*api.Facet) string {
	if len(fs) == 0 {
		return "-"
	}
	var s []string
	for _, f := range fs {
		s = append(s, fmt.Sprintf("%s=%d:%x", f.Key, f.ValType, f.Value))
	}
	sort.Strings(s)
	return strings.Join(s, ",")
}

func cloneEdges(in []*pb.DirectedEdge) []*pb.DirectedEdge {
	out := make([]*pb.DirectedEdge, len(in))
	for i, e := range in {
		c := &pb.DirectedEdge{
			Entity:    e.Entity,
			Attr:      e.Attr,
			Value:     append([]byte(nil), e.Value...),
			ValueId:   e.ValueId,
			ValueType: e.ValueType,
			Lang:      e.Lang,
			Op:        e.Op,
		}
		for _, f := range e.Facets {
			c.Facets = append(c.Facets, &api.Facet{
				Key:     f.Key,
				Value:   append([]byte(nil), f.Value...),
				ValType: f.ValType,
				Tokens:  append([]string(nil), f.Tokens...),
				Alias:   f.Alias,
			})
		}
		out[i] = c
	}
	return out
}

// apply wipes the store, applies the batches in order through the chosen path
// (committing each one, so later batches act on committed data) and snapshots.
func (h *diffH) apply(pipeline bool, batches ...[]*pb.DirectedEdge) snapshot {
	require.NoError(h.t, h.ps.DropAll())
	posting.ResetCache()
	posting.Oracle().ResetTxns()
	for _, edges := range batches {
		startTs, commitTs := h.ts, h.ts+1
		h.ts += 10
		ctx := schema.GetWriteContext(context.Background())
		txn := posting.Oracle().RegisterStartTs(startTs)
		if pipeline {
			require.NoError(h.t, posting.NewMutationPipeline(txn).Process(ctx, cloneEdges(edges)))
		} else {
			es := cloneEdges(edges)
			sort.SliceStable(es, func(i, j int) bool {
				if es[i].Attr != es[j].Attr {
					return es[i].Attr < es[j].Attr
				}
				return es[i].Entity < es[j].Entity
			})
			for _, e := range es {
				require.NoError(h.t, runMutation(ctx, e, txn))
			}
		}
		txn.Update()
		w := posting.NewTxnWriter(h.ps)
		require.NoError(h.t, txn.CommitToDisk(w, commitTs))
		require.NoError(h.t, w.Flush())
		txn.UpdateCachedKeys(commitTs)
	}
	return h.dump()
}

// requireSame fails if the pipeline's committed state differs from legacy's.
// With several batches, later ones act on data the earlier ones committed.
func (h *diffH) requireSame(batches ...[]*pb.DirectedEdge) {
	legacy := h.apply(false, batches...)
	pipe := h.apply(true, batches...)
	var all []*pb.DirectedEdge
	for _, b := range batches {
		all = append(all, b...)
	}
	require.Equal(h.t, legacy.data, pipe.data, "pipeline diverged from legacy\n  edges: %s", fmtEdges(all))
	require.Equal(h.t, legacy.vec, pipe.vec,
		"pipeline diverged from legacy (vector-index membership)\n  edges: %s", fmtEdges(all))
}

// caseSame runs one case as an isolated subtest with the subtest's *testing.T,
// so a failing case cannot abort its siblings.
func (h *diffH) caseSame(t *testing.T, name string, batches ...[]*pb.DirectedEdge) {
	t.Run(name, func(t *testing.T) {
		hh := *h
		hh.t = t
		defer func() { h.ts = hh.ts }()
		hh.requireSame(batches...)
	})
}

// ---- edge builders ----

func eSetStr(attr string, uid uint64, v string) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(v), ValueType: pb.Posting_STRING, Op: pb.DirectedEdge_SET}
}
func eDelStr(attr string, uid uint64, v string) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(v), ValueType: pb.Posting_STRING, Op: pb.DirectedEdge_DEL}
}
func eSetUid(attr string, uid, tgt uint64) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, ValueId: tgt, ValueType: pb.Posting_UID, Op: pb.DirectedEdge_SET}
}
func eLangSet(attr string, uid uint64, v, lang string) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(v), ValueType: pb.Posting_STRING, Lang: lang, Op: pb.DirectedEdge_SET}
}
func eLangDel(attr string, uid uint64, v, lang string) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(v), ValueType: pb.Posting_STRING, Lang: lang, Op: pb.DirectedEdge_DEL}
}
func eStarDel(attr string, uid uint64) *pb.DirectedEdge {
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: []byte(x.Star), Op: pb.DirectedEdge_DEL}
}

func fmtEdges(edges []*pb.DirectedEdge) string {
	var b []string
	for _, e := range edges {
		op := "SET"
		if e.Op == pb.DirectedEdge_DEL {
			op = "DEL"
		}
		v := string(e.Value)
		if e.ValueType == pb.Posting_UID {
			v = fmt.Sprintf("uid(%d)", e.ValueId)
		}
		lang := ""
		if e.Lang != "" {
			lang = "@" + e.Lang
		}
		b = append(b, fmt.Sprintf("%s %s e=%d v=%q%s", op, e.Attr, e.Entity, v, lang))
	}
	return fmt.Sprintf("%v", b)
}

// TestPipelineLegacyDifferential is the broad fuzz gate (discovery + acceptance)
// over every non-vector shape in the schema, star-deletes included, so every
// batch mixes legacy-routed and batched predicates. Vectors have their own fuzz
// with a membership compare, and password is excluded because bcrypt makes each
// seed ~100ms.
func TestPipelineLegacyDifferential(t *testing.T) {
	h := newDiffH(t)
	uids := []uint64{0x1, 0x2, 0x3, 0x4}
	strs := []string{"a", "b", "c", "d"}
	tgts := []uint64{0x100, 0x101, 0x102}
	langs := []string{"en", "de", "fr"}
	words := []string{"red apple", "green apple", "blue pear", "apple"}
	tris := []string{"apple", "zebra", "maple", "ape"}
	geos := []string{geoP1, geoP2}
	bfs := []string{"1.5", "2.25", "-3.125"}
	fls := []string{"0.5", "1.5", "2.5"}
	fks := [][2]string{{"w", "1"}, {"w", "2"}, {"tag", `"x"`}}
	starPreds := []string{"sv", "lv", "lc", "lp", "li", "ls", "lf", "lt", "geo", "bf", "bo", "fl", "su", "fs", "fu", "fr"}
	gen := func(rng *rand.Rand) []*pb.DirectedEdge {
		n := 3 + rng.Intn(8)
		var edges []*pb.DirectedEdge
		pick := func(ss []string) string { return ss[rng.Intn(len(ss))] }
		tgt := func() uint64 { return tgts[rng.Intn(len(tgts))] }
		for i := 0; i < n; i++ {
			uid := uids[rng.Intn(len(uids))]
			del := rng.Intn(2) == 1
			var e *pb.DirectedEdge
			switch rng.Intn(17) {
			case 0:
				e = eSetStr(h.A("sv"), uid, pick(strs))
			case 1:
				e = eSetUid(h.A("lv"), uid, tgt())
			case 2:
				e = eSetUid(h.A("lc"), uid, tgt())
			case 3:
				e = eLangSet(h.A("lp"), uid, pick(strs), pick(langs))
			case 4:
				e = eSetStr(h.A("ls"), uid, pick(strs))
			case 5:
				e = eSetStr(h.A("lf"), uid, pick(words))
			case 6:
				e = eSetStr(h.A("lt"), uid, pick(tris))
			case 7:
				e = eSetStr(h.A("geo"), uid, pick(geos))
			case 8:
				e = eSetStr(h.A("bf"), uid, pick(bfs))
			case 9:
				e = eSetStr(h.A("bo"), uid, pick([]string{"true", "false"}))
			case 10:
				e = eSetStr(h.A("fl"), uid, pick(fls))
			case 11:
				e = eSetUid(h.A("su"), uid, tgt())
			case 12:
				e = eSetStr(h.A("fs"), uid, pick(strs[:2]))
			case 13:
				e = eSetUid(h.A("fu"), uid, tgt())
			case 14:
				e = eSetUid(h.A("fr"), uid, tgt())
			case 15:
				e = eLangSet(h.A("li"), uid, pick(strs), pick(langs))
			case 16:
				e = eStarDel(h.A(pick(starPreds)), uid)
			}
			switch {
			case e.Op == pb.DirectedEdge_DEL: // star-delete
			case del:
				e.Op = pb.DirectedEdge_DEL
			default:
				if a := x.ParseAttr(e.Attr); a == "fs" || a == "fu" || a == "fr" {
					fk := fks[rng.Intn(len(fks))]
					e = withF(t, e, fk[0], fk[1])
				}
			}
			edges = append(edges, e)
		}
		return edges
	}
	const iters = 2000
	mism := 0
	byPred := map[string]int{}
	for seed := 0; seed < iters; seed++ {
		rng := rand.New(rand.NewSource(int64(seed) + 1))
		edges := gen(rng)
		legacy := h.apply(false, edges)
		pipe := h.apply(true, edges)
		if fmt.Sprintf("%v", legacy.data) != fmt.Sprintf("%v", pipe.data) {
			mism++
			diff := symDiff(legacy.data, pipe.data)
			for k := range diff {
				if pk, err := x.Parse(mustHex(k)); err == nil {
					byPred[x.ParseAttr(pk.Attr)]++
				}
			}
			if mism <= 5 { // a few concrete examples make the tally actionable
				t.Logf("seed %d diverged: %s", seed, fmtEdges(edges))
				for k := range diff {
					t.Logf("  key %s\n    legacy:   %s\n    pipeline: %s", k, legacy.data[k], pipe.data[k])
				}
			}
		}
	}
	t.Logf("fuzz differential: %d/%d seeds diverged; by predicate: %v", mism, iters, byPred)
	require.Zero(t, mism, "pipeline must match legacy")
}

func symDiff(a, b map[string]string) map[string]bool {
	out := map[string]bool{}
	for k, v := range a {
		if b[k] != v {
			out[k] = true
		}
	}
	for k, v := range b {
		if a[k] != v {
			out[k] = true
		}
	}
	return out
}

func mustHex(s string) []byte {
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		fmt.Sscanf(s[i*2:i*2+2], "%02x", &out[i])
	}
	return out
}

// Divergence class 1 — star-delete reorder. A SET followed by a star-delete on
// the same (pred,entity) in one proposal. The pipeline runs the delete inline
// and the set via a worker afterward, so the value (and its index) survive when
// they should be cleared.
func TestPipeline_StarDeleteThenSet(t *testing.T) {
	h := newDiffH(t)
	sv := h.A("sv")
	h.requireSame([]*pb.DirectedEdge{
		eSetStr(sv, 1, "a"),
		eStarDel(sv, 1), // after this, sv(1) must be empty
	})
}

// Star-deletes route their predicate through the legacy per-edge path for the
// batch (Process pre-pass -> PredicatePipeline.legacy), which preserves order
// against the SETs on the same list while the other predicates in the mutation
// keep the batched path.
func TestPipeline_StarDeleteRouting(t *testing.T) {
	h := newDiffH(t)
	sv, lv, lc, lp, ls, geo, v := h.A("sv"), h.A("lv"), h.A("lc"), h.A("lp"), h.A("ls"), h.A("geo"), h.A("vec")
	h.caseSame(t, "star-then-set", batch(eStarDel(sv, 1), eSetStr(sv, 1, "a"))) // the replace pattern keeps the SET
	h.caseSame(t, "interleaved",
		batch(eSetStr(sv, 1, "a"), eStarDel(sv, 1), eSetStr(sv, 1, "b"), eStarDel(sv, 1), eSetStr(sv, 1, "c")))
	h.caseSame(t, "committed+set+star", batch(eSetStr(sv, 1, "a")), batch(eSetStr(sv, 1, "b"), eStarDel(sv, 1)))
	h.caseSame(t, "committed+star+set", batch(eSetStr(sv, 1, "a")), batch(eStarDel(sv, 1), eSetStr(sv, 1, "b")))
	h.caseSame(t, "two-entities-one-star", batch(eSetStr(sv, 1, "a"), eSetStr(sv, 2, "b"), eStarDel(sv, 1)))
	h.caseSame(t, "reverse-list", batch(eSetUid(lv, 1, 0x100), eSetUid(lv, 1, 0x101), eStarDel(lv, 1)))
	h.caseSame(t, "reverse-list-committed",
		batch(eSetUid(lv, 1, 0x100), eSetUid(lv, 2, 0x100)), batch(eStarDel(lv, 1), eSetUid(lv, 1, 0x102)))
	h.caseSame(t, "count-list", batch(eSetUid(lc, 1, 0x100), eStarDel(lc, 1), eSetUid(lc, 1, 0x101)))
	h.caseSame(t, "lang", batch(eLangSet(lp, 1, "b", "en"), eLangSet(lp, 1, "c", "de"), eStarDel(lp, 1), eLangSet(lp, 1, "d", "fr")))
	h.caseSame(t, "indexed-list", batch(eSetStr(ls, 1, "c"), eStarDel(ls, 1), eSetStr(ls, 1, "d")))
	h.caseSame(t, "vector", batch(eSetStr(v, 1, "[1,0,0]"), eStarDel(v, 1), eSetStr(v, 1, "[0,1,0]")))
	// sv (star) and ls (indexed list) are legacy-routed; lv and geo stay batched.
	h.caseSame(t, "mixed-mode-batch", batch(
		eSetStr(sv, 1, "a"), eStarDel(sv, 1), eSetStr(sv, 1, "b"),
		eSetUid(lv, 2, 0x100), eSetUid(lv, 2, 0x101),
		eSetStr(geo, 3, geoP1),
		eSetStr(ls, 4, "x"), eDelStr(ls, 4, "x"), eSetStr(ls, 4, "y"),
	))
}

// Divergence class 2 — @count with repeated/multiple SETs changing a target's
// in-degree. The batched ProcessCount and legacy disagreed, and the evidence
// points at LEGACY's count index being the stale side; that is unresolved.
// @count predicates take the legacy path for now (routeToLegacy), so this case
// passes by construction and the open question is parked: whether legacy's
// count is right needs a semantic (not differential) check before @count is
// promoted back to the batched path.
func TestPipeline_CountDuplicateSets(t *testing.T) {
	h := newDiffH(t)
	lc := h.A("lc")
	h.requireSame([]*pb.DirectedEdge{
		eSetUid(lc, 1, 0x100),
		eSetUid(lc, 1, 0x100), // duplicate source->target
		eSetUid(lc, 3, 0x100), // target 0x100 in-degree 2
		eSetUid(lc, 3, 0x101), // target 0x101 in-degree 1
	})
}

// Divergence class 3 — @lang. ProcessList keys a scalar @lang slot by
// fingerprint(lang) and insertPosting is last-write-wins on that slot, so a
// value-specific delete used to land regardless of the value it named: it
// dropped a DIFFERENT value under the same tag, including an already committed
// one. Legacy applies such a delete only when value and type match
// (addMutationHelper); ProcessList now mirrors that (langDeleteMismatch).
func TestPipeline_LangValueDelete(t *testing.T) {
	h := newDiffH(t)
	lp, li := h.A("lp"), h.A("li")
	h.caseSame(t, "in-batch/nonmatching-del-other-tag-set", batch(
		eLangSet(lp, 1, "b", "en"),
		eLangSet(lp, 1, "c", "de"),
		eLangDel(lp, 1, "c", "en"), // deletes nothing (en is "b"); de must remain
	))
	h.caseSame(t, "in-batch/set-set-del-old",
		batch(eLangSet(lp, 1, "b", "en"), eLangSet(lp, 1, "c", "en"), eLangDel(lp, 1, "b", "en"))) // en stays "c"
	h.caseSame(t, "in-batch/matching-del", batch(eLangSet(lp, 1, "b", "en"), eLangDel(lp, 1, "b", "en")))
	h.caseSame(t, "committed/nonmatching-del", batch(eLangSet(lp, 1, "b", "en")), batch(eLangDel(lp, 1, "c", "en"))) // used to wipe "b"
	h.caseSame(t, "committed/matching-del", batch(eLangSet(lp, 1, "b", "en")), batch(eLangDel(lp, 1, "b", "en")))
	h.caseSame(t, "committed/double-del",
		batch(eLangSet(lp, 1, "b", "en")), batch(eLangDel(lp, 1, "b", "en"), eLangDel(lp, 1, "b", "en")))
	h.caseSame(t, "committed/del-then-set-same-tag",
		batch(eLangSet(lp, 1, "b", "en")), batch(eLangDel(lp, 1, "b", "en"), eLangSet(lp, 1, "c", "en")))
	h.caseSame(t, "committed/multi-tag-mixed",
		batch(eLangSet(lp, 1, "b", "en"), eLangSet(lp, 1, "c", "de"), eLangSet(lp, 1, "d", "fr")),
		batch(eLangDel(lp, 1, "x", "en"), eLangDel(lp, 1, "c", "de"), eLangSet(lp, 1, "e", "fr"))) // en kept, de gone, fr replaced
	// With an index the ignored delete must not emit an index Del either.
	h.caseSame(t, "indexed/committed-nonmatching-del", batch(eLangSet(li, 1, "b", "en")), batch(eLangDel(li, 1, "c", "en")))
	h.caseSame(t, "indexed/committed-matching-del", batch(eLangSet(li, 1, "b", "en")), batch(eLangDel(li, 1, "b", "en")))
	h.caseSame(t, "indexed/in-batch-churn",
		batch(eLangSet(li, 1, "b", "en"), eLangDel(li, 1, "zz", "en"), eLangSet(li, 1, "c", "de")))
	// Value-indexed @lang additionally takes the legacy path (routeToLegacy): the
	// batched index pass cannot retract an in-batch overwritten tag value, so
	// "b" would keep its index token here. Found by the fuzz (seed 432).
	h.caseSame(t, "indexed/in-batch-same-tag-replace", batch(eLangSet(li, 1, "b", "de"), eLangSet(li, 1, "a", "de")))
}

// Divergence class 4 — value-indexed list. SET/DELETE churn on a
// [string] @index(term) left a stale term in the index on the batched path,
// also on committed data. Value-indexed lists take the legacy path for now
// (routeToLegacy); these cases guard that routing until the batched path nets
// in-batch set/delete correctly.
func TestPipeline_IndexedListValueChurn(t *testing.T) {
	h := newDiffH(t)
	ls := h.A("ls")
	h.caseSame(t, "in-batch", batch(eSetStr(ls, 1, "c"), eDelStr(ls, 1, "c"), eSetStr(ls, 1, "d"))) // "c" must leave the index
	h.caseSame(t, "committed+churn", batch(eSetStr(ls, 1, "c")), batch(eSetStr(ls, 1, "c"), eDelStr(ls, 1, "c")))
	h.caseSame(t, "committed+del-old-set-new", batch(eSetStr(ls, 1, "c")), batch(eDelStr(ls, 1, "c"), eSetStr(ls, 1, "d")))
}

// ---- shape coverage: builders ----

const (
	geoP1 = `{"type":"Point","coordinates":[1.0,2.0]}`
	geoP2 = `{"type":"Point","coordinates":[3.5,-4.25]}`
)

func batch(edges ...*pb.DirectedEdge) []*pb.DirectedEdge { return edges }

// withF attaches facets built by facets.FacetFor: bare numbers parse as ints,
// string values must be quoted (e.g. `"x"`).
func withF(t *testing.T, e *pb.DirectedEdge, kv ...string) *pb.DirectedEdge {
	for i := 0; i+1 < len(kv); i += 2 {
		f, err := facets.FacetFor(kv[i], kv[i+1])
		require.NoError(t, err)
		e.Facets = append(e.Facets, f)
	}
	return e
}

// eSetVecTyped pre-converts the vector to VFLOAT binary — what typed RDF
// ("..."^^<xs:[]float32>) delivers via chunker/rdf_parser.go — so
// ValidateAndConvert is a no-op on both paths. Untyped input (JSON mutations,
// RDF without the type) arrives as a STRING/DEFAULT literal; eSetStr covers it.
func eSetVecTyped(t *testing.T, attr string, uid uint64, s string) *pb.DirectedEdge {
	dst, err := types.Convert(types.Val{Tid: types.StringID, Value: []byte(s)}, types.VFloatID)
	require.NoError(t, err)
	b := types.ValueForType(types.BinaryID)
	require.NoError(t, types.Marshal(dst, &b))
	return &pb.DirectedEdge{Entity: uid, Attr: attr, Value: b.Value.([]byte),
		ValueType: pb.Posting_VFLOAT, Op: pb.DirectedEdge_SET}
}

// Divergence class 4 on other tokenizers. Same mechanism as
// TestPipeline_IndexedListValueChurn: a deleted value's tokens survive in the
// index. Fulltext keeps the shared token "appl" correct (the case the
// Dels-before-Sets sort in InsertTokenizerIndexes was written for) but leaks
// the deleted value's own token "red". The trigram values share no trigrams, so
// stale tokens cannot hide behind legitimate ones.
func TestPipeline_IndexedListValueChurnFulltext(t *testing.T) {
	h := newDiffH(t)
	lf := h.A("lf")
	h.caseSame(t, "churn-shared-token",
		batch(eSetStr(lf, 1, "red apple"), eDelStr(lf, 1, "red apple"), eSetStr(lf, 1, "green apple")))
	h.caseSame(t, "committed+churn",
		batch(eSetStr(lf, 1, "red apple")), batch(eSetStr(lf, 1, "red apple"), eDelStr(lf, 1, "red apple")))
	h.caseSame(t, "del-old-set-new-shared-token",
		batch(eSetStr(lf, 1, "red apple")), batch(eDelStr(lf, 1, "red apple"), eSetStr(lf, 1, "green apple")))
}

func TestPipeline_IndexedListValueChurnTrigram(t *testing.T) {
	h := newDiffH(t)
	lt := h.A("lt")
	h.caseSame(t, "churn", batch(eSetStr(lt, 1, "apple"), eDelStr(lt, 1, "apple"), eSetStr(lt, 1, "zebra")))
	h.caseSame(t, "committed+churn",
		batch(eSetStr(lt, 1, "apple")), batch(eSetStr(lt, 1, "apple"), eDelStr(lt, 1, "apple")))
}

// Vector values that arrive untyped (JSON mutations, RDF without
// ^^<xs:[]float32>) must be converted to VFLOAT binary by the pipeline exactly
// as runMutation does. ProcessVectorIndex used to skip ValidateAndConvert and
// stored the literal text, which also left the vector out of the HNSW index.
func TestPipeline_VectorUntypedValue(t *testing.T) {
	h := newDiffH(t)
	v := h.A("vec")
	h.caseSame(t, "single-set", batch(eSetStr(v, 1, "[1.0, 0.0, 0.0]")))
	h.caseSame(t, "set-replace-del", batch(
		eSetStr(v, 1, "[1.0, 0.0, 0.0]"),
		eSetStr(v, 1, "[0.0, 1.0, 0.0]"), // replace
		eSetStr(v, 2, "[0.0, 0.0, 1.0]"),
		eDelStr(v, 2, "[0.0, 0.0, 1.0]"), // matching delete
	))
	h.caseSame(t, "committed+replace",
		batch(eSetStr(v, 1, "[1.0, 0.0, 0.0]")), batch(eSetStr(v, 1, "[0.5, 0.5, 0.0]")))
	h.caseSame(t, "committed+star",
		batch(eSetStr(v, 1, "[1.0, 0.0, 0.0]"), eSetStr(v, 2, "[0.0, 1.0, 0.0]")), batch(eStarDel(v, 1)))
}

// TestPipelineVectorDifferential fuzzes untyped vector SET/DEL mixes and
// compares vector data exactly and HNSW membership (not adjacency).
func TestPipelineVectorDifferential(t *testing.T) {
	h := newDiffH(t)
	v := h.A("vec")
	vecs := []string{"[1,0,0]", "[0,1,0]", "[0,0,1]", "[0.5,0.5,0]", "[0.2,0.3,0.9]"}
	const iters = 200
	mism := 0
	for seed := 0; seed < iters; seed++ {
		rng := rand.New(rand.NewSource(int64(seed) + 1))
		n := 2 + rng.Intn(6)
		var edges []*pb.DirectedEdge
		for i := 0; i < n; i++ {
			e := eSetStr(v, uint64(1+rng.Intn(4)), vecs[rng.Intn(len(vecs))])
			if rng.Intn(4) == 0 {
				e.Op = pb.DirectedEdge_DEL
			}
			edges = append(edges, e)
		}
		legacy := h.apply(false, edges)
		pipe := h.apply(true, edges)
		if fmt.Sprintf("%v", legacy.data) != fmt.Sprintf("%v", pipe.data) ||
			fmt.Sprintf("%v", legacy.vec) != fmt.Sprintf("%v", pipe.vec) {
			mism++
		}
	}
	t.Logf("vector fuzz: %d/%d seeds diverged", mism, iters)
	require.Zero(t, mism, "pipeline must match legacy on vector data and index membership")
}

// TestPipelineShapes covers the predicate shapes the differential found clean,
// in-batch and against committed data, so they stay guarded.
func TestPipelineShapes(t *testing.T) {
	h := newDiffH(t)
	g, b, bo, fl, pw, su := h.A("geo"), h.A("bf"), h.A("bo"), h.A("fl"), h.A("pw"), h.A("su")
	fs, fu, fr, v := h.A("fs"), h.A("fu"), h.A("fr"), h.A("vec")

	h.caseSame(t, "Geo/set-replace-del", batch(eSetStr(g, 1, geoP1), eSetStr(g, 1, geoP2), eDelStr(g, 1, geoP2)))
	h.caseSame(t, "Geo/committed+nonmatching-del", batch(eSetStr(g, 1, geoP1)), batch(eDelStr(g, 1, geoP2)))
	h.caseSame(t, "Geo/committed+star", batch(eSetStr(g, 1, geoP1)), batch(eStarDel(g, 1)))

	h.caseSame(t, "BigFloat/set-replace-del", batch(eSetStr(b, 1, "1.5"), eSetStr(b, 1, "2.25"), eDelStr(b, 1, "2.25")))
	h.caseSame(t, "BigFloat/committed+nonmatching-del", batch(eSetStr(b, 1, "1.5")), batch(eDelStr(b, 1, "9.9")))

	h.caseSame(t, "BoolFloat/in-batch", batch(
		eSetStr(bo, 1, "true"), eSetStr(bo, 1, "false"), eDelStr(bo, 1, "false"),
		eSetStr(fl, 2, "1.5"), eSetStr(fl, 2, "2.5"), eDelStr(fl, 2, "1.5"), // non-matching del
	))
	h.caseSame(t, "BoolFloat/committed+nonmatching-del", batch(eSetStr(fl, 2, "1.5")), batch(eDelStr(fl, 2, "7.5")))

	// Password: bcrypt is salted, so the dump masks the hash; verify the
	// plaintext against what each path stored. Passwords must be >= 6 chars.
	for _, pipeline := range []bool{false, true} {
		t.Run(fmt.Sprintf("Password/verify-plaintext/pipeline=%v", pipeline), func(t *testing.T) {
			hh := *h
			hh.t = t
			hh.apply(pipeline, batch(eSetStr(pw, 1, "first1")), batch(eSetStr(pw, 1, "second")))
			h.ts = hh.ts
			pl, err := posting.GetNoStore(x.DataKey(pw, 1), math.MaxUint64)
			require.NoError(t, err)
			n := 0
			require.NoError(t, pl.Iterate(math.MaxUint64, 0, func(p *pb.Posting) error {
				n++
				return types.VerifyPassword("second", string(p.Value))
			}))
			require.Equal(t, 1, n, "expected exactly one password posting")
		})
	}
	h.caseSame(t, "Password/structure-replace", batch(eSetStr(pw, 1, "alpha1")), batch(eSetStr(pw, 1, "bravo2")))
	h.caseSame(t, "Password/committed+star", batch(eSetStr(pw, 1, "alpha1")), batch(eStarDel(pw, 1)))

	suDel := eSetUid(su, 1, 0x100)
	suDel.Op = pb.DirectedEdge_DEL
	h.caseSame(t, "SingleUidReverse/replace", batch(eSetUid(su, 1, 0x100), eSetUid(su, 1, 0x101))) // reverse must move
	h.caseSame(t, "SingleUidReverse/committed+replace", batch(eSetUid(su, 1, 0x100)), batch(eSetUid(su, 1, 0x101)))
	h.caseSame(t, "SingleUidReverse/committed+star", batch(eSetUid(su, 1, 0x100)), batch(eStarDel(su, 1)))
	h.caseSame(t, "SingleUidReverse/committed+matching-del", batch(eSetUid(su, 1, 0x100)), batch(suDel))

	h.caseSame(t, "Facets/scalar", batch(
		withF(t, eSetStr(fs, 1, "a"), "w", "1"),
		withF(t, eSetStr(fs, 1, "a"), "w", "2"), // same value, facet update
		withF(t, eSetStr(fs, 2, "b"), "tag", `"x"`, "n", "7"),
		eDelStr(fs, 2, "b"),
	))
	h.caseSame(t, "Facets/scalar-committed+facet-update",
		batch(withF(t, eSetStr(fs, 1, "a"), "w", "1")), batch(withF(t, eSetStr(fs, 1, "a"), "w", "2")))
	h.caseSame(t, "Facets/uid-list", batch(
		withF(t, eSetUid(fu, 1, 0x100), "since", "2020"),
		withF(t, eSetUid(fu, 1, 0x100), "since", "2021"), // facet replace on same target
		withF(t, eSetUid(fu, 1, 0x101), "since", "2022"),
		eSetUid(fu, 1, 0x102), // no facets
	))
	h.caseSame(t, "Facets/uid-list-committed+facet-update",
		batch(withF(t, eSetUid(fu, 1, 0x100), "since", "2020")), batch(withF(t, eSetUid(fu, 1, 0x100), "since", "2021")))
	h.caseSame(t, "Facets/uid-list-reverse", batch(
		withF(t, eSetUid(fr, 1, 0x100), "since", "2020"),
		withF(t, eSetUid(fr, 2, 0x100), "since", "2021"),
		withF(t, eSetUid(fr, 1, 0x100), "since", "2022"), // facet replace; reverse posting must follow
	))
	h.caseSame(t, "Facets/uid-list-reverse-committed+facet-update",
		batch(withF(t, eSetUid(fr, 1, 0x100), "since", "2020")), batch(withF(t, eSetUid(fr, 1, 0x100), "since", "2021")))

	vDel := eSetVecTyped(t, v, 2, "[0.0, 0.0, 1.0]")
	vDel.Op = pb.DirectedEdge_DEL
	h.caseSame(t, "VectorTyped/single-set", batch(eSetVecTyped(t, v, 1, "[1.0, 0.0, 0.0]")))
	h.caseSame(t, "VectorTyped/set-replace", batch(
		eSetVecTyped(t, v, 1, "[1.0, 0.0, 0.0]"), eSetVecTyped(t, v, 1, "[0.0, 1.0, 0.0]"), eSetVecTyped(t, v, 2, "[0.0, 0.0, 1.0]")))
	h.caseSame(t, "VectorTyped/committed+replace",
		batch(eSetVecTyped(t, v, 1, "[1.0, 0.0, 0.0]")), batch(eSetVecTyped(t, v, 1, "[0.5, 0.5, 0.0]")))
	h.caseSame(t, "VectorTyped/committed+star",
		batch(eSetVecTyped(t, v, 1, "[1.0, 0.0, 0.0]"), eSetVecTyped(t, v, 2, "[0.0, 1.0, 0.0]")), batch(eStarDel(v, 1)))
	h.caseSame(t, "VectorTyped/committed+matching-del", batch(eSetVecTyped(t, v, 2, "[0.0, 0.0, 1.0]")), batch(vDel))
}
