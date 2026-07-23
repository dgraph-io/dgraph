/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package posting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/kmeans"
	"github.com/dgraph-io/dgraph/v25/tok/partitioned_hnsw"
	"github.com/dgraph-io/dgraph/v25/x"
)

func partitionedSchema(attr string, numClusters string) *pb.SchemaUpdate {
	return &pb.SchemaUpdate{
		Predicate: attr,
		ValueType: pb.Posting_VFLOAT,
		Directive: pb.SchemaUpdate_INDEX,
		IndexSpecs: []*pb.VectorIndexSpec{{
			Name: partitioned_hnsw.PartitionedHNSW,
			Options: []*pb.OptionPair{
				{Key: partitioned_hnsw.NumClustersOpt, Value: numClusters},
			},
		}},
	}
}

// TestDropPrefixesCoverPartitionedClusters pins the rebuild-hygiene fix:
// dropping a partitioned index must cover every per-cluster aux predicate of
// both the old and the new layout, plus the persisted centroid key. Split
// attrs are distinct predicates (length-prefixed keys), so the unsplit
// prefixes do not cover them.
func TestDropPrefixesCoverPartitionedClusters(t *testing.T) {
	attr := x.AttrInRootNamespace("vecpred")
	rb := &IndexRebuild{
		Attr:          attr,
		OldSchema:     partitionedSchema(attr, "8"),
		CurrentSchema: partitionedSchema(attr, "4"),
	}

	prefixes := prefixesToDropVectorIndexEdges(context.Background(), rb)
	require.NotEmpty(t, prefixes)

	covered := make(map[string]bool, len(prefixes))
	for _, p := range prefixes {
		covered[string(p)] = true
	}
	requireCovered := func(subAttr string) {
		require.Truef(t, covered[string(x.PredicatePrefix(subAttr))],
			"prefix for %q not dropped", subAttr)
	}

	// Old layout had 8 clusters: every one of them must be dropped,
	// including 4..7 which the new layout no longer has.
	for i := range 8 {
		requireCovered(hnsw.SplitEntryAttr(attr, i))
		requireCovered(hnsw.SplitVecAttr(attr, i))
		requireCovered(hnsw.SplitDeadAttr(attr, i))
	}
	// The persisted centroid set must go too.
	requireCovered(hnsw.ConcatStrings(attr, kmeans.CentroidPrefix))
	// Legacy unsplit prefixes stay covered (hnsw <-> partitioned moves).
	requireCovered(hnsw.ConcatStrings(attr, hnsw.VecEntry))
	requireCovered(hnsw.ConcatStrings(attr, hnsw.VecDead))
	requireCovered(hnsw.ConcatStrings(attr, hnsw.VecKeyword))
}

// TestNumClustersChangeTriggersRebuild pins the factory-identity fix: a
// numClusters change alters the on-disk layout, so the schema diff must
// register as a rebuild — and an identical re-apply must stay a no-op.
func TestNumClustersChangeTriggersRebuild(t *testing.T) {
	attr := x.AttrInRootNamespace("vecpred")

	rb := &IndexRebuild{
		Attr:          attr,
		OldSchema:     partitionedSchema(attr, "8"),
		CurrentSchema: partitionedSchema(attr, "4"),
	}
	require.EqualValues(t, indexRebuild, rb.needsVectorIndexEdgesRebuild(),
		"numClusters change must trigger a rebuild")

	same := &IndexRebuild{
		Attr:          attr,
		OldSchema:     partitionedSchema(attr, "8"),
		CurrentSchema: partitionedSchema(attr, "8"),
	}
	require.EqualValues(t, indexNoop, same.needsVectorIndexEdgesRebuild(),
		"identical schema re-apply must be a no-op")
}
