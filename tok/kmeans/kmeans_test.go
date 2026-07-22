/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package kmeans

import (
	"math"
	"testing"

	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
)

func newTestKmeans(seeds [][]float32) *Kmeans[float32] {
	km := CreateKMeans[float32](32, "0-pred", hnsw.EuclideanDistanceSq[float32]).(*Kmeans[float32])
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
