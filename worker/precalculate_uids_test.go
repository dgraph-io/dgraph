/*
 * SPDX-FileCopyrightText: 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
)

func TestShouldPrecalculateUids(t *testing.T) {
	tests := []struct {
		name       string
		q          *pb.Query
		srcFn      *functionContext
		facetsTree *facetsTree
		opts       posting.ListOptions
		want       bool
	}{
		{
			name:  "allows unbounded uid reads",
			q:     &pb.Query{},
			srcFn: &functionContext{fnType: standardFn},
			want:  true,
		},
		{
			name:  "allows internal unbounded first sentinel",
			q:     &pb.Query{},
			srcFn: &functionContext{fnType: standardFn},
			opts:  posting.ListOptions{First: math.MaxInt32},
			want:  true,
		},
		{
			name:  "skips bounded first reads",
			q:     &pb.Query{},
			srcFn: &functionContext{fnType: standardFn},
			opts:  posting.ListOptions{First: 10},
		},
		{
			name:  "skips intersect reads",
			q:     &pb.Query{},
			srcFn: &functionContext{fnType: standardFn},
			opts:  posting.ListOptions{Intersect: &pb.List{Uids: []uint64{1}}},
		},
		{
			name:  "skips count reads",
			q:     &pb.Query{DoCount: true},
			srcFn: &functionContext{fnType: standardFn},
		},
		{
			name:  "skips facet reads",
			q:     &pb.Query{FacetParam: &pb.FacetParams{}},
			srcFn: &functionContext{fnType: standardFn},
		},
		{
			name:       "skips facet filter reads",
			q:          &pb.Query{},
			srcFn:      &functionContext{fnType: standardFn},
			facetsTree: &facetsTree{},
		},
		{
			name:  "skips compare scalar reads",
			q:     &pb.Query{},
			srcFn: &functionContext{fnType: compareScalarFn},
		},
		{
			name:  "skips has function reads",
			q:     &pb.Query{},
			srcFn: &functionContext{fnType: hasFn},
		},
		{
			name:  "skips uid in reads",
			q:     &pb.Query{},
			srcFn: &functionContext{fnType: uidInFn},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldPrecalculateUids(tc.q, tc.srcFn, tc.facetsTree, tc.opts))
		})
	}
}
