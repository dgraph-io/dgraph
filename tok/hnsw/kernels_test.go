/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

// These tests exercise whichever kernel implementation the build selected, so they run
// identically with and without GOEXPERIMENT=simd. Every assertion is a tolerance
// comparison against a float64 reference: the kernels reassociate partial sums across
// independent accumulators and, on the SIMD path, use fused multiply-add, so results are
// deliberately not bit-identical to a naive summation. See kernels.go.

// relTol is a generous bound on float32 accumulation error over the dimensions tested.
// Measured error at 768 dimensions is ~4e-7; ranking decisions are unaffected well
// before this threshold.
const relTol = 1e-5

// kernelDims covers the tail cases that the unrolls and the partial vector loads have to
// handle: shorter than one vector, not a multiple of the unroll factor, and exactly on
// the boundary for widths up to 512 bits.
var kernelDims = []int{0, 1, 2, 3, 4, 5, 7, 8, 15, 16, 17, 31, 32, 33, 63, 64, 65, 128, 384, 768, 1000, 1536}

func randVec32(n int, seed int64) []float32 {
	r := rand.New(rand.NewSource(seed))
	v := make([]float32, n)
	for i := range v {
		v[i] = r.Float32()*2 - 1
	}
	return v
}

func randVec64(n int, seed int64) []float64 {
	r := rand.New(rand.NewSource(seed))
	v := make([]float64, n)
	for i := range v {
		v[i] = r.Float64()*2 - 1
	}
	return v
}

// refDot, refEuclideanSq and refCosine accumulate in float64 regardless of input width,
// giving a reference the kernels can be measured against.
func refDot(a, b []float64) float64 {
	var s float64
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func refEuclideanSq(a, b []float64) float64 {
	var s float64
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return s
}

func refCosine(a, b []float64) float64 {
	return refDot(a, b) / math.Sqrt(refDot(a, a)*refDot(b, b))
}

func widen(v []float32) []float64 {
	out := make([]float64, len(v))
	for i, f := range v {
		out[i] = float64(f)
	}
	return out
}

// requireClose compares against the reference with a relative tolerance, falling back to
// an absolute bound when the reference is near zero.
func requireClose(t *testing.T, got, want float64, what string) {
	t.Helper()
	if math.Abs(want) < 1e-6 {
		require.InDelta(t, want, got, 1e-6, what)
		return
	}
	require.InEpsilon(t, want, got, relTol, what)
}

func TestKernelsFloat32(t *testing.T) {
	for _, n := range kernelDims {
		t.Run(fmt.Sprintf("d=%d", n), func(t *testing.T) {
			a, b := randVec32(n, 1), randVec32(n, 2)
			a64, b64 := widen(a), widen(b)

			requireClose(t, float64(dotF32(a, b)), refDot(a64, b64), "dot")
			requireClose(t, float64(euclideanSqF32(a, b)), refEuclideanSq(a64, b64), "euclideanSq")
			requireClose(t, float64(euclideanF32(a, b)), math.Sqrt(refEuclideanSq(a64, b64)), "euclidean")

			// Cosine of a zero-length or zero-magnitude vector is 0/0. Skip the
			// comparison there; TestKernelsDegenerate covers it explicitly.
			if n > 0 {
				requireClose(t, float64(cosineSimF32(a, b)), refCosine(a64, b64), "cosine")
			}
		})
	}
}

func TestKernelsFloat64(t *testing.T) {
	for _, n := range kernelDims {
		t.Run(fmt.Sprintf("d=%d", n), func(t *testing.T) {
			a, b := randVec64(n, 3), randVec64(n, 4)

			requireClose(t, dotF64(a, b), refDot(a, b), "dot")
			requireClose(t, euclideanSqF64(a, b), refEuclideanSq(a, b), "euclideanSq")
			requireClose(t, euclideanF64(a, b), math.Sqrt(refEuclideanSq(a, b)), "euclidean")

			if n > 0 {
				requireClose(t, cosineSimF64(a, b), refCosine(a, b), "cosine")
			}
		})
	}
}

// TestKernelsSelfDistance pins the identities the HNSW search relies on: a vector is at
// distance zero from itself and has cosine similarity 1.
func TestKernelsSelfDistance(t *testing.T) {
	for _, n := range []int{1, 8, 17, 768} {
		a := randVec32(n, 5)
		require.Zero(t, euclideanSqF32(a, a), "euclideanSq(a,a) must be exactly 0")
		require.Zero(t, euclideanF32(a, a), "euclidean(a,a) must be exactly 0")
		require.InDelta(t, 1.0, float64(cosineSimF32(a, a)), relTol, "cosine(a,a)")

		d := randVec64(n, 6)
		require.Zero(t, euclideanSqF64(d, d), "euclideanSq(d,d) must be exactly 0")
		require.InDelta(t, 1.0, cosineSimF64(d, d), relTol, "cosine(d,d)")
	}
}

// TestKernelsDegenerate documents the empty and zero-vector behaviour promised in
// kernels.go. The vek implementation this replaced panicked on empty input.
func TestKernelsDegenerate(t *testing.T) {
	var empty32 []float32
	require.Zero(t, dotF32(empty32, empty32))
	require.Zero(t, euclideanSqF32(empty32, empty32))
	require.Zero(t, euclideanF32(empty32, empty32))
	require.True(t, math.IsNaN(float64(cosineSimF32(empty32, empty32))), "cosine of empty is 0/0")

	zeros := make([]float32, 16)
	require.Zero(t, dotF32(zeros, zeros))
	require.Zero(t, euclideanSqF32(zeros, zeros))
	require.True(t, math.IsNaN(float64(cosineSimF32(zeros, zeros))), "cosine of zero vector is 0/0")

	var empty64 []float64
	require.Zero(t, dotF64(empty64, empty64))
	require.Zero(t, euclideanSqF64(empty64, empty64))
	require.True(t, math.IsNaN(cosineSimF64(empty64, empty64)), "cosine of empty is 0/0")
}

// TestKernelsMismatchedLength pins the real boundary behaviour, which is subtler than it
// looks. Kernels reslice b to len(a) for bounds-check elimination, so a genuinely short b
// panics, but a short *subslice of a longer array* does not: the reslice stays within
// capacity and silently reads the elements past b's length. That is why the length guard
// lives in applyDistanceFunction rather than in the kernels, and why nothing should call
// a kernel directly. See TestDistanceScoreLengthMismatch for the enforced path.
func TestKernelsMismatchedLength(t *testing.T) {
	a := randVec32(16, 7)

	// Insufficient capacity: the reslice panics.
	short := randVec32(8, 13)
	require.Panics(t, func() { dotF32(a, short) })
	require.Panics(t, func() { euclideanSqF32(a, short) })
	require.Panics(t, func() { cosineSimF32(a, short) })

	// Sufficient capacity: the reslice succeeds and reads past len(b). Documented here
	// so the behaviour is deliberate rather than a latent surprise.
	sub := a[:8]
	require.NotPanics(t, func() { dotF32(a, sub) })
	require.Equal(t, dotF32(a, a), dotF32(a, sub),
		"a short subslice is silently widened back to the full array")
}

// TestDistanceScoreLengthMismatch covers the wrapper that guards the kernels.
func TestDistanceScoreLengthMismatch(t *testing.T) {
	a := randVec32(16, 8)
	for name, fn := range map[string]func(a, b []float32, floatBits int) (float32, error){
		"dot":       dotProduct[float32],
		"cosine":    cosineSimilarity[float32],
		"euclidean": euclideanDistance[float32],
	} {
		_, err := fn(a, a[:8], 32)
		require.Error(t, err, name)
		require.Contains(t, err.Error(), "different lengths", name)
	}
}

// TestKernelsNoAllocs guards against a regression that would matter: the horizontal
// reduction on the SIMD path uses a fixed-size stack array precisely so the hottest loop
// in vector search stays allocation free.
func TestKernelsNoAllocs(t *testing.T) {
	a, b := randVec32(768, 9), randVec32(768, 10)
	c, d := randVec64(768, 11), randVec64(768, 12)
	allocs := testing.AllocsPerRun(100, func() {
		_ = dotF32(a, b)
		_ = euclideanSqF32(a, b)
		_ = cosineSimF32(a, b)
		_ = dotF64(c, d)
		_ = euclideanSqF64(c, d)
		_ = cosineSimF64(c, d)
	})
	require.Zero(t, allocs, "distance kernels must not allocate")
}

var benchDims = []int{384, 768, 1536}

func BenchmarkKernelsFloat32(b *testing.B) {
	for _, n := range benchDims {
		x, y := randVec32(n, 1), randVec32(n, 2)
		b.Run(fmt.Sprintf("dot/d=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n * 4 * 2))
			for b.Loop() {
				_ = dotF32(x, y)
			}
		})
		b.Run(fmt.Sprintf("euclideanSq/d=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n * 4 * 2))
			for b.Loop() {
				_ = euclideanSqF32(x, y)
			}
		})
		b.Run(fmt.Sprintf("euclidean/d=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n * 4 * 2))
			for b.Loop() {
				_ = euclideanF32(x, y)
			}
		})
		b.Run(fmt.Sprintf("cosine/d=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n * 4 * 2))
			for b.Loop() {
				_ = cosineSimF32(x, y)
			}
		})
	}
}

func BenchmarkKernelsFloat64(b *testing.B) {
	for _, n := range benchDims {
		x, y := randVec64(n, 1), randVec64(n, 2)
		b.Run(fmt.Sprintf("dot/d=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n * 8 * 2))
			for b.Loop() {
				_ = dotF64(x, y)
			}
		})
		b.Run(fmt.Sprintf("euclideanSq/d=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n * 8 * 2))
			for b.Loop() {
				_ = euclideanSqF64(x, y)
			}
		})
		b.Run(fmt.Sprintf("cosine/d=%d", n), func(b *testing.B) {
			b.SetBytes(int64(n * 8 * 2))
			for b.Loop() {
				_ = cosineSimF64(x, y)
			}
		})
	}
}
