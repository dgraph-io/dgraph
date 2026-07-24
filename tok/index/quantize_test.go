/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package index

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func randVec(rng *rand.Rand, dim int) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = float32(rng.NormFloat64())
	}
	return v
}

func exactSqL2(a, b []float32) float64 {
	var s float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		s += d * d
	}
	return s
}

func TestQuantizeRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, dim := range []int{1, 3, 8, 128, 384, 768} {
		v := randVec(rng, dim)
		blob := QuantizeFloat32(v)
		require.Equal(t, QuantizedLen(dim), len(blob))
		require.Equal(t, dim, QuantizedDim(blob))

		var got []float32
		require.NoError(t, DequantizeFloat32(blob, &got))
		require.Equal(t, dim, len(got))

		// Per-component reconstruction error is bounded by step/2 = range/510.
		lo, hi := v[0], v[0]
		for _, x := range v {
			lo, hi = float32(math.Min(float64(lo), float64(x))), float32(math.Max(float64(hi), float64(x)))
		}
		tol := (hi-lo)/510.0 + 1e-6
		for i := range v {
			require.LessOrEqual(t, math.Abs(float64(v[i]-got[i])), float64(tol),
				"dim=%d i=%d v=%f got=%f", dim, i, v[i], got[i])
		}
	}
}

func TestQuantizeEdgeCases(t *testing.T) {
	// empty
	require.Nil(t, QuantizeFloat32(nil))
	require.Nil(t, QuantizeFloat32([]float32{}))
	var out []float32
	require.NoError(t, DequantizeFloat32(nil, &out))
	require.Empty(t, out)

	// constant vector -> step 0, dequant returns the constant
	blob := QuantizeFloat32([]float32{2.5, 2.5, 2.5})
	require.NoError(t, DequantizeFloat32(blob, &out))
	require.Equal(t, []float32{2.5, 2.5, 2.5}, out)

	// malformed blob
	_, _, _, err := quantParams([]byte{1, 2, 3})
	require.ErrorIs(t, err, ErrInvalidQuantBlob)
	_, err = AsymSquaredL2Float32([]float32{1, 2}, blob) // dim mismatch (blob dim 3)
	require.ErrorIs(t, err, ErrInvalidQuantBlob)
}

func TestQuantizeSanitizeAndHeader(t *testing.T) {
	// NaN/Inf are sanitized at encode time so they never reach a distance.
	v := []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1)), 1.0}
	blob := QuantizeFloat32(v)
	require.Equal(t, 4, QuantizedDim(blob))
	var out []float32
	require.NoError(t, DequantizeFloat32(blob, &out))
	for _, x := range out {
		require.False(t, math.IsNaN(float64(x)) || math.IsInf(float64(x), 0), "got non-finite %v", x)
	}
	// A distance may legitimately overflow to +Inf for absurd (~MaxFloat32)
	// inputs, but it must never be NaN — NaN would break HNSW heap ordering,
	// which is the whole reason we sanitize.
	q := []float32{0, 0, 0, 0}
	d, err := AsymSquaredL2Float32(q, blob)
	require.NoError(t, err)
	require.False(t, math.IsNaN(float64(d)), "distance must never be NaN")

	// Header validation: bad magic / version / codec / truncated all rejected.
	good := QuantizeFloat32([]float32{1, 2, 3})
	bad := append([]byte{}, good...)
	bad[0] = 0x00 // wrong magic
	_, _, _, err = quantParams(bad)
	require.ErrorIs(t, err, ErrInvalidQuantBlob)
	bad = append([]byte{}, good...)
	bad[1] = 9 // wrong version
	_, _, _, err = quantParams(bad)
	require.ErrorIs(t, err, ErrInvalidQuantBlob)
	bad = append([]byte{}, good...)
	bad[2] = 9 // wrong codec
	_, _, _, err = quantParams(bad)
	require.ErrorIs(t, err, ErrInvalidQuantBlob)
	_, _, _, err = quantParams(good[:len(good)-1]) // length != header+dim
	require.ErrorIs(t, err, ErrInvalidQuantBlob)
}

func TestAsymDistanceApproxMatchesExact(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const dim = 384
	for trial := 0; trial < 200; trial++ {
		q := randVec(rng, dim)
		v := randVec(rng, dim)
		blob := QuantizeFloat32(v)

		// Squared L2: asymmetric (q exact, v quantized) vs fully-exact.
		gotL2, err := AsymSquaredL2Float32(q, blob)
		require.NoError(t, err)
		wantL2 := exactSqL2(q, v)
		// Relative error should be small (quantization only perturbs v).
		require.InEpsilon(t, wantL2, float64(gotL2), 0.05, "trial=%d", trial)

		// Dot product.
		gotDot, err := AsymDotFloat32(q, blob)
		require.NoError(t, err)
		var wantDot float64
		for i := range q {
			wantDot += float64(q[i]) * float64(v[i])
		}
		require.InDelta(t, wantDot, float64(gotDot), 0.02*math.Abs(wantDot)+0.5, "trial=%d", trial)

		// Cosine.
		gotCos, err := AsymCosineSimilarityFloat32(q, blob)
		require.NoError(t, err)
		var dot, qn, vn float64
		for i := range q {
			dot += float64(q[i]) * float64(v[i])
			qn += float64(q[i]) * float64(q[i])
			vn += float64(v[i]) * float64(v[i])
		}
		wantCos := dot / (math.Sqrt(qn) * math.Sqrt(vn))
		require.InDelta(t, wantCos, float64(gotCos), 0.02, "trial=%d", trial)
	}
}

// TestQuantizationRecall is the key quality test: over a corpus of random
// vectors, the top-k nearest neighbors ranked by asymmetric quantized L2 must
// largely agree with the exact top-k. This is the metric that matters for an
// ANN index using quantized distance.
func TestQuantizationRecall(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const (
		dim     = 384
		corpus  = 2000
		queries = 200
		k       = 10
	)
	vecs := make([][]float32, corpus)
	blobs := make([][]byte, corpus)
	for i := range vecs {
		vecs[i] = randVec(rng, dim)
		blobs[i] = QuantizeFloat32(vecs[i])
	}

	topK := func(score func(i int) float64) []int {
		idx := make([]int, corpus)
		for i := range idx {
			idx[i] = i
		}
		sort.Slice(idx, func(a, b int) bool { return score(idx[a]) < score(idx[b]) })
		return idx[:k]
	}

	var hits, total int
	for qi := 0; qi < queries; qi++ {
		q := randVec(rng, dim)
		exact := topK(func(i int) float64 { return exactSqL2(q, vecs[i]) })
		approx := topK(func(i int) float64 {
			d, _ := AsymSquaredL2Float32(q, blobs[i])
			return float64(d)
		})
		exactSet := map[int]bool{}
		for _, i := range exact {
			exactSet[i] = true
		}
		for _, i := range approx {
			if exactSet[i] {
				hits++
			}
		}
		total += k
	}
	recall := float64(hits) / float64(total)
	t.Logf("recall@%d = %.4f (corpus=%d, dim=%d, queries=%d)", k, recall, corpus, dim, queries)
	// int8 scalar quantization should preserve recall well above 0.90.
	require.Greater(t, recall, 0.90, "recall@%d too low: %.4f", k, recall)
}

func BenchmarkQuantizeFloat32(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	v := randVec(rng, 384)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = QuantizeFloat32(v)
	}
}

func BenchmarkAsymSquaredL2(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	q := randVec(rng, 384)
	blob := QuantizeFloat32(randVec(rng, 384))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = AsymSquaredL2Float32(q, blob)
	}
}

// BenchmarkExactSquaredL2 is the float32 baseline for comparison.
func BenchmarkExactSquaredL2(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	q := randVec(rng, 384)
	v := randVec(rng, 384)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = exactSqL2(q, v)
	}
}

// BenchmarkMemoryFootprint documents the size reduction.
func BenchmarkMemoryFootprint(b *testing.B) {
	const dim = 384
	raw := dim * 4
	quant := QuantizedLen(dim)
	b.ReportMetric(float64(raw)/float64(quant), "x-smaller")
	b.ReportMetric(float64(raw), "raw-bytes")
	b.ReportMetric(float64(quant), "quant-bytes")
}
