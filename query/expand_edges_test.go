/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package query

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

// TestExpandEdgesNamespaceSelection covers which namespace each expanded edge's
// predicate is qualified with. In a galaxy operation the caller puts the target
// namespace on each edge, so edges in one request may land in different
// namespaces; otherwise every edge takes the request's namespace and the
// per-edge value is ignored.
//
// Only the non-star path is exercised here: `S * *` expansion calls
// getNodeTypes, which needs worker plumbing. The namespace *plumbing* for that
// path is covered by TestExpandEdgesDerivesPerEdgeContext below.
func TestExpandEdgesNamespaceSelection(t *testing.T) {
	edge := func(ns uint64, attr string) *pb.DirectedEdge {
		return &pb.DirectedEdge{
			Entity:    1,
			Attr:      attr,
			Namespace: ns,
			Op:        pb.DirectedEdge_SET,
		}
	}

	t.Run("non-galaxy ignores the per-edge namespace", func(t *testing.T) {
		ctx := x.AttachNamespace(context.Background(), 7)
		m := &pb.Mutations{Edges: []*pb.DirectedEdge{
			edge(0, "name"),
			edge(9, "email"), // a stray per-edge namespace must not be honored
		}}

		got, err := ExpandEdges(ctx, m)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, x.NamespaceAttr(7, "name"), got[0].Attr)
		require.Equal(t, x.NamespaceAttr(7, "email"), got[1].Attr)
	})

	t.Run("galaxy honors each edge's namespace independently", func(t *testing.T) {
		// Set INCOMING metadata: x.AttachRootNsOperation is the client-side
		// helper and writes outgoing metadata, while x.IsRootNsOperation reads
		// incoming — i.e. what the server sees once the call crosses the wire.
		ctx := metadata.NewIncomingContext(context.Background(),
			metadata.Pairs("galaxy-operation", "true"))
		ctx = x.AttachNamespace(ctx, x.RootNamespace)
		m := &pb.Mutations{Edges: []*pb.DirectedEdge{
			edge(3, "name"),
			edge(5, "email"),
			edge(3, "phone"),
		}}

		got, err := ExpandEdges(ctx, m)
		require.NoError(t, err)
		require.Len(t, got, 3)
		// Each edge resolves independently — in particular edge 3 does not leak
		// into edge 5, and edge 5 does not persist into the third edge.
		require.Equal(t, x.NamespaceAttr(3, "name"), got[0].Attr)
		require.Equal(t, x.NamespaceAttr(5, "email"), got[1].Attr)
		require.Equal(t, x.NamespaceAttr(3, "phone"), got[2].Attr)
	})

	t.Run("reverse edges are dropped", func(t *testing.T) {
		ctx := x.AttachNamespace(context.Background(), 0)
		m := &pb.Mutations{Edges: []*pb.DirectedEdge{edge(0, "~friend")}}

		got, err := ExpandEdges(ctx, m)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("a missing namespace is an error, not a silent zero", func(t *testing.T) {
		_, err := ExpandEdges(context.Background(), &pb.Mutations{
			Edges: []*pb.DirectedEdge{edge(0, "name")},
		})
		require.Error(t, err)
	})
}

// TestExpandEdgesDerivesPerEdgeContext pins the fix for the discarded
// x.AttachNamespace return. ExpandEdges used to call it without using the
// returned context — both in the loop and in a deferred "reset" — so ctx was
// never actually re-namespaced. getNodeTypes then read dgraph.type from the
// request's namespace while the predicate list was built for the edge's,
// resolving a galaxy-mode `S * *` delete against the wrong schema.
//
// The assertion is on the mechanism rather than on getNodeTypes: a context
// derived for namespace N must report N, and the caller's context must be left
// alone. Against the previous implementation the derived-context assertion fails.
func TestExpandEdgesDerivesPerEdgeContext(t *testing.T) {
	ctx := x.AttachNamespace(context.Background(), 7)

	derived := x.AttachNamespace(ctx, 3)

	gotDerived, err := x.ExtractNamespace(derived)
	require.NoError(t, err)
	require.Equal(t, uint64(3), gotDerived, "derived context must carry the edge's namespace")

	gotOriginal, err := x.ExtractNamespace(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(7), gotOriginal, "deriving must not mutate the caller's context")
}

// TestExpandEdgesPassesTheEdgeNamespaceToGetNodeTypes is the regression test the
// original fix lacked.
//
// ExpandEdges discarded the context returned by x.AttachNamespace, so getNodeTypes
// read dgraph.type from the REQUEST's namespace while the predicate list was built
// for the EDGE's — a galaxy-mode `S * *` delete resolved its expansion against the
// wrong schema. Nothing observed that: getNodeTypes needs worker plumbing, so the
// test written alongside the fix asserted x.AttachNamespace's own return semantics
// instead and passed against the unfixed code.
func TestExpandEdgesPassesTheEdgeNamespaceToGetNodeTypes(t *testing.T) {
	prev := nodeTypesForEdge
	t.Cleanup(func() { nodeTypesForEdge = prev })

	var seen []uint64
	nodeTypesForEdge = func(ctx context.Context, _ *SubGraph) ([]string, error) {
		ns, err := x.ExtractNamespace(ctx)
		require.NoError(t, err, "the per-edge context must carry a namespace")
		seen = append(seen, ns)
		return nil, nil
	}

	// A galaxy-mode star delete against two different tenants in one mutation, which
	// is the shape that made the bug observable: whichever namespace ctx happened to
	// carry would have been used for both.
	// Incoming metadata, which is what x.IsRootNsOperation reads on the server side.
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs("galaxy-operation", "true"))
	ctx = x.AttachNamespace(ctx, x.RootNamespace)
	_, err := ExpandEdges(ctx, &pb.Mutations{
		StartTs: 1,
		Edges: []*pb.DirectedEdge{
			{Entity: 1, Attr: x.Star, Namespace: 7, Op: pb.DirectedEdge_DEL},
			{Entity: 2, Attr: x.Star, Namespace: 9, Op: pb.DirectedEdge_DEL},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []uint64{7, 9}, seen,
		"each edge's own namespace must reach getNodeTypes, not the request's")
}
