//go:build goexperiment.simd

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"math"
	"simd"
)

// Vectorised distance kernels, used when the build enables GOEXPERIMENT=simd.
// See kernels.go for the contract these must satisfy.
//
// The vector width is chosen once per program execution by the simd package: 128 bits
// on arm64 (Neon) and wasm, and 256 or 512 bits on amd64 depending on AVX2/AVX-512
// support. Nothing here assumes a width; Len() is queried at run time and the tail
// is handled with the zero-filling Part loads.
//
// Dot product and squared euclidean use four independent vector accumulators, so the
// effective instruction-level parallelism is four times the lane count. Cosine uses two,
// across its three chains, for six vector accumulators total.
//
// maxF32Lanes and maxF64Lanes bound the horizontal-reduction scratch buffers so they
// stack-allocate. They cover the widest vector the simd package can select, 512 bits.

const (
	maxF32Lanes = 16
	maxF64Lanes = 8
)

// hsumF32 reduces a vector accumulator to a scalar. The scratch array is fixed size so
// it does not escape; a make() sized from Len() would heap-allocate on every distance
// computation, which is the hottest loop in vector search.
func hsumF32(v simd.Float32s) float32 {
	var buf [maxF32Lanes]float32
	n := v.Len()
	v.Store(buf[:n])
	var s float32
	for _, f := range buf[:n] {
		s += f
	}
	return s
}

func hsumF64(v simd.Float64s) float64 {
	var buf [maxF64Lanes]float64
	n := v.Len()
	v.Store(buf[:n])
	var s float64
	for _, f := range buf[:n] {
		s += f
	}
	return s
}

func dotF32(a, b []float32) float32 {
	b = b[:len(a)]
	var acc0, acc1, acc2, acc3 simd.Float32s
	w := acc0.Len()
	i := 0
	for ; i+4*w <= len(a); i += 4 * w {
		acc0 = simd.LoadFloat32s(a[i:]).MulAdd(simd.LoadFloat32s(b[i:]), acc0)
		acc1 = simd.LoadFloat32s(a[i+w:]).MulAdd(simd.LoadFloat32s(b[i+w:]), acc1)
		acc2 = simd.LoadFloat32s(a[i+2*w:]).MulAdd(simd.LoadFloat32s(b[i+2*w:]), acc2)
		acc3 = simd.LoadFloat32s(a[i+3*w:]).MulAdd(simd.LoadFloat32s(b[i+3*w:]), acc3)
	}
	for ; i+w <= len(a); i += w {
		acc0 = simd.LoadFloat32s(a[i:]).MulAdd(simd.LoadFloat32s(b[i:]), acc0)
	}
	if i < len(a) {
		av, _ := simd.LoadFloat32sPart(a[i:])
		bv, _ := simd.LoadFloat32sPart(b[i:])
		acc0 = av.MulAdd(bv, acc0)
	}
	return hsumF32(acc0.Add(acc1).Add(acc2.Add(acc3)))
}

func dotF64(a, b []float64) float64 {
	b = b[:len(a)]
	var acc0, acc1, acc2, acc3 simd.Float64s
	w := acc0.Len()
	i := 0
	for ; i+4*w <= len(a); i += 4 * w {
		acc0 = simd.LoadFloat64s(a[i:]).MulAdd(simd.LoadFloat64s(b[i:]), acc0)
		acc1 = simd.LoadFloat64s(a[i+w:]).MulAdd(simd.LoadFloat64s(b[i+w:]), acc1)
		acc2 = simd.LoadFloat64s(a[i+2*w:]).MulAdd(simd.LoadFloat64s(b[i+2*w:]), acc2)
		acc3 = simd.LoadFloat64s(a[i+3*w:]).MulAdd(simd.LoadFloat64s(b[i+3*w:]), acc3)
	}
	for ; i+w <= len(a); i += w {
		acc0 = simd.LoadFloat64s(a[i:]).MulAdd(simd.LoadFloat64s(b[i:]), acc0)
	}
	if i < len(a) {
		av, _ := simd.LoadFloat64sPart(a[i:])
		bv, _ := simd.LoadFloat64sPart(b[i:])
		acc0 = av.MulAdd(bv, acc0)
	}
	return hsumF64(acc0.Add(acc1).Add(acc2.Add(acc3)))
}

func euclideanSqF32(a, b []float32) float32 {
	b = b[:len(a)]
	var acc0, acc1, acc2, acc3 simd.Float32s
	w := acc0.Len()
	i := 0
	for ; i+4*w <= len(a); i += 4 * w {
		d0 := simd.LoadFloat32s(a[i:]).Sub(simd.LoadFloat32s(b[i:]))
		d1 := simd.LoadFloat32s(a[i+w:]).Sub(simd.LoadFloat32s(b[i+w:]))
		d2 := simd.LoadFloat32s(a[i+2*w:]).Sub(simd.LoadFloat32s(b[i+2*w:]))
		d3 := simd.LoadFloat32s(a[i+3*w:]).Sub(simd.LoadFloat32s(b[i+3*w:]))
		acc0 = d0.MulAdd(d0, acc0)
		acc1 = d1.MulAdd(d1, acc1)
		acc2 = d2.MulAdd(d2, acc2)
		acc3 = d3.MulAdd(d3, acc3)
	}
	for ; i+w <= len(a); i += w {
		d := simd.LoadFloat32s(a[i:]).Sub(simd.LoadFloat32s(b[i:]))
		acc0 = d.MulAdd(d, acc0)
	}
	if i < len(a) {
		av, _ := simd.LoadFloat32sPart(a[i:])
		bv, _ := simd.LoadFloat32sPart(b[i:])
		d := av.Sub(bv)
		acc0 = d.MulAdd(d, acc0)
	}
	return hsumF32(acc0.Add(acc1).Add(acc2.Add(acc3)))
}

func euclideanSqF64(a, b []float64) float64 {
	b = b[:len(a)]
	var acc0, acc1, acc2, acc3 simd.Float64s
	w := acc0.Len()
	i := 0
	for ; i+4*w <= len(a); i += 4 * w {
		d0 := simd.LoadFloat64s(a[i:]).Sub(simd.LoadFloat64s(b[i:]))
		d1 := simd.LoadFloat64s(a[i+w:]).Sub(simd.LoadFloat64s(b[i+w:]))
		d2 := simd.LoadFloat64s(a[i+2*w:]).Sub(simd.LoadFloat64s(b[i+2*w:]))
		d3 := simd.LoadFloat64s(a[i+3*w:]).Sub(simd.LoadFloat64s(b[i+3*w:]))
		acc0 = d0.MulAdd(d0, acc0)
		acc1 = d1.MulAdd(d1, acc1)
		acc2 = d2.MulAdd(d2, acc2)
		acc3 = d3.MulAdd(d3, acc3)
	}
	for ; i+w <= len(a); i += w {
		d := simd.LoadFloat64s(a[i:]).Sub(simd.LoadFloat64s(b[i:]))
		acc0 = d.MulAdd(d, acc0)
	}
	if i < len(a) {
		av, _ := simd.LoadFloat64sPart(a[i:])
		bv, _ := simd.LoadFloat64sPart(b[i:])
		d := av.Sub(bv)
		acc0 = d.MulAdd(d, acc0)
	}
	return hsumF64(acc0.Add(acc1).Add(acc2.Add(acc3)))
}

func euclideanF32(a, b []float32) float32 {
	return float32(math.Sqrt(float64(euclideanSqF32(a, b))))
}

func euclideanF64(a, b []float64) float64 {
	return math.Sqrt(euclideanSqF64(a, b))
}

func cosineSimF32(a, b []float32) float32 {
	b = b[:len(a)]
	var dot0, dot1, na0, na1, nb0, nb1 simd.Float32s
	w := dot0.Len()
	i := 0
	for ; i+2*w <= len(a); i += 2 * w {
		av0 := simd.LoadFloat32s(a[i:])
		bv0 := simd.LoadFloat32s(b[i:])
		av1 := simd.LoadFloat32s(a[i+w:])
		bv1 := simd.LoadFloat32s(b[i+w:])
		dot0 = av0.MulAdd(bv0, dot0)
		dot1 = av1.MulAdd(bv1, dot1)
		na0 = av0.MulAdd(av0, na0)
		na1 = av1.MulAdd(av1, na1)
		nb0 = bv0.MulAdd(bv0, nb0)
		nb1 = bv1.MulAdd(bv1, nb1)
	}
	for ; i+w <= len(a); i += w {
		av := simd.LoadFloat32s(a[i:])
		bv := simd.LoadFloat32s(b[i:])
		dot0 = av.MulAdd(bv, dot0)
		na0 = av.MulAdd(av, na0)
		nb0 = bv.MulAdd(bv, nb0)
	}
	if i < len(a) {
		av, _ := simd.LoadFloat32sPart(a[i:])
		bv, _ := simd.LoadFloat32sPart(b[i:])
		dot0 = av.MulAdd(bv, dot0)
		na0 = av.MulAdd(av, na0)
		nb0 = bv.MulAdd(bv, nb0)
	}
	dot := hsumF32(dot0.Add(dot1))
	na := hsumF32(na0.Add(na1))
	nb := hsumF32(nb0.Add(nb1))
	return dot / float32(math.Sqrt(float64(na)*float64(nb)))
}

func cosineSimF64(a, b []float64) float64 {
	b = b[:len(a)]
	var dot0, dot1, na0, na1, nb0, nb1 simd.Float64s
	w := dot0.Len()
	i := 0
	for ; i+2*w <= len(a); i += 2 * w {
		av0 := simd.LoadFloat64s(a[i:])
		bv0 := simd.LoadFloat64s(b[i:])
		av1 := simd.LoadFloat64s(a[i+w:])
		bv1 := simd.LoadFloat64s(b[i+w:])
		dot0 = av0.MulAdd(bv0, dot0)
		dot1 = av1.MulAdd(bv1, dot1)
		na0 = av0.MulAdd(av0, na0)
		na1 = av1.MulAdd(av1, na1)
		nb0 = bv0.MulAdd(bv0, nb0)
		nb1 = bv1.MulAdd(bv1, nb1)
	}
	for ; i+w <= len(a); i += w {
		av := simd.LoadFloat64s(a[i:])
		bv := simd.LoadFloat64s(b[i:])
		dot0 = av.MulAdd(bv, dot0)
		na0 = av.MulAdd(av, na0)
		nb0 = bv.MulAdd(bv, nb0)
	}
	if i < len(a) {
		av, _ := simd.LoadFloat64sPart(a[i:])
		bv, _ := simd.LoadFloat64sPart(b[i:])
		dot0 = av.MulAdd(bv, dot0)
		na0 = av.MulAdd(av, na0)
		nb0 = bv.MulAdd(bv, nb0)
	}
	dot := hsumF64(dot0.Add(dot1))
	na := hsumF64(na0.Add(na1))
	nb := hsumF64(nb0.Add(nb1))
	return dot / math.Sqrt(na*nb)
}
