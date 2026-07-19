/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/dql"
	"github.com/dgraph-io/dgraph/v25/types"
)

// ch is a small helper to build a fusion channel from a uid->score map with a
// default weight of 1.0.
func ch(scores map[uint64]float64) fuseChannel {
	return fuseChannel{scores: scores, weight: 1.0}
}

// asMap collapses a fused result slice into a uid->score map for assertions that
// don't care about ordering.
func asMap(res []scoredUid) map[uint64]float64 {
	m := make(map[uint64]float64, len(res))
	for _, r := range res {
		m[r.uid] = r.score
	}
	return m
}

func TestFuseRRF_BasicRanks(t *testing.T) {
	// Channel A order: 10, 20, 30  (ranks 1,2,3)
	// Channel B order: 30, 10, 40  (ranks 1,2,3)
	a := ch(map[uint64]float64{10: 9.0, 20: 5.0, 30: 1.0})
	b := ch(map[uint64]float64{30: 0.9, 10: 0.5, 40: 0.1})

	res := fuseChannels([]fuseChannel{a, b}, fuseOpts{method: fusionRRF, k: 60})
	got := asMap(res)

	const k = 60.0
	// uid 10: rank1 in A, rank2 in B
	require.InDelta(t, 1/(k+1)+1/(k+2), got[10], 1e-9)
	// uid 20: rank2 in A only
	require.InDelta(t, 1/(k+2), got[20], 1e-9)
	// uid 30: rank3 in A, rank1 in B
	require.InDelta(t, 1/(k+3)+1/(k+1), got[30], 1e-9)
	// uid 40: rank3 in B only
	require.InDelta(t, 1/(k+3), got[40], 1e-9)
}

func TestFuseRRF_OrderingAndUnion(t *testing.T) {
	a := ch(map[uint64]float64{10: 9.0, 20: 5.0, 30: 1.0})
	b := ch(map[uint64]float64{30: 0.9, 10: 0.5, 40: 0.1})

	res := fuseChannels([]fuseChannel{a, b}, fuseOpts{method: fusionRRF, k: 60})

	// Union of all uids is present (outer join, not intersection).
	require.Len(t, res, 4)
	// Sorted by fused score descending. uid 10 and 30 both appear in both channels
	// near the top; 10 is rank1+rank2, 30 is rank3+rank1 -> 10 slightly higher.
	require.Equal(t, uint64(10), res[0].uid)
	require.Equal(t, uint64(30), res[1].uid)
	// Scores must be monotonically non-increasing.
	for i := 1; i < len(res); i++ {
		require.LessOrEqual(t, res[i].score, res[i-1].score)
	}
}

func TestFuseRRF_DefaultK(t *testing.T) {
	a := ch(map[uint64]float64{1: 1.0})
	// k<=0 should fall back to the default of 60.
	res := fuseChannels([]fuseChannel{a}, fuseOpts{method: fusionRRF, k: 0})
	require.InDelta(t, 1/(60.0+1), res[0].score, 1e-9)
}

func TestFuseRRF_TieBreakByUidAscending(t *testing.T) {
	// Equal scores within a channel -> lower uid gets the better (smaller) rank.
	a := ch(map[uint64]float64{2: 5.0, 1: 5.0, 3: 5.0})
	res := fuseChannels([]fuseChannel{a}, fuseOpts{method: fusionRRF, k: 60})
	got := asMap(res)
	// uid 1 rank1, uid 2 rank2, uid 3 rank3.
	require.InDelta(t, 1/(60.0+1), got[1], 1e-9)
	require.InDelta(t, 1/(60.0+2), got[2], 1e-9)
	require.InDelta(t, 1/(60.0+3), got[3], 1e-9)
	// Final output tie-broken by uid ascending when fused scores are equal.
	require.Equal(t, uint64(1), res[0].uid)
}

func TestFuseRRF_DisjointChannels(t *testing.T) {
	a := ch(map[uint64]float64{1: 9.0, 2: 8.0})
	b := ch(map[uint64]float64{3: 9.0, 4: 8.0})
	res := fuseChannels([]fuseChannel{a, b}, fuseOpts{method: fusionRRF, k: 60})
	require.Len(t, res, 4)
	got := asMap(res)
	// Each uid scored only by its single channel rank.
	require.InDelta(t, 1/(60.0+1), got[1], 1e-9)
	require.InDelta(t, 1/(60.0+1), got[3], 1e-9)
	require.InDelta(t, 1/(60.0+2), got[2], 1e-9)
	require.InDelta(t, 1/(60.0+2), got[4], 1e-9)
}

func TestFuseRRF_AppliesWeights(t *testing.T) {
	// Weights must affect RRF (not only linear): a uid ranked #1 in a 2x-weighted
	// channel should beat a uid ranked #1 in a unit-weighted channel.
	heavy := fuseChannel{scores: map[uint64]float64{1: 9.0}, weight: 2.0}
	light := fuseChannel{scores: map[uint64]float64{2: 9.0}, weight: 1.0}
	res := fuseChannels([]fuseChannel{heavy, light}, fuseOpts{method: fusionRRF, k: 60})
	got := asMap(res)
	require.InDelta(t, 2.0*(1/(60.0+1)), got[1], 1e-9)
	require.InDelta(t, 1.0*(1/(60.0+1)), got[2], 1e-9)
	require.Equal(t, uint64(1), res[0].uid, "heavier-weighted channel's top doc wins")
}

func TestFuseRRF_DefaultWeightIsStandardRRF(t *testing.T) {
	// With the default weight of 1.0, weighted RRF reduces to standard RRF.
	a := ch(map[uint64]float64{10: 9.0, 20: 5.0})
	res := fuseChannels([]fuseChannel{a}, fuseOpts{method: fusionRRF, k: 60})
	got := asMap(res)
	require.InDelta(t, 1/(60.0+1), got[10], 1e-9)
	require.InDelta(t, 1/(60.0+2), got[20], 1e-9)
}

func TestFuseLinear_MaxNormalizeAndWeights(t *testing.T) {
	// BM25-ish scale vs cosine-ish scale.
	text := fuseChannel{scores: map[uint64]float64{1: 10.0, 2: 5.0}, weight: 0.3}
	vec := fuseChannel{scores: map[uint64]float64{1: 0.8, 2: 0.4, 3: 0.2}, weight: 0.7}

	res := fuseChannels([]fuseChannel{text, vec},
		fuseOpts{method: fusionLinear, normalize: normalizeMax})
	got := asMap(res)

	// max-normalize: text/10, vec/0.8.
	// uid1: 0.3*(10/10) + 0.7*(0.8/0.8) = 0.3 + 0.7 = 1.0
	require.InDelta(t, 1.0, got[1], 1e-9)
	// uid2: 0.3*(5/10) + 0.7*(0.4/0.8) = 0.15 + 0.35 = 0.5
	require.InDelta(t, 0.5, got[2], 1e-9)
	// uid3: only vec: 0.7*(0.2/0.8) = 0.175 (text contributes 0, not NaN)
	require.InDelta(t, 0.175, got[3], 1e-9)

	require.Equal(t, uint64(1), res[0].uid)
}

func TestFuseLinear_NoNormalize(t *testing.T) {
	a := fuseChannel{scores: map[uint64]float64{1: 2.0, 2: 1.0}, weight: 1.0}
	b := fuseChannel{scores: map[uint64]float64{1: 3.0}, weight: 2.0}
	res := fuseChannels([]fuseChannel{a, b}, fuseOpts{method: fusionLinear, normalize: normalizeNone})
	got := asMap(res)
	// uid1: 1*2 + 2*3 = 8 ; uid2: 1*1 = 1
	require.InDelta(t, 8.0, got[1], 1e-9)
	require.InDelta(t, 1.0, got[2], 1e-9)
}

func TestFuseLinear_ZeroMaxChannelContributesZero(t *testing.T) {
	// A channel whose scores are all zero must not divide-by-zero / NaN.
	a := fuseChannel{scores: map[uint64]float64{1: 0.0, 2: 0.0}, weight: 1.0}
	b := fuseChannel{scores: map[uint64]float64{1: 4.0}, weight: 1.0}
	res := fuseChannels([]fuseChannel{a, b}, fuseOpts{method: fusionLinear, normalize: normalizeMax})
	got := asMap(res)
	require.False(t, math.IsNaN(got[1]))
	require.False(t, math.IsNaN(got[2]))
	// uid1: a contributes 0, b contributes 4/4=1 -> 1.0
	require.InDelta(t, 1.0, got[1], 1e-9)
	// uid2: only in a (all-zero) -> 0.0
	require.InDelta(t, 0.0, got[2], 1e-9)
}

func TestFuseLinear_NegativeSimilarityNotBelowMissing(t *testing.T) {
	// A signed vector channel (cosine/dot) can retrieve a document with negative
	// similarity. Under the union's missing-uid=0 convention, that document must not
	// fuse BELOW a document the channel never retrieved (which contributes 0). uid3 is
	// retrieved by vec with a negative cosine; uid2 is absent from vec entirely.
	text := fuseChannel{scores: map[uint64]float64{1: 10.0}, weight: 1.0}
	vec := fuseChannel{scores: map[uint64]float64{1: 0.9, 3: -0.4}, weight: 1.0}

	res := fuseChannels([]fuseChannel{text, vec},
		fuseOpts{method: fusionLinear, normalize: normalizeMax})
	got := asMap(res)

	// uid3's negative similarity is clamped to 0, so it ties the missing baseline
	// rather than going negative.
	require.InDelta(t, 0.0, got[3], 1e-9)
	// uid2 is absent from every channel, so it is not in the union at all.
	require.NotContains(t, got, uint64(2))
	// A retrieved-but-dissimilar doc (uid3, score 0) is never ranked below a doc that
	// would only appear via a negative contribution — no fused score is negative.
	for uid, s := range got {
		require.GreaterOrEqual(t, s, 0.0, "uid %d fused below the missing baseline", uid)
	}
}

func TestFuse_TopKTruncation(t *testing.T) {
	a := ch(map[uint64]float64{1: 9, 2: 8, 3: 7, 4: 6, 5: 5})
	res := fuseChannels([]fuseChannel{a}, fuseOpts{method: fusionRRF, k: 60, topk: 3})
	require.Len(t, res, 3)
	require.Equal(t, uint64(1), res[0].uid)
	require.Equal(t, uint64(3), res[2].uid)
}

func TestFuse_SingleChannelPassthroughOrder(t *testing.T) {
	a := ch(map[uint64]float64{1: 1, 2: 9, 3: 5})
	res := fuseChannels([]fuseChannel{a}, fuseOpts{method: fusionRRF, k: 60})
	// Order should reflect channel ranking: 2 (rank1), 3 (rank2), 1 (rank3).
	require.Equal(t, []uint64{2, 3, 1}, []uint64{res[0].uid, res[1].uid, res[2].uid})
}

func TestFuse_EmptyChannels(t *testing.T) {
	res := fuseChannels([]fuseChannel{ch(nil), ch(map[uint64]float64{})},
		fuseOpts{method: fusionRRF, k: 60})
	require.Empty(t, res)
}

func TestScoresFromVar_DropsNonFinite(t *testing.T) {
	// Non-finite scores from a channel must be dropped so they can't break the sort
	// comparator or poison linear sums.
	m := types.NewShardedMap()
	m.Set(1, types.Val{Tid: types.FloatID, Value: 0.5})
	m.Set(2, types.Val{Tid: types.FloatID, Value: math.NaN()})
	m.Set(3, types.Val{Tid: types.FloatID, Value: math.Inf(1)})
	m.Set(4, types.Val{Tid: types.FloatID, Value: math.Inf(-1)})
	m.Set(5, types.Val{Tid: types.FloatID, Value: 2.0})

	scores, err := scoresFromVar(varValue{Vals: m}, "ch")
	require.NoError(t, err)
	require.Len(t, scores, 2, "only finite scores should survive")
	require.Contains(t, scores, uint64(1))
	require.Contains(t, scores, uint64(5))
	require.NotContains(t, scores, uint64(2))
	require.NotContains(t, scores, uint64(3))
	require.NotContains(t, scores, uint64(4))
}

func TestFuseLinear_NonFiniteChannelDoesNotPoison(t *testing.T) {
	// Even if a NaN slips into a channel passed directly to the core, max-normalize
	// must not produce a NaN denominator that propagates.
	bad := fuseChannel{scores: map[uint64]float64{1: math.NaN(), 2: math.NaN()}, weight: 1.0}
	good := fuseChannel{scores: map[uint64]float64{1: 4.0}, weight: 1.0}
	res := fuseChannels([]fuseChannel{bad, good}, fuseOpts{method: fusionLinear, normalize: normalizeMax})
	for _, r := range res {
		require.False(t, math.IsNaN(r.score), "uid %d score must not be NaN", r.uid)
	}
}

func TestFuse_Determinism(t *testing.T) {
	a := ch(map[uint64]float64{1: 5, 2: 5, 3: 5})
	b := ch(map[uint64]float64{3: 1, 2: 1, 1: 1})
	first := fuseChannels([]fuseChannel{a, b}, fuseOpts{method: fusionRRF, k: 60})
	for i := 0; i < 20; i++ {
		again := fuseChannels([]fuseChannel{a, b}, fuseOpts{method: fusionRRF, k: 60})
		require.Equal(t, first, again)
	}
}

func TestFuseLinear_WindowDependentNormalization(t *testing.T) {
	// SEMANTICS-OPEN (Q9): max-normalization divides by channelMaxAbs computed over
	// whatever scores the channel var happened to retrieve (its window), not over any
	// global corpus maximum — no global max exists anywhere in the pipeline. The SAME
	// raw score for the SAME uid therefore normalizes differently depending on which
	// other documents share its retrieval window (e.g. a bm25 channel with first:N vs
	// unbounded). This test demonstrates and pins that window dependence; whether the
	// denominator should instead be window-invariant is an open design decision.
	const raw = 2.0

	// Window 1: the channel also retrieved a doc scoring 10.0, so max is 10.0.
	wide := fuseChannel{scores: map[uint64]float64{1: 10.0, 2: raw}, weight: 1.0}
	res := fuseChannels([]fuseChannel{wide},
		fuseOpts{method: fusionLinear, normalize: normalizeMax})
	require.InDelta(t, 0.2, asMap(res)[2], 1e-9, "raw 2.0 under window max 10.0")

	// Window 2 (e.g. a deeper offset window): raw 2.0 is now the window max.
	narrow := fuseChannel{scores: map[uint64]float64{2: raw, 3: 1.0}, weight: 1.0}
	res = fuseChannels([]fuseChannel{narrow},
		fuseOpts{method: fusionLinear, normalize: normalizeMax})
	require.InDelta(t, 1.0, asMap(res)[2], 1e-9, "same raw 2.0 under window max 2.0")
}

func TestFuseRRF_MassTiePlateau(t *testing.T) {
	// SEMANTICS-OPEN (Q9): channelRanks assigns 1-based POSITIONAL ranks with a uid
	// tie-break, so a plateau of identical scores receives ranks 1..n and RRF
	// contributions ranging from 1/(k+1) down to 1/(k+n) — equal-relevance documents
	// get very different fused scores purely by uid order. Dense ranking (all tied
	// docs sharing one rank) is not implemented; this pins the positional choice
	// until that decision is made.
	const n = 1000
	scores := make(map[uint64]float64, n)
	for uid := uint64(1); uid <= n; uid++ {
		scores[uid] = 5.0
	}
	res := fuseChannels([]fuseChannel{ch(scores)}, fuseOpts{method: fusionRRF, k: 60})
	require.Len(t, res, n)
	got := asMap(res)

	const k = 60.0
	require.InDelta(t, 1/(k+1), got[1], 1e-12, "first tied uid gets rank 1")
	require.InDelta(t, 1/(k+n), got[n], 1e-12, "last tied uid gets rank n")
	// The spread across a single tie plateau: (k+n)/(k+1) = 1060/61 ≈ 17.4x.
	require.Greater(t, got[1]/got[n], 10.0,
		"positional ranking spreads identical-score docs by >10x under k=60")
	// Output order across the plateau is uid ascending (rank order == uid order).
	require.Equal(t, uint64(1), res[0].uid)
	require.Equal(t, uint64(n), res[n-1].uid)
}

func TestParseFuseOpts_NegativeWeights(t *testing.T) {
	// SEMANTICS-OPEN (Q9): parseFuseOpts rejects only NaN/Inf weights — negative and
	// zero weights are ACCEPTED today, even though fuseLinear's clamp-then-weight
	// order means negative weights produce fused scores outside any documented
	// range (see TestFuseLinear_NegativeWeightOutput). Whether to reject them is an
	// open decision; this pins the current accept behavior.
	args := func(spec string) []dql.Arg {
		return []dql.Arg{{Value: "weights"}, {Value: spec}}
	}

	_, weights, err := parseFuseOpts(args("-1,1"), 2)
	require.NoError(t, err, "negative weight is currently accepted")
	require.Equal(t, []float64{-1, 1}, weights)

	_, weights, err = parseFuseOpts(args("0,1"), 2)
	require.NoError(t, err, "zero weight is currently accepted")
	require.Equal(t, []float64{0, 1}, weights)

	// Non-finite weights are rejected (strconv.ParseFloat accepts these spellings,
	// so the explicit IsNaN/IsInf guard is what fires).
	for _, bad := range []string{"NaN,1", "Inf,1", "+Inf,1", "-Infinity,1"} {
		_, _, err := parseFuseOpts(args(bad), 2)
		require.Error(t, err, "weight spec %q must be rejected", bad)
		require.Contains(t, err.Error(), "invalid weight")
	}
}

func TestFuseLinear_NegativeWeightOutput(t *testing.T) {
	// SEMANTICS-OPEN (Q9): with an accepted negative weight, fuseLinear emits
	// negative fused scores — outside the [0, sum(weights)] range that max-normalized
	// non-negative weights would guarantee. Pin the exact arithmetic so any future
	// rejection (or re-ranged) fix consciously changes this test.
	a := fuseChannel{scores: map[uint64]float64{1: 4.0, 2: 2.0}, weight: -1.0}
	b := fuseChannel{scores: map[uint64]float64{2: 3.0}, weight: 1.0}
	res := fuseChannels([]fuseChannel{a, b},
		fuseOpts{method: fusionLinear, normalize: normalizeMax})
	got := asMap(res)

	// uid1: -1*(4/4) = -1.0 (a negative fused score escapes into the ranking).
	require.InDelta(t, -1.0, got[1], 1e-9)
	// uid2: -1*(2/4) + 1*(3/3) = 0.5
	require.InDelta(t, 0.5, got[2], 1e-9)
	require.Equal(t, uint64(2), res[0].uid, "negatively-fused doc sinks below the positive one")
}

func TestFuseLinear_ClampBeforeWeight(t *testing.T) {
	// Pin (Q9): the negative-similarity clamp applies to the NORMALIZED score BEFORE
	// the channel weight multiplies in. A negative raw score under a negative weight
	// must therefore contribute exactly 0 (clamp(-0.5)=0, then -1*0=0) — never a
	// positive contribution, which weighting-before-clamping would produce
	// (-1 * -0.5 = +0.5).
	c := fuseChannel{scores: map[uint64]float64{1: -0.5, 2: 1.0}, weight: -1.0}
	res := fuseChannels([]fuseChannel{c},
		fuseOpts{method: fusionLinear, normalize: normalizeMax})
	got := asMap(res)

	require.InDelta(t, 0.0, got[1], 1e-9, "clamped-to-zero norm stays 0 under any weight")
	require.InDelta(t, -1.0, got[2], 1e-9, "positive norm 1.0 weighted by -1")
}

func TestFuseRRF_WeightSemanticsDeviateFromStandardRRF(t *testing.T) {
	// INTENTIONAL DEVIATION (Q9): standard RRF (Cormack et al. 2009) is the
	// unweighted sum over channels of 1/(k+rank). This implementation multiplies each
	// channel's reciprocal-rank term by that channel's weight — weight * 1/(k+rank) —
	// so that fuse(..., weights:) biases channels under method:rrf instead of
	// silently ignoring the option. With the default weight 1.0 it reduces to
	// standard RRF (see TestFuseRRF_DefaultWeightIsStandardRRF). Pin the exact
	// weighted math on a case standard RRF would score as a dead tie.
	a := fuseChannel{scores: map[uint64]float64{1: 9.0, 2: 5.0}, weight: 0.25}
	b := fuseChannel{scores: map[uint64]float64{2: 9.0, 1: 5.0}, weight: 4.0}
	res := fuseChannels([]fuseChannel{a, b}, fuseOpts{method: fusionRRF, k: 60})
	got := asMap(res)

	const k = 60.0
	// uid1: rank1 in a, rank2 in b. uid2: mirror image.
	require.InDelta(t, 0.25/(k+1)+4.0/(k+2), got[1], 1e-12)
	require.InDelta(t, 0.25/(k+2)+4.0/(k+1), got[2], 1e-12)
	// Standard (unweighted) RRF would tie uid1 and uid2 exactly; the weights break
	// the tie in favor of the heavier channel's top document.
	require.Equal(t, uint64(2), res[0].uid)
	require.Greater(t, got[2], got[1])
}
