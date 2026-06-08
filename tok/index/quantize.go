/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package index

import (
	"encoding/binary"
	"errors"
	"math"
)

// Scalar (uint8) quantization for dense float vectors.
//
// Each vector is quantized independently with an affine (per-vector min/step)
// scheme: every component is mapped to a uint8 code in [0,255]. The encoded
// blob is self-describing and versioned:
//
//	[0]      magic = 'q' (0x71)
//	[1]      version (quantVersion1)
//	[2]      codec  (quantCodecAffineU8)
//	[3]      flags  (reserved, 0)
//	[4:6)    uint16 dim          (little-endian)
//	[6:10)   float32 min         (little-endian)
//	[10:14)  float32 step        (= (max-min)/255; 0 for a constant vector)
//	[14:14+dim) dim uint8 codes
//
// so the on-disk size is dim+14 bytes vs dim*4 for raw float32 — ~3.8x smaller
// at 384 dims. The codec is stateless (no global training pass), so it is safe
// to apply per vector as they are written, and robust to distribution shifts.
//
// Distance is computed asymmetrically: the query stays full-precision and each
// stored component is dequantized (min + code*step). In dgraph's HNSW the
// integration dequantizes a blob into a reused float32 buffer and reuses the
// existing SIMD distance kernels; the Asym* helpers here are an allocation-free
// alternative kept for tests and a possible future fast path.
//
// NaN/Inf inputs are sanitized at encode time (NaN->0, +/-Inf->+/-MaxFloat32)
// because a NaN leaking into a distance would corrupt HNSW's ordering.

const (
	quantMagic         = 0x71 // 'q'
	quantVersion1      = 1
	quantCodecAffineU8 = 1
	quantHeaderSize    = 14 // magic+version+codec+flags + u16 dim + f32 min + f32 step
)

// ErrInvalidQuantBlob is returned when a quantized blob is malformed,
// the wrong version/codec, or its declared dim does not match its length.
var ErrInvalidQuantBlob = errors.New("index: invalid quantized vector blob")

// QuantizedLen returns the encoded byte length for a vector of dimension dim.
func QuantizedLen(dim int) int { return quantHeaderSize + dim }

func sanitize(x float32) float32 {
	switch {
	case x != x: // NaN
		return 0
	case math.IsInf(float64(x), 1):
		return math.MaxFloat32
	case math.IsInf(float64(x), -1):
		return -math.MaxFloat32
	default:
		return x
	}
}

// QuantizeFloat32 affinely quantizes v into a self-describing, versioned blob.
// A nil/empty input yields a nil blob (mirrors an empty vector). Dimensions
// above the uint16 range are not supported and yield a nil blob.
func QuantizeFloat32(v []float32) []byte {
	if len(v) == 0 || len(v) > math.MaxUint16 {
		return nil
	}
	lo, hi := sanitize(v[0]), sanitize(v[0])
	for _, raw := range v[1:] {
		x := sanitize(raw)
		if x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	// Compute in float64 to avoid overflow when the (sanitized) range is huge.
	step := float32((float64(hi) - float64(lo)) / 255.0)

	out := make([]byte, quantHeaderSize+len(v))
	out[0] = quantMagic
	out[1] = quantVersion1
	out[2] = quantCodecAffineU8
	out[3] = 0
	binary.LittleEndian.PutUint16(out[4:6], uint16(len(v)))
	binary.LittleEndian.PutUint32(out[6:10], math.Float32bits(lo))
	binary.LittleEndian.PutUint32(out[10:14], math.Float32bits(step))
	codes := out[quantHeaderSize:]
	if step == 0 {
		// Constant vector: all codes 0, dequantize back to lo.
		return out
	}
	inv := 1.0 / step
	for i, raw := range v {
		c := int32(math.Round(float64((sanitize(raw) - lo) * inv)))
		if c < 0 {
			c = 0
		} else if c > 255 {
			c = 255
		}
		codes[i] = byte(c)
	}
	return out
}

// quantParams validates a blob and extracts (min, step, codes).
func quantParams(blob []byte) (lo, step float32, codes []byte, err error) {
	if len(blob) == 0 {
		return 0, 0, nil, nil
	}
	if len(blob) < quantHeaderSize ||
		blob[0] != quantMagic || blob[1] != quantVersion1 || blob[2] != quantCodecAffineU8 {
		return 0, 0, nil, ErrInvalidQuantBlob
	}
	dim := int(binary.LittleEndian.Uint16(blob[4:6]))
	if len(blob) != quantHeaderSize+dim {
		return 0, 0, nil, ErrInvalidQuantBlob
	}
	lo = math.Float32frombits(binary.LittleEndian.Uint32(blob[6:10]))
	step = math.Float32frombits(binary.LittleEndian.Uint32(blob[10:14]))
	codes = blob[quantHeaderSize:]
	return lo, step, codes, nil
}

// QuantizedDim returns the vector dimension declared in blob, or 0 if the blob
// is empty or malformed.
func QuantizedDim(blob []byte) int {
	if len(blob) < quantHeaderSize || blob[0] != quantMagic {
		return 0
	}
	return int(binary.LittleEndian.Uint16(blob[4:6]))
}

// DequantizeFloat32 reconstructs an approximate float32 vector from a blob,
// appending into *out (reusing its capacity when possible). The result slice is
// sliced to exactly the blob's dimension.
func DequantizeFloat32(blob []byte, out *[]float32) error {
	lo, step, codes, err := quantParams(blob)
	if err != nil {
		return err
	}
	dst := (*out)[:0]
	if cap(dst) < len(codes) {
		dst = make([]float32, 0, len(codes))
	}
	lo64, step64 := float64(lo), float64(step)
	for _, c := range codes {
		dst = append(dst, float32(lo64+float64(c)*step64))
	}
	*out = dst
	return nil
}

// AsymSquaredL2Float32 returns the squared L2 distance between a full-precision
// query and a quantized vector, dequantizing each stored component on the fly.
func AsymSquaredL2Float32(query []float32, blob []byte) (float32, error) {
	lo, step, codes, err := quantParams(blob)
	if err != nil {
		return 0, err
	}
	if len(query) != len(codes) {
		return 0, ErrInvalidQuantBlob
	}
	lo64, step64 := float64(lo), float64(step)
	var sum float64
	for i, c := range codes {
		d := float64(query[i]) - (lo64 + float64(c)*step64)
		sum += d * d
	}
	return float32(sum), nil
}

// AsymDotFloat32 returns the dot product of a full-precision query and a
// quantized vector.
func AsymDotFloat32(query []float32, blob []byte) (float32, error) {
	lo, step, codes, err := quantParams(blob)
	if err != nil {
		return 0, err
	}
	if len(query) != len(codes) {
		return 0, ErrInvalidQuantBlob
	}
	lo64, step64 := float64(lo), float64(step)
	var sum float64
	for i, c := range codes {
		sum += float64(query[i]) * (lo64 + float64(c)*step64)
	}
	return float32(sum), nil
}

// AsymCosineSimilarityFloat32 returns the cosine similarity between a
// full-precision query and a quantized vector. Returns 0 if either side has
// zero magnitude.
func AsymCosineSimilarityFloat32(query []float32, blob []byte) (float32, error) {
	lo, step, codes, err := quantParams(blob)
	if err != nil {
		return 0, err
	}
	if len(query) != len(codes) {
		return 0, ErrInvalidQuantBlob
	}
	lo64, step64 := float64(lo), float64(step)
	var dot, qn, vn float64
	for i, c := range codes {
		q := float64(query[i])
		v := lo64 + float64(c)*step64
		dot += q * v
		qn += q * q
		vn += v * v
	}
	if qn == 0 || vn == 0 {
		return 0, nil
	}
	return float32(dot / (math.Sqrt(qn) * math.Sqrt(vn))), nil
}
