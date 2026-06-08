/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/tok/index"
	opt "github.com/dgraph-io/dgraph/v25/tok/options"
	"github.com/dgraph-io/dgraph/v25/x"
)

func float32ArrayAsBytes(v []float32) []byte {
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// TestQuantizedOptionParsing checks the opt-in plumbing and the float-width guard.
func TestQuantizedOptionParsing(t *testing.T) {
	mk := func(bits int, q string) (*persistentHNSW[float32], error) {
		options := opt.NewOptions()
		options.SetOpt(MaxLevelsOpt, 2)
		options.SetOpt(MetricOpt, GetSimType[float32](Euclidean, bits))
		if q != "" {
			options.SetOpt(QuantizeOpt, q)
		}
		idx, err := CreateFactory[float32](bits).Create(
			x.NamespaceAttr(x.RootNamespace, "quant_opt_"+q), options, bits)
		if err != nil {
			return nil, err
		}
		return idx.(*persistentHNSW[float32]), nil
	}

	ph, err := mk(32, "int8")
	require.NoError(t, err)
	require.True(t, ph.quantize)
	require.Equal(t, ConcatStrings(ph.pred, VecQuant), ph.vecQKey)

	ph, err = mk(32, "")
	require.NoError(t, err)
	require.False(t, ph.quantize, "quantization must be off by default")

	_, err = mk(32, "int4")
	require.Error(t, err, "unsupported quantize value must be rejected")

	// 64-bit float vectors are not supported for int8 quantization.
	options := opt.NewOptions()
	options.SetOpt(MaxLevelsOpt, 2)
	options.SetOpt(MetricOpt, GetSimType[float64](Euclidean, 64))
	options.SetOpt(QuantizeOpt, "int8")
	_, err = CreateFactory[float64](64).Create(
		x.NamespaceAttr(x.RootNamespace, "quant_opt_64"), options, 64)
	require.Error(t, err, "int8 quantization must require 32-bit vectors")
}

// TestQuantizedSearchReadPath drives a real SearchWithOptions over a quantized
// index whose vectors live in __vector_q. It mirrors the non-quantized recall
// test (same graph/vectors) and must surface the true nearest neighbor (uid
// 100), proving the read+dequantize path feeds distances correctly.
func TestQuantizedSearchReadPath(t *testing.T) {
	ctx := context.Background()
	options := opt.NewOptions()
	options.SetOpt(MaxLevelsOpt, 2)
	options.SetOpt(EfSearchOpt, 1)
	options.SetOpt(MetricOpt, GetSimType[float32](Euclidean, 32))
	options.SetOpt(QuantizeOpt, "int8")
	pred := x.NamespaceAttr(x.RootNamespace, "quant_read_pred")
	idx, err := CreateFactory[float32](32).Create(pred, options, 32)
	require.NoError(t, err)
	ph := idx.(*persistentHNSW[float32])
	require.True(t, ph.quantize)

	vectors := map[uint64][]float32{
		1:   {0, 0, 10, 0},
		100: {0, 0, 0.1, 0},
		200: {0, 0, 3, 0},
		201: {0, 0, 3.2, 0},
	}
	data := make(map[string][]byte)
	for uid, vec := range vectors {
		// Stored as quantized blobs in __vector_q (what the index reads).
		data[string(DataKey(ph.vecQKey, uid))] = index.QuantizeFloat32(vec)
	}
	// Provide the raw vector for the entry node only: the first read seeds the
	// index dimension from trusted full-precision data. The neighbors (200, 201,
	// 100) have NO raw vector, so search MUST use their quantized blobs — this
	// proves the quantized read path drives traversal/distance.
	data[string(DataKey(ph.pred, 1))] = float32ArrayAsBytes(vectors[1])
	data[string(DataKey(ph.vecEntryKey, 1))] = Uint64ToBytes(1)
	ph.nodeAllEdges[1] = [][]uint64{{}, {200, 201}}
	ph.nodeAllEdges[200] = [][]uint64{{1}, {1}}
	ph.nodeAllEdges[201] = [][]uint64{{1}, {100}}
	ph.nodeAllEdges[100] = [][]uint64{{201}, {201}}

	cache := &memoryCache{data: data}
	query := []float32{0, 0, 0.12, 0}

	res, err := ph.SearchWithOptions(ctx, cache, query, 1,
		index.VectorIndexOptions[float32]{EfOverride: 4})
	require.NoError(t, err)
	require.Equal(t, []uint64{100}, res, "quantized search must find the true nearest neighbor")
}

// TestQuantizedInsertWritesBlob exercises the write path: a real Insert on a
// quantized index must persist a __vector_q blob that round-trips back to ~the
// input vector.
func TestQuantizedInsertWritesBlob(t *testing.T) {
	emptyTsDbs()
	options := opt.NewOptions()
	options.SetOpt(MaxLevelsOpt, 2)
	options.SetOpt(EfConstructionOpt, 5)
	options.SetOpt(EfSearchOpt, 5)
	options.SetOpt(MetricOpt, GetSimType[float32](Euclidean, 32))
	options.SetOpt(QuantizeOpt, "int8")
	pred := x.NamespaceAttr(x.RootNamespace, "quant_insert_pred")
	idx, err := CreateFactory[float32](32).Create(pred, options, 32)
	require.NoError(t, err)
	ph := idx.(*persistentHNSW[float32])

	tc := NewTxnCache(&inMemTxn{startTs: 0, commitTs: 1}, 0)
	vec := []float32{1, 2, 3, 4, 5, 6, 7, 8}
	_, err = ph.Insert(context.TODO(), tc, 42, vec)
	require.NoError(t, err)

	blob := tsDbs[99].inMemTestDb[string(DataKey(ph.vecQKey, 42))]
	require.NotEmpty(t, blob, "Insert must persist a __vector_q blob")
	require.Equal(t, len(vec), index.QuantizedDim(blob))

	var got []float32
	require.NoError(t, index.DequantizeInto(blob, &got))
	require.Len(t, got, len(vec))
	for i := range vec {
		require.InDelta(t, vec[i], got[i], 0.05, "dim %d", i)
	}
}

// TestQuantizedFallbackOnBadBlob verifies that a corrupt or wrong-dimension
// __vector_q blob does not crash search: getVecFromUid falls back to the raw
// vector, and search still returns the true nearest neighbor.
func TestQuantizedFallbackOnBadBlob(t *testing.T) {
	ctx := context.Background()
	options := opt.NewOptions()
	options.SetOpt(MaxLevelsOpt, 2)
	options.SetOpt(EfSearchOpt, 1)
	options.SetOpt(MetricOpt, GetSimType[float32](Euclidean, 32))
	options.SetOpt(QuantizeOpt, "int8")
	pred := x.NamespaceAttr(x.RootNamespace, "quant_fallback_pred")
	idx, err := CreateFactory[float32](32).Create(pred, options, 32)
	require.NoError(t, err)
	ph := idx.(*persistentHNSW[float32])

	vectors := map[uint64][]float32{
		1:   {0, 0, 10, 0},
		100: {0, 0, 0.1, 0},
		200: {0, 0, 3, 0},
		201: {0, 0, 3.2, 0},
	}
	data := make(map[string][]byte)
	for uid, vec := range vectors {
		data[string(DataKey(ph.pred, uid))] = float32ArrayAsBytes(vec)   // raw (fallback)
		data[string(DataKey(ph.vecQKey, uid))] = index.QuantizeFloat32(vec) // good quant
	}
	// Corrupt uid 100's blob (valid header claiming dim 9 but no codes -> length
	// mismatch -> decode error -> raw fallback).
	bad := []byte{0x71, 1, 1, 0, 9, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	data[string(DataKey(ph.vecQKey, 100))] = bad
	// Also corrupt the ENTRY node's blob (uid 1, read first): proves ph.dim is
	// established from its raw fallback rather than poisoned by a bad first blob.
	data[string(DataKey(ph.vecQKey, 1))] = bad
	// Wrong-dimension (valid) blob for uid 201: dim 3 != index dim 4 -> dim
	// guard rejects it -> raw fallback.
	data[string(DataKey(ph.vecQKey, 201))] = index.QuantizeFloat32([]float32{1, 2, 3})

	data[string(DataKey(ph.vecEntryKey, 1))] = Uint64ToBytes(1)
	ph.nodeAllEdges[1] = [][]uint64{{}, {200, 201}}
	ph.nodeAllEdges[200] = [][]uint64{{1}, {1}}
	ph.nodeAllEdges[201] = [][]uint64{{1}, {100}}
	ph.nodeAllEdges[100] = [][]uint64{{201}, {201}}

	cache := &memoryCache{data: data}
	query := []float32{0, 0, 0.12, 0}
	res, err := ph.SearchWithOptions(ctx, cache, query, 1,
		index.VectorIndexOptions[float32]{EfOverride: 4})
	require.NoError(t, err)
	require.Equal(t, []uint64{100}, res, "must fall back to raw and find the true NN without crashing")
}
