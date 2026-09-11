/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package partitioned_hnsw

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/tok/kmeans"
	opt "github.com/dgraph-io/dgraph/v25/tok/options"
	"github.com/dgraph-io/dgraph/v25/x"
)

// fakeCache is a minimal index.CacheType serving a fixed key/value map.
type fakeCache struct {
	data map[string][]byte
}

func (f *fakeCache) Get(key []byte) ([]byte, error) {
	if v, ok := f.data[string(key)]; ok {
		return v, nil
	}
	return nil, errors.New("no value found")
}

func (f *fakeCache) Ts() uint64 { return 1 }

func (f *fakeCache) Find([]byte, func([]byte) bool) (uint64, error) {
	return 0, errors.New("not implemented")
}

// TestDeadAttrForVectorMatchesInsertRouting pins the delete-routing contract:
// the dead list a delete writes to must belong to the exact cluster the
// insert routed the vector into, or partitioned searches keep returning
// deleted uids.
func TestDeadAttrForVectorMatchesInsertRouting(t *testing.T) {
	const pred = "0-embedding"
	centroids := [][]float32{{0, 0}, {50, 50}, {100, 0}}
	data, err := json.Marshal(centroids)
	if err != nil {
		t.Fatalf("marshal centroids: %v", err)
	}
	key := x.DataKey(hnsw.ConcatStrings(pred, kmeans.CentroidPrefix), 1)
	cache := &fakeCache{data: map[string][]byte{string(key): data}}

	o := opt.NewOptions()
	o.SetOpt(NumClustersOpt, 3)
	f := CreateFactory[float32](32)
	vi, err := f.FindOrCreate(pred, o, 32)
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	ph := vi.(*partitionedHNSW[float32])

	resolver, ok := vi.(index.VectorDeadListResolver[float32])
	if !ok {
		t.Fatal("partitionedHNSW must implement VectorDeadListResolver")
	}

	for _, vec := range [][]float32{{1, 1}, {49, 51}, {99, 2}, {60, 40}} {
		wantIdx, err := ph.partition.FindIndexForInsert(cache, vec)
		if err != nil {
			t.Fatalf("FindIndexForInsert(%v): %v", vec, err)
		}
		deadAttr, err := resolver.DeadAttrForVector(cache, vec)
		if err != nil {
			t.Fatalf("DeadAttrForVector(%v): %v", vec, err)
		}
		if want := hnsw.SplitDeadAttr(pred, wantIdx); deadAttr != want {
			t.Fatalf("vector %v: delete routed to %q, insert routed to cluster %d (%q)",
				vec, deadAttr, wantIdx, want)
		}
	}
}

// TestSetDimensionDoesNotMutateSchema pins that SetDimension records the
// dimension on the instance only and never appends a vectorDimension option
// into the schema — otherwise derived state (including a "-1" for empty
// predicates, and duplicates across rebuilds) would leak into schema queries
// and exports.
func TestSetDimensionDoesNotMutateSchema(t *testing.T) {
	pred := "0-vec"
	o := opt.NewOptions()
	o.SetOpt(NumClustersOpt, 3)
	f := CreateFactory[float32](32)
	vi, err := f.FindOrCreate(pred, o, 32)
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}

	schema := &pb.SchemaUpdate{
		Predicate: pred,
		ValueType: pb.Posting_VFLOAT,
		Directive: pb.SchemaUpdate_INDEX,
		IndexSpecs: []*pb.VectorIndexSpec{{
			Name:    "hnsw",
			Options: []*pb.OptionPair{{Key: NumClustersOpt, Value: "3"}},
		}},
	}
	before := len(schema.IndexSpecs[0].Options)

	vi.SetDimension(schema, 10)

	if got := len(schema.IndexSpecs[0].Options); got != before {
		t.Fatalf("SetDimension mutated schema options: had %d, now %d", before, got)
	}
	if SpecHasOption(schema.IndexSpecs[0], VectorDimensionOpt) {
		t.Fatal("SetDimension leaked a vectorDimension option into the schema")
	}
	if got := vi.Dimension(); got != 10 {
		t.Fatalf("Dimension() = %d, want 10", got)
	}
}
