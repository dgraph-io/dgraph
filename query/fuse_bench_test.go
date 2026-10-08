/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"fmt"
	"math/rand"
	"testing"
)

// Q32 fusion-overhead benchmarks (pure unit level, no cluster).
//
// fuseChannels makes three to four full passes over the channel union with no
// early bound: channelRanks sorts every channel (RRF), fuseChannels materializes
// and sorts the full union, and topk truncation happens only AFTER the full
// sort. These benchmarks quantify that cost across channel count, channel size,
// overlap, fusion method, and topk, plus an isolation benchmark for
// channelRanks (the rank-map allocation is the suspected hotspot).
//
// Corpora are deterministic: every channel set is generated from a constant
// rand seed, so numbers are comparable across runs.

const fuseBenchSeed = 42

// buildFuseBenchChannels builds numChannels channels of `size` uids each, where
// a fraction `overlap` of every channel's uids is drawn from a pool shared by
// ALL channels and the remainder is unique to that channel. Scores are uniform
// random in (0, 10). The union cardinality is therefore
// shared + numChannels*(size-shared).
func buildFuseBenchChannels(numChannels, size int, overlap float64) []fuseChannel {
	rng := rand.New(rand.NewSource(fuseBenchSeed))
	shared := int(float64(size) * overlap)

	// Shared pool: uids 1..shared (uid 0 is invalid in Dgraph).
	sharedUids := make([]uint64, shared)
	for i := range sharedUids {
		sharedUids[i] = uint64(i + 1)
	}

	channels := make([]fuseChannel, numChannels)
	for ci := 0; ci < numChannels; ci++ {
		scores := make(map[uint64]float64, size)
		for _, uid := range sharedUids {
			scores[uid] = rng.Float64() * 10
		}
		// Unique uids live in a per-channel range far above the shared pool.
		base := uint64(ci+1) * 1_000_000_000
		for i := 0; i < size-shared; i++ {
			scores[base+uint64(i)] = rng.Float64() * 10
		}
		channels[ci] = fuseChannel{scores: scores, weight: 1.0}
	}
	return channels
}

// fuseBenchUnionSize computes the union cardinality of the channels, counted in
// the harness because fuseChannels exposes no counter.
func fuseBenchUnionSize(channels []fuseChannel) int {
	union := make(map[uint64]struct{})
	for _, c := range channels {
		for uid := range c.scores {
			union[uid] = struct{}{}
		}
	}
	return len(union)
}

// BenchmarkFuseChannels is the Q32 matrix: 2/4/8 channels x 1k/10k/100k uids
// per channel x 10%/90% overlap x rrf vs linear(normalizeMax) x topk 10 vs 0
// (unbounded). union_uids/op is the fused candidate-set cardinality;
// fused_out/op is the emitted result length (post-topk).
func BenchmarkFuseChannels(b *testing.B) {
	methods := []struct {
		name string
		opts fuseOpts
	}{
		{"rrf", fuseOpts{method: fusionRRF, k: defaultRRFK}},
		{"linmax", fuseOpts{method: fusionLinear, normalize: normalizeMax}},
	}

	for _, numChannels := range []int{2, 4, 8} {
		for _, size := range []int{1000, 10000, 100000} {
			for _, overlap := range []float64{0.10, 0.90} {
				channels := buildFuseBenchChannels(numChannels, size, overlap)
				union := fuseBenchUnionSize(channels)
				for _, m := range methods {
					for _, topk := range []int{10, 0} {
						opts := m.opts
						opts.topk = topk
						name := fmt.Sprintf("ch=%d/sz=%d/ov=%d/%s/topk=%d",
							numChannels, size, int(overlap*100), m.name, topk)
						b.Run(name, func(b *testing.B) {
							b.ReportAllocs()
							var res []scoredUid
							for i := 0; i < b.N; i++ {
								res = fuseChannels(channels, opts)
							}
							b.ReportMetric(float64(union), "union_uids/op")
							b.ReportMetric(float64(len(res)), "fused_out/op")
						})
					}
				}
			}
		}
	}
}

// BenchmarkChannelOrder isolates the per-channel rank computation used by RRF:
// one full sort of the channel's uids plus a fresh map[uint64]int allocation
// per call (the suspected allocation hotspot inside fuseRRF's channel loop).
func BenchmarkChannelOrder(b *testing.B) {
	for _, size := range []int{1000, 10000, 100000} {
		channels := buildFuseBenchChannels(1, size, 0)
		c := channels[0]
		b.Run(fmt.Sprintf("sz=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			var order []uint64
			for i := 0; i < b.N; i++ {
				order = channelOrder(c)
			}
			b.ReportMetric(float64(len(order)), "ranks/op")
		})
	}
}
