/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/v25/dql"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/pkg/errors"
)

// This file implements the pure, I/O-free core of native hybrid search: combining
// several already-scored result sets ("channels") into a single ranked list.
//
// A channel is a uid->score map produced by an upstream ranker bound to a DQL value
// variable (e.g. bm25(...) or similar_to(...)). Fusion is a coordinator-side
// operation over resolved value variables; everything here is deterministic and
// independent of storage, sharding, and the query pipeline so it can be tested in
// isolation. The query-layer adapter (see populateUidValVar) converts value
// variables into fuseChannels and the result back into a value variable.

// fusionMethod selects how channel scores are combined.
type fusionMethod int

const (
	// fusionRRF is Reciprocal Rank Fusion: each channel contributes 1/(k+rank),
	// where rank is the 1-based position of the uid within that channel. Robust to
	// heterogeneous score scales because it uses ranks, not raw scores.
	fusionRRF fusionMethod = iota
	// fusionLinear is a weighted sum of (optionally normalized) raw scores.
	fusionLinear
)

// linearNormalize selects score normalization for fusionLinear.
type linearNormalize int

const (
	// normalizeMax divides each channel's scores by that channel's maximum absolute
	// score and clamps the result to [0,1]. Clamping matters under the union's
	// missing-uid=0 convention: a signed metric (cosine/dot) can retrieve a document
	// with a negative similarity, and without the clamp that document would fuse below
	// a document the channel never retrieved at all (which contributes 0). Clamping
	// negative similarities to 0 makes "retrieved but dissimilar" tie with "not
	// retrieved" instead of ranking beneath it. BM25 and euclidean (1/(1+d)) scores are
	// already >=0, so they are unaffected.
	normalizeMax linearNormalize = iota
	// normalizeNone uses raw scores as-is (the caller asserts comparability).
	normalizeNone
)

// defaultRRFK is the conventional RRF rank constant. Larger k flattens the
// contribution of top ranks; 60 is the widely used default.
const defaultRRFK = 60.0

// fuseChannel is one scored input to fusion. scores maps uid -> raw score with the
// convention that higher is always better (all Dgraph rankers surface
// higher-is-better scores). weight applies only to fusionLinear.
type fuseChannel struct {
	scores map[uint64]float64
	weight float64
}

// fuseOpts configures a fusion run.
type fuseOpts struct {
	method    fusionMethod
	k         float64         // RRF rank constant; <=0 falls back to defaultRRFK.
	normalize linearNormalize // linear only.
	topk      int             // if >0, truncate output to the top topk results.
}

// scoredUid is a uid paired with its fused score.
type scoredUid struct {
	uid   uint64
	score float64
}

// fuseChannels combines channels into a single ranked list, sorted by fused score
// descending and tie-broken by uid ascending. The candidate set is the UNION of all
// channels' uids (outer join): a uid missing from a channel simply receives no
// contribution from it (RRF: as if rank = infinity; linear: as if score = 0). It is
// never dropped and never produces NaN.
func fuseChannels(channels []fuseChannel, opts fuseOpts) []scoredUid {
	var fused map[uint64]float64
	switch opts.method {
	case fusionLinear:
		fused = fuseLinear(channels, opts.normalize)
	default:
		fused = fuseRRF(channels, opts.k)
	}

	out := make([]scoredUid, 0, len(fused))
	for uid, s := range fused {
		out = append(out, scoredUid{uid: uid, score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].uid < out[j].uid
	})
	if opts.topk > 0 && len(out) > opts.topk {
		out = out[:opts.topk]
	}
	return out
}

// channelRanks returns the 1-based rank of every uid in a channel, computed by
// sorting on score descending and tie-breaking by uid ascending. The tie-break
// matches the deterministic ordering used elsewhere in the codebase (bm25/HNSW
// sorted()), so equal-scored uids rank stably by uid.
func channelRanks(c fuseChannel) map[uint64]int {
	uids := make([]uint64, 0, len(c.scores))
	for uid := range c.scores {
		uids = append(uids, uid)
	}
	sort.Slice(uids, func(i, j int) bool {
		si, sj := c.scores[uids[i]], c.scores[uids[j]]
		if si != sj {
			return si > sj
		}
		return uids[i] < uids[j]
	})
	ranks := make(map[uint64]int, len(uids))
	for i, uid := range uids {
		ranks[uid] = i + 1
	}
	return ranks
}

// fuseRRF computes (weighted) Reciprocal Rank Fusion over the channels. Each
// channel contributes weight * 1/(k+rank); with the default weight of 1.0 this is
// standard RRF, and per-channel weights let callers bias channels under either
// fusion method rather than silently ignoring weights for rrf.
func fuseRRF(channels []fuseChannel, k float64) map[uint64]float64 {
	if k <= 0 || math.IsNaN(k) || math.IsInf(k, 0) {
		k = defaultRRFK
	}
	fused := make(map[uint64]float64)
	for _, c := range channels {
		for uid, rank := range channelRanks(c) {
			fused[uid] += c.weight * (1.0 / (k + float64(rank)))
		}
	}
	return fused
}

// channelMaxAbs returns the maximum finite absolute score in a channel, used to
// max-normalize heterogeneous score scales. Non-finite scores are ignored;
// returns 0 for an empty channel (callers treat a 0 denominator as "contribute 0").
func channelMaxAbs(c fuseChannel) float64 {
	var maxAbs float64
	for _, s := range c.scores {
		if math.IsNaN(s) || math.IsInf(s, 0) {
			continue
		}
		if a := math.Abs(s); a > maxAbs {
			maxAbs = a
		}
	}
	return maxAbs
}

// fuseLinear computes a weighted sum of (optionally max-normalized) raw scores. A
// uid missing from a channel contributes 0 from that channel. A channel whose
// maximum absolute score is 0 (all zeros / empty) contributes 0 rather than
// dividing by zero.
func fuseLinear(channels []fuseChannel, normalize linearNormalize) map[uint64]float64 {
	denoms := make([]float64, len(channels))
	for i, c := range channels {
		if normalize == normalizeMax {
			denoms[i] = channelMaxAbs(c)
		} else {
			denoms[i] = 1.0
		}
	}

	fused := make(map[uint64]float64)
	for i, c := range channels {
		denom := denoms[i]
		for uid, s := range c.scores {
			norm := s
			if normalize == normalizeMax {
				if denom == 0 {
					norm = 0
				} else {
					norm = s / denom
				}
				// Clamp negatives to the missing-uid baseline (0) so a retrieved but
				// dissimilar document never fuses below one the channel never retrieved.
				if norm < 0 {
					norm = 0
				}
			}
			fused[uid] += c.weight * norm
		}
	}
	// Ensure uids that appear only in zero-contribution channels are still present
	// in the union (e.g. an all-zero max-normalized channel). The loop above already
	// inserts them with a running sum (possibly 0), so the union is complete.
	return fused
}

// --- Query-layer adapter -----------------------------------------------------
//
// The functions below bridge the pure fusion core to DQL value variables. They
// parse the fuse() options, read each channel's scores from the already-resolved
// variable map, run fusion, and return a varValue carrying both the union uid set
// and the fused uid->score map (the same shape the bm25 ranker binds).

// parseFuseOpts extracts fuse() options from the function's key/value arg pairs and
// returns the resolved fuseOpts plus the optional per-channel linear weights
// (nil when unspecified). numChannels is used to validate the weights count.
func parseFuseOpts(args []dql.Arg, numChannels int) (fuseOpts, []float64, error) {
	opts := fuseOpts{method: fusionRRF, k: defaultRRFK, normalize: normalizeMax}
	var weights []float64

	for i := 0; i+1 < len(args); i += 2 {
		key := strings.ToLower(args[i].Value)
		val := args[i+1].Value
		switch key {
		case "method":
			switch strings.ToLower(val) {
			case "rrf":
				opts.method = fusionRRF
			case "linear":
				opts.method = fusionLinear
			default:
				return opts, nil, errors.Errorf("fuse: unknown method %q (want rrf or linear)", val)
			}
		case "k":
			k, err := strconv.ParseFloat(val, 64)
			if err != nil || k <= 0 || math.IsNaN(k) || math.IsInf(k, 0) {
				return opts, nil, errors.Errorf("fuse: k must be a positive finite number, got %q", val)
			}
			opts.k = k
		case "normalize":
			switch strings.ToLower(val) {
			case "max":
				opts.normalize = normalizeMax
			case "none":
				opts.normalize = normalizeNone
			default:
				return opts, nil, errors.Errorf("fuse: unknown normalize %q (want max or none)", val)
			}
		case "topk":
			tk, err := strconv.Atoi(val)
			if err != nil || tk < 0 {
				return opts, nil, errors.Errorf("fuse: topk must be a non-negative integer, got %q", val)
			}
			opts.topk = tk
		case "weights":
			parts := strings.Split(val, ",")
			weights = make([]float64, 0, len(parts))
			for _, p := range parts {
				w, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
				if err != nil || math.IsNaN(w) || math.IsInf(w, 0) {
					return opts, nil, errors.Errorf("fuse: invalid weight %q", p)
				}
				weights = append(weights, w)
			}
		default:
			return opts, nil, errors.Errorf("fuse: unknown option %q", key)
		}
	}

	if weights != nil && len(weights) != numChannels {
		return opts, nil, errors.Errorf("fuse: weights count (%d) must match channel count (%d)",
			len(weights), numChannels)
	}
	return opts, weights, nil
}

// computeFuse reads the channel value variables named in the fuse function's
// NeedsVar from doneVars, runs fusion, and returns a varValue with the union uid
// set and the fused uid->score map.
func computeFuse(args []dql.Arg, needsVar []dql.VarContext,
	doneVars map[string]varValue, sgPath []*SubGraph) (varValue, error) {

	if len(needsVar) == 0 {
		return varValue{}, errors.Errorf("fuse: requires at least one value variable channel")
	}

	opts, weights, err := parseFuseOpts(args, len(needsVar))
	if err != nil {
		return varValue{}, err
	}

	channels := make([]fuseChannel, len(needsVar))
	for i, nv := range needsVar {
		v, ok := doneVars[nv.Name]
		switch {
		case !ok:
			// The dependency scheduler guarantees every channel block has run and
			// populated doneVars before this fuse block. A genuinely absent channel
			// therefore signals an internal invariant violation rather than an empty
			// result — surface it instead of silently degrading the fusion.
			return varValue{}, errors.Errorf("fuse: channel %q was not produced", nv.Name)
		// Note: ShardedMap.IsEmpty() only detects nil (a fresh NewShardedMap has 30
		// empty shards), so entry-count emptiness must use Len() == 0.
		case v.Vals == nil || v.Vals.Len() == 0:
			// A uid variable carries matched uids but no scores. Treating it as an
			// empty channel would silently drop its uids from the fused ranking, so
			// reject it explicitly — fusion needs scored channels.
			if v.Uids != nil && len(v.Uids.GetUids()) > 0 {
				return varValue{}, errors.Errorf("fuse: channel %q is a uid variable "+
					"without scores; use a ranker such as bm25 or similar_to", nv.Name)
			}
			// A channel that ran but matched nothing is a valid empty channel: it
			// contributes nothing but must not drop the other channels' results.
			channels[i] = fuseChannel{scores: map[uint64]float64{}, weight: 1.0}
		default:
			scores, err := scoresFromVar(v, nv.Name)
			if err != nil {
				return varValue{}, err
			}
			channels[i] = fuseChannel{scores: scores, weight: 1.0}
		}
		if weights != nil {
			channels[i].weight = weights[i]
		}
	}

	fused := fuseChannels(channels, opts)

	out := varValue{Vals: types.NewShardedMap(), path: sgPath}
	uids := make([]uint64, len(fused))
	for i, r := range fused {
		uids[i] = r.uid
		out.Vals.Set(r.uid, types.Val{Tid: types.FloatID, Value: r.score})
	}
	// Emit the uid set in ascending order — the Dgraph value-variable contract (the
	// same one bm25 follows): Uids is the unordered candidate set used by uid(var),
	// and the fused score lives in Vals. Callers recover ranked order with
	// `orderdesc: val(var)`. When `topk` is set, fuseChannels has already selected
	// the top-k by fused score before this ascending sort, so top-k + orderdesc is
	// correct. (Sorting here by score would break uid(var) set semantics.)
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	out.Uids = &pb.List{Uids: uids}
	return out, nil
}

// scoresFromVar extracts a uid->float64 score map from a value variable. The
// variable must carry numeric scores (as bm25/similar_to bind); a uid variable
// without scores cannot be a fusion channel.
func scoresFromVar(v varValue, name string) (map[uint64]float64, error) {
	scores := make(map[uint64]float64, v.Vals.Len())
	var convErr error
	err := v.Vals.Iterate(func(uid uint64, val types.Val) error {
		var s float64
		// bm25 and similar_to bind FloatID scores directly; take that fast path so a
		// non-finite value (which types.Convert rejects) is dropped rather than
		// mis-reported as "non-numeric". Other numeric types go through Convert.
		if val.Tid == types.FloatID {
			f, ok := val.Value.(float64)
			if !ok {
				convErr = errors.Errorf("fuse: channel %q has a malformed score value", name)
				return convErr
			}
			s = f
		} else {
			f, err := types.Convert(val, types.FloatID)
			if err != nil {
				convErr = errors.Errorf("fuse: channel %q has non-numeric scores; use a "+
					"ranker such as bm25 or similar_to", name)
				return convErr
			}
			s = f.Value.(float64)
		}
		// Drop non-finite scores so they can never poison fusion: a NaN/Inf would
		// break the sort comparator's strict-weak-ordering and propagate through
		// linear sums. A uid dropped here simply doesn't participate via this channel.
		if math.IsNaN(s) || math.IsInf(s, 0) {
			return nil
		}
		scores[uid] = s
		return nil
	})
	if convErr != nil {
		return nil, convErr
	}
	if err != nil {
		return nil, err
	}
	return scores, nil
}
