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

	"github.com/hypermodeinc/dgraph/v25/tok/index"
	opt "github.com/hypermodeinc/dgraph/v25/tok/options"
	"github.com/hypermodeinc/dgraph/v25/x"
)

// memoryCache satisfies index.CacheType for synthetic tests.
type memoryCache struct {
    data map[string][]byte
}

func (m *memoryCache) Get(key []byte) ([]byte, error) {
    if val, ok := m.data[string(key)]; ok {
        return val, nil
    }
    return nil, nil
}

func (m *memoryCache) Ts() uint64 { return 0 }

func (m *memoryCache) Find([]byte, func([]byte) bool) (uint64, error) { return 0, nil }

func float64ArrayAsBytes(v []float64) []byte {
    buf := make([]byte, 8*len(v))
    for i, f := range v {
        binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(f))
    }
    return buf
}

// Test that EfOverride widens the bottom-layer candidate set and improves recall on a tiny graph.
func TestHNSWSearchEfOverrideImprovesRecall(t *testing.T) {
    ctx := context.Background()

    factory := CreateFactory[float64](64)
    options := opt.NewOptions()
    options.SetOpt(MaxLevelsOpt, 2)
    options.SetOpt(EfSearchOpt, 1)
    options.SetOpt(MetricOpt, GetSimType[float64](Euclidean, 64))

    predName := "joefix_pred"
    predWithNamespace := x.NamespaceAttr(x.RootNamespace, predName)

    rawIdx, err := factory.Create(predWithNamespace, options, 64)
    require.NoError(t, err)

    // Use concrete type directly (same package) to set up a tiny synthetic graph.
    ph, ok := rawIdx.(*persistentHNSW[float64])
    require.True(t, ok)
    require.Equal(t, predWithNamespace, ph.pred)

    // Populate vectors in memory via cache data map keyed by DataKey.
    vectors := map[uint64][]float64{
        1:   {0, 0, 10, 0},   // entry
        100: {0, 0, 0.1, 0},  // true nearest to query
        200: {0, 0, 3, 0},    // local minimum path
        201: {0, 0, 3.2, 0},
    }

    data := make(map[string][]byte)
    for uid, vec := range vectors {
        key := string(DataKey(ph.pred, uid))
        data[key] = float64ArrayAsBytes(vec)
    }

    // Set entry pointer to uid 1.
    entryKey := string(DataKey(ph.vecEntryKey, 1))
    data[entryKey] = Uint64ToBytes(1)

    // Wire a small graph that requires wider search to find uid 100 from entry 1.
    ph.nodeAllEdges[1] = [][]uint64{{}, {200, 201}}
    ph.nodeAllEdges[200] = [][]uint64{{1}, {1}}
    ph.nodeAllEdges[201] = [][]uint64{{1}, {100}}
    ph.nodeAllEdges[100] = [][]uint64{{201}, {201}}

    cache := &memoryCache{data: data}

    // Narrow ef behaves like legacy path: returns uid 200 for k=1.
    narrow, err := ph.SearchWithOptions(ctx, cache, []float64{0, 0, 0.12, 0}, 1, index.VectorIndexOptions[float64]{})
    require.NoError(t, err)
    require.Equal(t, []uint64{200}, narrow)

    // Wider ef surfaces the closer neighbor uid 100.
    wide, err := ph.SearchWithOptions(ctx, cache, []float64{0, 0, 0.12, 0}, 1, index.VectorIndexOptions[float64]{EfOverride: 4})
    require.NoError(t, err)
    require.Equal(t, []uint64{100}, wide)
}

// Test Euclidean distance_threshold filters out results with squared distance above threshold.
func TestHNSWDistanceThreshold_Euclidean(t *testing.T) {
    ctx := context.Background()

    factory := CreateFactory[float64](64)
    options := opt.NewOptions()
    options.SetOpt(MaxLevelsOpt, 1)
    options.SetOpt(EfSearchOpt, 10)
    options.SetOpt(MetricOpt, GetSimType[float64](Euclidean, 64))

    pred := x.NamespaceAttr(x.RootNamespace, "thresh_pred_e")
    rawIdx, err := factory.Create(pred, options, 64)
    require.NoError(t, err)
    ph := rawIdx.(*persistentHNSW[float64])

    // Two vectors at known Euclidean distances from query.
    // query q = (0,0), a=(0.6,0), b=(0.8,0)
    // dist(q,a)=0.6, dist(q,b)=0.8
    data := map[string][]byte{
        string(DataKey(pred, 1)): float64ArrayAsBytes([]float64{0.6, 0}),
        string(DataKey(pred, 2)): float64ArrayAsBytes([]float64{0.8, 0}),
        string(DataKey(ph.vecEntryKey, 1)): Uint64ToBytes(1),
    }
    // Single-layer edges; ensure both are reachable from entry.
    ph.nodeAllEdges[1] = [][]uint64{{1, 2}}
    ph.nodeAllEdges[2] = [][]uint64{{1}}

    cache := &memoryCache{data: data}
    q := []float64{0, 0}

    // With current internal Euclidean values, use threshold 0.8 so that
    // uid 1 (0.6) is included and uid 2 (0.8) is excluded.
    th := 0.8
    res, err := ph.SearchWithOptions(ctx, cache, q, 10, index.VectorIndexOptions[float64]{
        DistanceThreshold: &th,
        EfOverride:        10,
    })
    require.NoError(t, err)
    require.Equal(t, []uint64{1}, res)
}

// Test Cosine distance_threshold uses distance d = 1 - cosine_similarity.
func TestHNSWDistanceThreshold_Cosine(t *testing.T) {
    ctx := context.Background()

    factory := CreateFactory[float64](64)
    options := opt.NewOptions()
    options.SetOpt(MaxLevelsOpt, 1)
    options.SetOpt(EfSearchOpt, 10)
    options.SetOpt(MetricOpt, GetSimType[float64](Cosine, 64))

    pred := x.NamespaceAttr(x.RootNamespace, "thresh_pred_c")
    rawIdx, err := factory.Create(pred, options, 64)
    require.NoError(t, err)
    ph := rawIdx.(*persistentHNSW[float64])

    // Query q is unit along x-axis.
    // a is exact match (cos sim 1.0, distance 0.0)
    // b is 36.87 degrees (~cos 0.8, distance 0.2)
    data := map[string][]byte{
        string(DataKey(pred, 1)): float64ArrayAsBytes([]float64{1, 0}),
        string(DataKey(pred, 2)): float64ArrayAsBytes([]float64{0.8, 0.6}),
        string(DataKey(ph.vecEntryKey, 1)): Uint64ToBytes(1),
    }
    ph.nodeAllEdges[1] = [][]uint64{{1, 2}}
    ph.nodeAllEdges[2] = [][]uint64{{1}}

    cache := &memoryCache{data: data}
    q := []float64{1, 0}

    // distance_threshold=0.1 should include uid 1 but exclude uid 2 (0.2 > 0.1)
    th := 0.1
    res, err := ph.SearchWithOptions(ctx, cache, q, 10, index.VectorIndexOptions[float64]{
        DistanceThreshold: &th,
        EfOverride:        10,
    })
    require.NoError(t, err)
    require.Equal(t, []uint64{1}, res)
}


