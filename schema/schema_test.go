/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package schema

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/kmeans"
)

func TestPredicatesToDelete(t *testing.T) {
	const pred = "v"
	tests := []struct {
		name   string
		schema *pb.SchemaUpdate
		expect []string
	}{
		{
			name:   "non-vector predicate",
			schema: &pb.SchemaUpdate{Predicate: pred, ValueType: pb.Posting_STRING},
			expect: []string{pred},
		},
		{
			name: "monolithic vector predicate",
			schema: &pb.SchemaUpdate{
				Predicate:  pred,
				ValueType:  pb.Posting_VFLOAT,
				IndexSpecs: []*pb.VectorIndexSpec{{Name: "hnsw"}},
			},
			expect: []string{
				pred,
				pred + hnsw.VecEntry, pred + hnsw.VecKeyword, pred + hnsw.VecDead,
				pred + hnsw.VecMeta,
			},
		},
		{
			// Regression: dropping a partitioned index must also remove the
			// dimension metadata, every per-cluster split attr, and the centroid
			// set — otherwise they leak (orphaned data + a stale dimension that
			// rejects re-inserts of a different length).
			name: "partitioned vector predicate",
			schema: &pb.SchemaUpdate{
				Predicate: pred,
				ValueType: pb.Posting_VFLOAT,
				IndexSpecs: []*pb.VectorIndexSpec{{
					Name:    "hnsw",
					Options: []*pb.OptionPair{{Key: "numClusters", Value: "2"}},
				}},
			},
			expect: []string{
				pred,
				pred + hnsw.VecEntry, pred + hnsw.VecKeyword, pred + hnsw.VecDead,
				pred + hnsw.VecMeta,
				hnsw.SplitEntryAttr(pred, 0), hnsw.SplitVecAttr(pred, 0), hnsw.SplitDeadAttr(pred, 0),
				hnsw.SplitEntryAttr(pred, 1), hnsw.SplitVecAttr(pred, 1), hnsw.SplitDeadAttr(pred, 1),
				pred + kmeans.CentroidPrefix,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &state{predicate: map[string]*pb.SchemaUpdate{pred: tc.schema}}
			require.ElementsMatch(t, tc.expect, s.PredicatesToDelete(pred))
		})
	}
}

func TestCompareSchemaUpdates(t *testing.T) {
	tests := []struct {
		name     string
		original *pb.SchemaUpdate
		update   *pb.SchemaUpdate
		expected []string
	}{
		{
			name: "No changes",
			original: &pb.SchemaUpdate{
				Predicate: "name",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			update: &pb.SchemaUpdate{
				Predicate: "name",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			expected: []string(nil),
		},
		{
			name: "Predicate changed",
			original: &pb.SchemaUpdate{
				Predicate: "name",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			update: &pb.SchemaUpdate{
				Predicate: "age",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			expected: []string{"Predicate"},
		},
		{
			name: "Multiple fields changed",
			original: &pb.SchemaUpdate{
				Predicate: "name",
				ValueType: pb.Posting_STRING,
				Count:     true,
			},
			update: &pb.SchemaUpdate{
				Predicate: "age",
				ValueType: pb.Posting_STRING,
				Count:     false,
			},
			expected: []string{"Predicate", "Count"},
		},
		{
			name: "Unchanged and changed fields",
			original: &pb.SchemaUpdate{
				Predicate: "name",
				Count:     true,
				Upsert:    false,
			},
			update: &pb.SchemaUpdate{
				Predicate: "name",
				Count:     true,
				Upsert:    true,
			},
			expected: []string{"Upsert"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, compareSchemaUpdates(tt.original, tt.update))
		})
	}
}
