/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/dgraph/v25/dql"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

// These tests cover the query-layer half of similar_to(..., filter: <var>): how the
// filter variable is turned into the allow-set, that it never leaks into DestUIDs,
// and that the allow-set only ever replaces UidList for a similar_to function.

func valueVar(uids ...uint64) varValue {
	m := types.NewShardedMap()
	for _, u := range uids {
		m.Set(u, types.Val{Tid: types.IntID, Value: int64(u)})
	}
	return varValue{Vals: m}
}

func TestVectorFilterAllowSet_UidVar(t *testing.T) {
	l := &pb.List{Uids: []uint64{3, 7, 9}}
	got, err := vectorFilterAllowSet("v", varValue{Uids: l, Vals: types.NewShardedMap()})
	require.NoError(t, err)
	require.Same(t, l, got, "a uid variable's list is used as-is")
}

func TestVectorFilterAllowSet_EmptyUidVar(t *testing.T) {
	got, err := vectorFilterAllowSet("v", varValue{Uids: &pb.List{}, Vals: types.NewShardedMap()})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got.GetUids(), "an empty uid variable is an explicit empty scope")
}

func TestVectorFilterAllowSet_ValueVarUsesKeys(t *testing.T) {
	// Same reading uid(var) gives a value variable: the uids it is keyed by, sorted.
	got, err := vectorFilterAllowSet("v", valueVar(42, 5, 17))
	require.NoError(t, err)
	require.Equal(t, []uint64{5, 17, 42}, got.GetUids())
}

func TestVectorFilterAllowSet_UnresolvedVarIsEmptyScope(t *testing.T) {
	// A variable that resolved to nothing (no uids, no values) must still produce a
	// non-nil empty list so the worker sees "filter requested, scope empty".
	got, err := vectorFilterAllowSet("v", varValue{Vals: types.NewShardedMap()})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got.GetUids())

	got, err = vectorFilterAllowSet("v", varValue{})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got.GetUids())
}

func TestVectorFilterAllowSet_ListVarErrors(t *testing.T) {
	// expand(val(x))-style list variables do not name a uid set.
	_, err := vectorFilterAllowSet("v", varValue{
		Vals:    types.NewShardedMap(),
		strList: []*pb.ValueList{{}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Wrong variable type")
	require.Contains(t, err.Error(), "var(v)")
}

func prefilterSubGraph(fn string) *SubGraph {
	return &SubGraph{
		Attr: "emb",
		Params: params{
			NeedsVar:        []dql.VarContext{{Name: "allowed", Typ: dql.UidVar}},
			VectorFilterVar: "allowed",
		},
		SrcFunc: &Function{
			Name: fn,
			Args: []dql.Arg{{Value: "5"}, {Value: "[0.0, 0.0]"}, {Value: "filter"}, {Value: "1"}},
		},
	}
}

func TestFillVars_FilterVarBecomesAllowSetNotDestUIDs(t *testing.T) {
	sg := prefilterSubGraph("similar_to")
	allow := &pb.List{Uids: []uint64{10, 20, 30}}
	require.NoError(t, sg.fillVars(map[string]varValue{
		"allowed": {Uids: allow, Vals: types.NewShardedMap()},
	}))
	require.Equal(t, allow.GetUids(), sg.vectorFilterUids.GetUids())
	require.Empty(t, sg.DestUIDs.GetUids(),
		"the filter var restricts the search; it must not seed the result set")
	// The marker args are untouched by variable substitution.
	require.Equal(t, "filter", sg.SrcFunc.Args[2].Value)
	require.Equal(t, "1", sg.SrcFunc.Args[3].Value)
}

func TestFillVars_FilterValueVarDerivesUids(t *testing.T) {
	sg := prefilterSubGraph("similar_to")
	require.NoError(t, sg.fillVars(map[string]varValue{"allowed": valueVar(9, 4)}))
	require.Equal(t, []uint64{4, 9}, sg.vectorFilterUids.GetUids())
	require.Empty(t, sg.DestUIDs.GetUids())
}

func TestFillVars_FilterEmptyVarIsExplicitEmptyScope(t *testing.T) {
	sg := prefilterSubGraph("similar_to")
	require.NoError(t, sg.fillVars(map[string]varValue{
		"allowed": {Uids: &pb.List{}, Vals: types.NewShardedMap()},
	}))
	require.NotNil(t, sg.vectorFilterUids)
	require.Empty(t, sg.vectorFilterUids.GetUids())
}

func TestFillVars_FilterListVarErrors(t *testing.T) {
	sg := prefilterSubGraph("similar_to")
	err := sg.fillVars(map[string]varValue{
		"allowed": {Vals: types.NewShardedMap(), strList: []*pb.ValueList{{}}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Wrong variable type")
}

func TestFillVars_FilterVarUnresolvedLeavesAllowSetNil(t *testing.T) {
	// If the scheduler has not produced the var yet, fillVars leaves the allow-set
	// alone (nil) exactly as it does for every other variable.
	sg := prefilterSubGraph("similar_to")
	require.NoError(t, sg.fillVars(map[string]varValue{}))
	require.Nil(t, sg.vectorFilterUids)
}

func TestFillVars_VectorFilterVarIgnoredForOtherFunctions(t *testing.T) {
	// VectorFilterVar is only meaningful on similar_to. On any other function the
	// same NeedsVar entry follows the ordinary uid-var path into DestUIDs.
	sg := prefilterSubGraph("eq")
	allow := &pb.List{Uids: []uint64{1, 2}}
	require.NoError(t, sg.fillVars(map[string]varValue{
		"allowed": {Uids: allow, Vals: types.NewShardedMap()},
	}))
	require.Nil(t, sg.vectorFilterUids)
	require.Equal(t, []uint64{1, 2}, sg.DestUIDs.GetUids())
}

func nsCtx() context.Context {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs())
	return x.AttachNamespace(ctx, x.RootNamespace)
}

func TestCreateTaskQuery_AllowSetRidesInUidListForSimilarTo(t *testing.T) {
	sg := prefilterSubGraph("similar_to")
	sg.vectorFilterUids = &pb.List{Uids: []uint64{7, 8}}
	q, err := createTaskQuery(nsCtx(), sg)
	require.NoError(t, err)
	require.Equal(t, []uint64{7, 8}, q.UidList.GetUids())
	require.Equal(t, []string{"5", "[0.0, 0.0]", "filter", "1"}, q.SrcFunc.Args)
}

func TestCreateTaskQuery_AllowSetWinsOverSrcUIDsInsideFilter(t *testing.T) {
	// similar_to inside @filter has SrcUIDs (the parent's uids). The worker's
	// similar_to path never reads UidList as source uids, so the allow-set may take
	// the field; the parent uids are applied when the filter result is merged.
	sg := prefilterSubGraph("similar_to")
	sg.SrcUIDs = &pb.List{Uids: []uint64{1, 2, 3}}
	sg.vectorFilterUids = &pb.List{Uids: []uint64{2}}
	q, err := createTaskQuery(nsCtx(), sg)
	require.NoError(t, err)
	require.Equal(t, []uint64{2}, q.UidList.GetUids())
}

func TestCreateTaskQuery_EmptyAllowSetIsSentExplicitly(t *testing.T) {
	sg := prefilterSubGraph("similar_to")
	sg.vectorFilterUids = &pb.List{}
	q, err := createTaskQuery(nsCtx(), sg)
	require.NoError(t, err)
	require.NotNil(t, q.UidList)
	require.Empty(t, q.UidList.GetUids())
}

func TestCreateTaskQuery_NoAllowSetKeepsSrcUIDs(t *testing.T) {
	sg := prefilterSubGraph("similar_to")
	sg.SrcUIDs = &pb.List{Uids: []uint64{1, 2, 3}}
	q, err := createTaskQuery(nsCtx(), sg)
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 2, 3}, q.UidList.GetUids())
}

func TestCreateTaskQuery_AllowSetNeverReplacesSrcUIDsForOtherFunctions(t *testing.T) {
	// Guard against a stale allow-set on a non-similar_to SubGraph.
	sg := prefilterSubGraph("eq")
	sg.SrcUIDs = &pb.List{Uids: []uint64{1, 2, 3}}
	sg.vectorFilterUids = &pb.List{Uids: []uint64{9}}
	q, err := createTaskQuery(nsCtx(), sg)
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 2, 3}, q.UidList.GetUids())
}
