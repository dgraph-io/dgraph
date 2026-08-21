//go:build !goexperiment.simd

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import "math"

// Portable distance kernels used when the build does not enable GOEXPERIMENT=simd.
// See kernels.go for the contract these must satisfy.
//
// These are deliberately unrolled into independent accumulators rather than written
// as the obvious single-accumulator loop. Dot product and squared euclidean use a
// 4-way unroll; cosine already has three independent chains (dot, |a|, |b|) so it
// uses a 2-way unroll, giving six, which is enough to saturate the FP pipeline
// without risking register spills.

func dotF32(a, b []float32) float32 {
	b = b[:len(a)]
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		// The loads are hoisted into locals rather than indexed inline in the
		// accumulation. That is not cosmetic: under go1.27.0 on arm64 the inline form
		// generates a 1.6x slower loop (253ns vs 158ns at 768 dimensions), while the
		// hoisted form matches go1.26.5. The other two kernels here happen to hoist
		// already, via their difference and product temporaries.
		a0, b0 := a[i], b[i]
		a1, b1 := a[i+1], b[i+1]
		a2, b2 := a[i+2], b[i+2]
		a3, b3 := a[i+3], b[i+3]
		s0 += a0 * b0
		s1 += a1 * b1
		s2 += a2 * b2
		s3 += a3 * b3
	}
	for ; i < len(a); i++ {
		s0 += a[i] * b[i]
	}
	return (s0 + s1) + (s2 + s3)
}

func dotF64(a, b []float64) float64 {
	b = b[:len(a)]
	var s0, s1, s2, s3 float64
	i := 0
	for ; i+4 <= len(a); i += 4 {
		// The loads are hoisted into locals rather than indexed inline in the
		// accumulation. That is not cosmetic: under go1.27.0 on arm64 the inline form
		// generates a 1.6x slower loop (253ns vs 158ns at 768 dimensions), while the
		// hoisted form matches go1.26.5. The other two kernels here happen to hoist
		// already, via their difference and product temporaries.
		a0, b0 := a[i], b[i]
		a1, b1 := a[i+1], b[i+1]
		a2, b2 := a[i+2], b[i+2]
		a3, b3 := a[i+3], b[i+3]
		s0 += a0 * b0
		s1 += a1 * b1
		s2 += a2 * b2
		s3 += a3 * b3
	}
	for ; i < len(a); i++ {
		s0 += a[i] * b[i]
	}
	return (s0 + s1) + (s2 + s3)
}

func euclideanSqF32(a, b []float32) float32 {
	b = b[:len(a)]
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		d0 := a[i] - b[i]
		d1 := a[i+1] - b[i+1]
		d2 := a[i+2] - b[i+2]
		d3 := a[i+3] - b[i+3]
		s0 += d0 * d0
		s1 += d1 * d1
		s2 += d2 * d2
		s3 += d3 * d3
	}
	for ; i < len(a); i++ {
		d := a[i] - b[i]
		s0 += d * d
	}
	return (s0 + s1) + (s2 + s3)
}

func euclideanSqF64(a, b []float64) float64 {
	b = b[:len(a)]
	var s0, s1, s2, s3 float64
	i := 0
	for ; i+4 <= len(a); i += 4 {
		d0 := a[i] - b[i]
		d1 := a[i+1] - b[i+1]
		d2 := a[i+2] - b[i+2]
		d3 := a[i+3] - b[i+3]
		s0 += d0 * d0
		s1 += d1 * d1
		s2 += d2 * d2
		s3 += d3 * d3
	}
	for ; i < len(a); i++ {
		d := a[i] - b[i]
		s0 += d * d
	}
	return (s0 + s1) + (s2 + s3)
}

func euclideanF32(a, b []float32) float32 {
	return float32(math.Sqrt(float64(euclideanSqF32(a, b))))
}

func euclideanF64(a, b []float64) float64 {
	return math.Sqrt(euclideanSqF64(a, b))
}

func cosineSimF32(a, b []float32) float32 {
	b = b[:len(a)]
	var d0, d1, x0, x1, y0, y1 float32
	i := 0
	for ; i+2 <= len(a); i += 2 {
		av0, bv0 := a[i], b[i]
		av1, bv1 := a[i+1], b[i+1]
		d0 += av0 * bv0
		d1 += av1 * bv1
		x0 += av0 * av0
		x1 += av1 * av1
		y0 += bv0 * bv0
		y1 += bv1 * bv1
	}
	for ; i < len(a); i++ {
		d0 += a[i] * b[i]
		x0 += a[i] * a[i]
		y0 += b[i] * b[i]
	}
	dot, na, nb := d0+d1, x0+x1, y0+y1
	return dot / float32(math.Sqrt(float64(na)*float64(nb)))
}

func cosineSimF64(a, b []float64) float64 {
	b = b[:len(a)]
	var d0, d1, x0, x1, y0, y1 float64
	i := 0
	for ; i+2 <= len(a); i += 2 {
		av0, bv0 := a[i], b[i]
		av1, bv1 := a[i+1], b[i+1]
		d0 += av0 * bv0
		d1 += av1 * bv1
		x0 += av0 * av0
		x1 += av1 * av1
		y0 += bv0 * bv0
		y1 += bv1 * bv1
	}
	for ; i < len(a); i++ {
		d0 += a[i] * b[i]
		x0 += a[i] * a[i]
		y0 += b[i] * b[i]
	}
	dot, na, nb := d0+d1, x0+x1, y0+y1
	return dot / math.Sqrt(na*nb)
}
