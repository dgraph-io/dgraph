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

// Scalar (int8/uint8) quantization for dense float vectors.
//
// Each vector is quantized independently with an affine (per-vector min/step)
// scheme: every component is mapped to a uint8 code in [0,255]. The encoded
// blob is self-describing:
//
//	[0:4)   float32 min   (little-endian)
//	[4:8)   float32 step  (= (max-min)/255; 0 for a constant vector)
//	[8:8+n) n uint8 codes
//
// so the on-wire/on-disk size is n+8 bytes vs n*4 for raw float32 — ~3.9x
// smaller at 384 dims. The codec is stateless (no global training pass), which
// makes it safe to apply per vector as they are written.
//
// Distance is computed asymmetrically: the query stays full-precision float and
// each stored component is dequantized on the fly (min + code*step). This keeps
// recall high (only the stored side is quantized) and needs no full-vector
// float materialization, so the read cache holds the small int8 blob, not a
// float32 copy.

const quantHeaderSize = 8 // float32 min + float32 step

// ErrInvalidQuantBlob is returned when a quantized blob is malformed.
var ErrInvalidQuantBlob = errors.New("index: invalid quantized vector blob")

// QuantizedLen returns the encoded byte length for a vector of dimension dim.
func QuantizedLen(dim int) int { return quantHeaderSize + dim }

// QuantizeFloat32 affinely quantizes v into a self-describing uint8 blob.
// A nil/empty input yields a nil blob (mirrors an empty vector).
func QuantizeFloat32(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	lo, hi := v[0], v[0]
	for _, x := range v[1:] {
		if x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	step := (hi - lo) / 255.0

	out := make([]byte, quantHeaderSize+len(v))
	binary.LittleEndian.PutUint32(out[0:4], math.Float32bits(lo))
	binary.LittleEndian.PutUint32(out[4:8], math.Float32bits(step))
	codes := out[quantHeaderSize:]
	if step == 0 {
		// Constant vector: all codes 0, dequantize back to lo.
		return out
	}
	inv := 1.0 / step
	for i, x := range v {
		// round-to-nearest, clamped into [0,255].
		c := int32(math.Round(float64((x - lo) * inv)))
		if c < 0 {
			c = 0
		} else if c > 255 {
			c = 255
		}
		codes[i] = byte(c)
	}
	return out
}

// quantParams extracts (min, step, codes) from a blob, validating length.
func quantParams(blob []byte) (lo, step float32, codes []byte, err error) {
	if len(blob) == 0 {
		return 0, 0, nil, nil
	}
	if len(blob) < quantHeaderSize {
		return 0, 0, nil, ErrInvalidQuantBlob
	}
	lo = math.Float32frombits(binary.LittleEndian.Uint32(blob[0:4]))
	step = math.Float32frombits(binary.LittleEndian.Uint32(blob[4:8]))
	codes = blob[quantHeaderSize:]
	return lo, step, codes, nil
}

// QuantizedDim returns the vector dimension encoded in blob.
func QuantizedDim(blob []byte) int {
	if len(blob) < quantHeaderSize {
		return 0
	}
	return len(blob) - quantHeaderSize
}

// DequantizeFloat32 reconstructs an approximate float32 vector from a blob,
// appending into *out (reusing its capacity when possible).
func DequantizeFloat32(blob []byte, out *[]float32) error {
	lo, step, codes, err := quantParams(blob)
	if err != nil {
		return err
	}
	dst := (*out)[:0]
	if cap(dst) < len(codes) {
		dst = make([]float32, 0, len(codes))
	}
	for _, c := range codes {
		dst = append(dst, lo+float32(c)*step)
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
	var sum float64
	for i, c := range codes {
		d := float64(query[i]) - float64(lo+float32(c)*step)
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
	var sum float64
	for i, c := range codes {
		sum += float64(query[i]) * float64(lo+float32(c)*step)
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
	var dot, qn, vn float64
	for i, c := range codes {
		q := float64(query[i])
		v := float64(lo + float32(c)*step)
		dot += q * v
		qn += q * q
		vn += v * v
	}
	if qn == 0 || vn == 0 {
		return 0, nil
	}
	return float32(dot / (math.Sqrt(qn) * math.Sqrt(vn))), nil
}
