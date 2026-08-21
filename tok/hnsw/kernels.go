/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

// This file documents the distance-kernel contract shared by the two build-tagged
// implementations in kernels_simd.go (//go:build goexperiment.simd) and
// kernels_generic.go (//go:build !goexperiment.simd).
//
// Each implementation provides, for both float32 and float64:
//
//	dotF32/dotF64                 - sum(a[i]*b[i])
//	euclideanSqF32/euclideanSqF64 - sum((a[i]-b[i])^2), NOT square-rooted
//	euclideanF32/euclideanF64     - sqrt of the above, the metric-domain distance
//	cosineSimF32/cosineSimF64     - dot(a,b) / sqrt(dot(a,a)*dot(b,b))
//
// Contract for every kernel:
//
//   - Callers guarantee len(a) == len(b), and applyDistanceFunction enforces it by
//     returning an error before any kernel is reached. Each kernel then reslices b to
//     len(a) so the compiler can eliminate bounds checks on b inside the loop. Note
//     that this reslice is not itself a length check: b[:len(a)] succeeds whenever
//     cap(b) >= len(a), so passing a short subslice of a longer array reads the
//     elements beyond its length rather than panicking. The guard is the wrapper, not
//     the kernel.
//   - Zero-length input yields 0 for dot and euclidean, and NaN for cosine (0/0).
//     This matches the behaviour of a zero vector and is strictly safer than the
//     vek implementation this replaced, which panicked on empty input.
//   - Results are not bit-identical to a naive left-to-right summation. Both
//     implementations use multiple independent accumulators, and the SIMD path
//     additionally uses fused multiply-add, so partial sums are reassociated and
//     rounded differently. Relative error against a float64 reference stays within
//     a few ULP of float32 (~4e-7 measured at 768 dimensions), which is far below
//     the resolution at which ranking decisions differ. Tests must compare with a
//     tolerance rather than for exact equality.
//
// Why multiple accumulators: the obvious `sum += a[i]*b[i]` loop is bound by the
// latency of the floating-point add dependency chain, not by throughput. Splitting
// into independent partial sums lets the CPU keep several adds in flight, which is
// worth roughly 2x on its own before any vectorisation.
