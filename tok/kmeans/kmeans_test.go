/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package kmeans

import (
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"testing"

	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/x"
)

// fakeCache is a minimal index.CacheType serving a fixed key/value map.
type fakeCache struct {
	data map[string][]byte
	gets int
}

func (f *fakeCache) Get(key []byte) ([]byte, error) {
	f.gets++
	if v, ok := f.data[string(key)]; ok {
		return v, nil
	}
	return nil, errors.New("no value found")
}

func (f *fakeCache) Ts() uint64 { return 1 }

func (f *fakeCache) Find([]byte, func([]byte) bool) (uint64, error) {
	return 0, errors.New("not implemented")
}

func centroidCacheFor(t *testing.T, pred string, centroids [][]float32) *fakeCache {
	t.Helper()
	data, err := json.Marshal(centroids)
	if err != nil {
		t.Fatalf("marshal centroids: %v", err)
	}
	key := x.DataKey(hnsw.ConcatStrings(pred, CentroidPrefix), 1)
	return &fakeCache{data: map[string][]byte{string(key): data}}
}

func newTestKmeans(seeds [][]float32) *Kmeans[float32] {
	km := CreateKMeans[float32](32, "0-pred", len(seeds), 1,
		hnsw.EuclideanDistanceSq[float32]).(*Kmeans[float32])
	for _, s := range seeds {
		km.AddSeedVector(s)
	}
	return km
}

// TestUpdateCentroidsEmptyCluster pins that a cluster which receives no
// vectors during a pass keeps its previous centroid instead of becoming NaN
// (division by a zero count).
func TestUpdateCentroidsEmptyCluster(t *testing.T) {
	seeds := [][]float32{
		{0, 0},
		{10, 10},
		{100, 100},
	}
	km := newTestKmeans(seeds)
	km.StartBuildPass()

	// Assign vectors near the first two centroids only; centroid 2 stays empty.
	for _, v := range [][]float32{{1, 1}, {-1, -1}, {9, 9}, {11, 11}} {
		if err := km.AddVector(v); err != nil {
			t.Fatalf("AddVector: %v", err)
		}
	}
	km.EndBuildPass()

	centroids := km.GetCentroids()
	if len(centroids) != 3 {
		t.Fatalf("expected 3 centroids, got %d", len(centroids))
	}
	for i, c := range centroids {
		for j, val := range c {
			if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
				t.Fatalf("centroid %d component %d is not finite: %v", i, j, val)
			}
		}
	}
	// The empty cluster keeps its previous centroid.
	if centroids[2][0] != 100 || centroids[2][1] != 100 {
		t.Fatalf("empty cluster centroid changed: %v", centroids[2])
	}
	// The populated clusters move to the mean of their assigned vectors.
	if centroids[0][0] != 0 || centroids[0][1] != 0 {
		t.Fatalf("cluster 0 centroid expected (0,0), got %v", centroids[0])
	}
	if centroids[1][0] != 10 || centroids[1][1] != 10 {
		t.Fatalf("cluster 1 centroid expected (10,10), got %v", centroids[1])
	}
}

// TestNumSeedVectorsMatchesNumClusters pins the seed/cluster coupling: the
// number of seed vectors the build collects must equal numClusters, and
// insert routing must never return an index outside [0, numClusters).
func TestNumSeedVectorsMatchesNumClusters(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	for _, n := range []int{1, 7, 100} {
		km := CreateKMeans[float32](32, "0-pred", n, 1,
			hnsw.EuclideanDistanceSq[float32]).(*Kmeans[float32])
		if km.NumSeedVectors() != n {
			t.Fatalf("numClusters=%d: NumSeedVectors()=%d, want %d", n, km.NumSeedVectors(), n)
		}
		for range n {
			km.AddSeedVector([]float32{rng.Float32() * 100, rng.Float32() * 100})
		}
		km.StartBuildPass()
		for range 50 {
			vec := []float32{rng.Float32() * 100, rng.Float32() * 100}
			if err := km.AddVector(vec); err != nil {
				t.Fatalf("numClusters=%d: AddVector: %v", n, err)
			}
		}
		km.EndBuildPass()
		for range 20 {
			vec := []float32{rng.Float32() * 100, rng.Float32() * 100}
			idx, err := km.FindIndexForInsert(nil, vec)
			if err != nil {
				t.Fatalf("numClusters=%d: FindIndexForInsert: %v", n, err)
			}
			if idx < 0 || idx >= n {
				t.Fatalf("numClusters=%d: routed to cluster %d, out of range", n, idx)
			}
		}
	}
}

// TestKmeansConvergesOnBlobs runs the full multi-pass protocol on three
// well-separated Gaussian blobs and asserts each trained centroid lands
// inside a distinct blob.
func TestKmeansConvergesOnBlobs(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	means := [][]float32{{0, 0}, {50, 50}, {100, 0}}
	blob := func(m []float32) []float32 {
		return []float32{
			m[0] + float32(rng.NormFloat64()),
			m[1] + float32(rng.NormFloat64()),
		}
	}
	var points [][]float32
	for i := range 300 {
		points = append(points, blob(means[i%3]))
	}

	km := CreateKMeans[float32](32, "0-pred", 3, 1,
		hnsw.EuclideanDistanceSq[float32]).(*Kmeans[float32])
	// Seeds: one point per blob (the rebuild pass would feed arbitrary
	// vectors; picking spread seeds keeps the test deterministic).
	for i := range 3 {
		km.AddSeedVector(points[i])
	}
	for range km.NumPasses() {
		km.StartBuildPass()
		for _, p := range points {
			if err := km.AddVector(p); err != nil {
				t.Fatalf("AddVector: %v", err)
			}
		}
		km.EndBuildPass()
	}

	centroids := km.GetCentroids()
	if len(centroids) != 3 {
		t.Fatalf("expected 3 centroids, got %d", len(centroids))
	}
	used := map[int]bool{}
	for _, c := range centroids {
		best, bestDist := -1, math.MaxFloat64
		for i, m := range means {
			dx := float64(c[0] - m[0])
			dy := float64(c[1] - m[1])
			d := dx*dx + dy*dy
			if d < bestDist {
				best, bestDist = i, d
			}
		}
		if bestDist > 25 { // within 5 units of a blob mean
			t.Fatalf("centroid %v is not near any blob mean (dist²=%f)", c, bestDist)
		}
		if used[best] {
			t.Fatalf("two centroids converged to the same blob %d: %v", best, centroids)
		}
		used[best] = true
	}
}

// TestRoutingHydratesFromDisk pins the restart path: a fresh Kmeans instance
// with no in-memory centroids must load the persisted set through the cache
// and route searches and inserts by it — not fall back to cluster 0.
func TestRoutingHydratesFromDisk(t *testing.T) {
	centroids := [][]float32{{0, 0}, {50, 50}, {100, 0}}
	cache := centroidCacheFor(t, "0-pred", centroids)

	km := CreateKMeans[float32](32, "0-pred", 3, 2,
		hnsw.EuclideanDistanceSq[float32]).(*Kmeans[float32])

	// Insert routing: nearest centroid wins.
	idx, err := km.FindIndexForInsert(cache, []float32{99, 1})
	if err != nil {
		t.Fatalf("FindIndexForInsert: %v", err)
	}
	if idx != 2 {
		t.Fatalf("expected insert routed to cluster 2, got %d", idx)
	}

	// Search routing: numProbes=2 nearest clusters, must include the best one.
	probes, err := km.FindIndexForSearch(cache, []float32{1, 1})
	if err != nil {
		t.Fatalf("FindIndexForSearch: %v", err)
	}
	if len(probes) != 2 {
		t.Fatalf("expected 2 probes (numProbes=2), got %v", probes)
	}
	found := false
	for _, p := range probes {
		if p == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("probes %v do not include the nearest cluster 0", probes)
	}

	// Hydration happens once: further routing calls must not re-read disk.
	gets := cache.gets
	if _, err := km.FindIndexForInsert(cache, []float32{1, 2}); err != nil {
		t.Fatalf("FindIndexForInsert: %v", err)
	}
	if cache.gets != gets {
		t.Fatalf("expected no further cache reads after hydration, got %d extra", cache.gets-gets)
	}
}

// TestRoutingWithoutPersistedCentroids pins cluster-0 mode: when nothing was
// ever built for the predicate, inserts go to cluster 0 and searches probe
// only cluster 0 — consistently.
func TestRoutingWithoutPersistedCentroids(t *testing.T) {
	cache := &fakeCache{data: map[string][]byte{}}
	km := CreateKMeans[float32](32, "0-pred", 8, 3,
		hnsw.EuclideanDistanceSq[float32]).(*Kmeans[float32])

	idx, err := km.FindIndexForInsert(cache, []float32{5, 5})
	if err != nil {
		t.Fatalf("FindIndexForInsert: %v", err)
	}
	if idx != 0 {
		t.Fatalf("expected cluster-0 mode for unbuilt index, got %d", idx)
	}
	probes, err := km.FindIndexForSearch(cache, []float32{5, 5})
	if err != nil {
		t.Fatalf("FindIndexForSearch: %v", err)
	}
	if len(probes) != 1 || probes[0] != 0 {
		t.Fatalf("expected probes [0] for unbuilt index, got %v", probes)
	}
	// The miss is cached: no disk read storm on every call.
	gets := cache.gets
	if _, err := km.FindIndexForInsert(cache, []float32{5, 5}); err != nil {
		t.Fatalf("FindIndexForInsert: %v", err)
	}
	if cache.gets != gets {
		t.Fatalf("expected the hydration miss to be cached, got %d extra reads", cache.gets-gets)
	}
}

// TestFindNClosestCentroids pins the top-n selection used for search routing.
func TestFindNClosestCentroids(t *testing.T) {
	centroids := [][]float32{{0, 0}, {10, 0}, {20, 0}, {30, 0}}
	cache := centroidCacheFor(t, "0-pred", centroids)
	km := CreateKMeans[float32](32, "0-pred", 4, 2,
		hnsw.EuclideanDistanceSq[float32]).(*Kmeans[float32])

	probes, err := km.FindIndexForSearch(cache, []float32{11, 0})
	if err != nil {
		t.Fatalf("FindIndexForSearch: %v", err)
	}
	got := map[int]bool{}
	for _, p := range probes {
		got[p] = true
	}
	if len(probes) != 2 || !got[1] || !got[2] {
		t.Fatalf("expected the two nearest clusters {1,2}, got %v", probes)
	}
}

// TestUpdateCentroidsWeightsResetAfterEmptyPass pins that an empty cluster's
// accumulated weights are cleared so the next pass starts fresh.
func TestUpdateCentroidsWeightsResetAfterEmptyPass(t *testing.T) {
	seeds := [][]float32{
		{0, 0},
		{10, 10},
	}
	km := newTestKmeans(seeds)
	km.StartBuildPass()
	if err := km.AddVector([]float32{1, 1}); err != nil {
		t.Fatalf("AddVector: %v", err)
	}
	km.EndBuildPass()

	// Second pass: both clusters get vectors; results must reflect only this
	// pass's assignments.
	km.StartBuildPass()
	for _, v := range [][]float32{{2, 2}, {8, 8}} {
		if err := km.AddVector(v); err != nil {
			t.Fatalf("AddVector: %v", err)
		}
	}
	km.EndBuildPass()

	centroids := km.GetCentroids()
	if centroids[0][0] != 2 || centroids[0][1] != 2 {
		t.Fatalf("cluster 0 centroid expected (2,2), got %v", centroids[0])
	}
	if centroids[1][0] != 8 || centroids[1][1] != 8 {
		t.Fatalf("cluster 1 centroid expected (8,8), got %v", centroids[1])
	}
}
