/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"context"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
)

func TestReverseEdge(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("revc: [uid] @reverse @count ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.AttrInRootNamespace("revc")

	edge := &pb.DirectedEdge{
		ValueId: 2,
		Attr:    attr,
		Entity:  1,
		Op:      pb.DirectedEdge_DEL,
	}

	x.Check(runMutation(ctx, edge, txn))
	x.Check(runMutation(ctx, edge, txn))

	pl, err := txn.Get(x.DataKey(attr, 1))
	require.NoError(t, err)
	pl.RLock()
	c := pl.GetLength(5)
	pl.RUnlock()
	require.Equal(t, 0, c)
}

func TestReverseEdgeSetDel(t *testing.T) {
	dir, err := os.MkdirTemp("", "storetest_")
	x.Check(err)
	defer os.RemoveAll(dir)

	opt := badger.DefaultOptions(dir)
	ps, err := badger.OpenManaged(opt)
	x.Check(err)
	pstore = ps
	// Not using posting list cache
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("revc: [uid] @reverse @count ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	txn := posting.Oracle().RegisterStartTs(5)
	attr := x.AttrInRootNamespace("revc")

	edgeDel := &pb.DirectedEdge{
		ValueId: 2,
		Attr:    attr,
		Entity:  3,
		Op:      pb.DirectedEdge_DEL,
	}

	edgeSet1 := &pb.DirectedEdge{
		ValueId: 2,
		Attr:    attr,
		Entity:  1,
		Op:      pb.DirectedEdge_SET,
	}

	edgeSet2 := &pb.DirectedEdge{
		ValueId: 2,
		Attr:    attr,
		Entity:  3,
		Op:      pb.DirectedEdge_SET,
	}

	edgeSet3 := &pb.DirectedEdge{
		ValueId: 2,
		Attr:    attr,
		Entity:  4,
		Op:      pb.DirectedEdge_SET,
	}

	x.Check(runMutation(ctx, edgeSet1, txn))
	x.Check(runMutation(ctx, edgeSet2, txn))
	x.Check(runMutation(ctx, edgeSet3, txn))
	x.Check(runMutation(ctx, edgeDel, txn))

	pl, err := txn.Get(x.ReverseKey(attr, 2))
	require.NoError(t, err)
	pl.RLock()
	c := pl.GetLength(5)
	pl.RUnlock()
	require.Equal(t, 2, c)
}

func TestConvertEdgeType(t *testing.T) {
	var testEdges = []struct {
		input     *pb.DirectedEdge
		to        types.TypeID
		expectErr bool
		output    *pb.DirectedEdge
	}{
		{
			input: &pb.DirectedEdge{
				Value: []byte("set edge"),
				Attr:  x.AttrInRootNamespace("name"),
			},
			to:        types.StringID,
			expectErr: false,
			output: &pb.DirectedEdge{
				Value:     []byte("set edge"),
				Attr:      x.AttrInRootNamespace("name"),
				ValueType: 9,
			},
		},
		{
			input: &pb.DirectedEdge{
				Value: []byte("set edge"),
				Attr:  x.NamespaceAttr(0xf2, "name"),
				Op:    pb.DirectedEdge_DEL,
			},
			to:        types.StringID,
			expectErr: false,
			output: &pb.DirectedEdge{
				Value:     []byte("set edge"),
				Attr:      x.NamespaceAttr(0xf2, "name"),
				Op:        pb.DirectedEdge_DEL,
				ValueType: 9,
			},
		},
		{
			input: &pb.DirectedEdge{
				ValueId: 123,
				Attr:    x.AttrInRootNamespace("name"),
			},
			to:        types.StringID,
			expectErr: true,
		},
		{
			input: &pb.DirectedEdge{
				Value: []byte("set edge"),
				Attr:  x.AttrInRootNamespace("name"),
			},
			to:        types.UidID,
			expectErr: true,
		},
	}

	for _, testEdge := range testEdges {
		err := ValidateAndConvert(testEdge.input,
			&pb.SchemaUpdate{
				ValueType: pb.Posting_ValType(testEdge.to),
			})
		if testEdge.expectErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.True(t, reflect.DeepEqual(testEdge.input, testEdge.output))
		}
	}

}

func TestValidateEdgeTypeError(t *testing.T) {
	edge := &pb.DirectedEdge{
		Value: []byte("set edge"),
		Attr:  x.AttrInRootNamespace("name"),
	}

	err := ValidateAndConvert(edge,
		&pb.SchemaUpdate{
			ValueType: pb.Posting_ValType(types.DateTimeID),
		})
	require.Error(t, err)
}

func TestTypeSanityCheck(t *testing.T) {
	// Empty field name check.
	typeDef := &pb.TypeUpdate{
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.AttrInRootNamespace(""),
			},
		},
	}
	err := typeSanityCheck(typeDef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Field in type definition must have a name")

	// Object type without object name.
	typeDef = &pb.TypeUpdate{
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.AttrInRootNamespace("name"),
				ValueType: pb.Posting_OBJECT,
			},
		},
	}
	err = typeSanityCheck(typeDef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Field with value type OBJECT must specify the name")

	// Field with directive.
	typeDef = &pb.TypeUpdate{
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.AttrInRootNamespace("name"),
				Directive: pb.SchemaUpdate_REVERSE,
			},
		},
	}
	err = typeSanityCheck(typeDef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Field in type definition cannot have a directive")

	// Field with tokenizer.
	typeDef = &pb.TypeUpdate{
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.AttrInRootNamespace("name"),
				Tokenizer: []string{"int"},
			},
		},
	}
	err = typeSanityCheck(typeDef)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Field in type definition cannot have tokenizers")
}

func TestHasEdgesAfterDelete(t *testing.T) {
	opt := badger.DefaultOptions(t.TempDir())
	ps, err := badger.OpenManaged(opt)
	require.NoError(t, err)
	prevStore := pstore
	pstore = ps
	t.Cleanup(func() {
		pstore = prevStore
		// posting.Init above rebound posting's package-level store and
		// memory layer to the temporary DB. Stop its monitor goroutine,
		// close the temporary DB, then point posting back at the store
		// the test found, so later tests do not read from a closed DB
		// (e.g. TestExportRdf would die in x.Checkf on ErrDBClosed).
		posting.Cleanup()
		require.NoError(t, ps.Close())
		if prevStore != nil {
			posting.Init(prevStore, 0, false)
		}
	})
	// Not using posting list cache
	posting.Init(ps, 0, false)
	Init(ps)
	err = schema.ParseBytes([]byte("hasEdgesFriend: uid ."), 1)
	require.NoError(t, err)

	ctx := context.Background()
	attr := x.AttrInRootNamespace("hasEdgesFriend")

	// commit applies the edges in a txn and writes its deltas to disk, so that
	// hasEdges, which reads Badger directly, can see them. No rollup runs, so
	// the newest version of each key stays a delta.
	commit := func(startTs, commitTs uint64, edges []*pb.DirectedEdge) {
		t.Helper()
		txn := posting.Oracle().RegisterStartTs(startTs)
		for _, edge := range edges {
			require.NoError(t, runMutation(ctx, edge, txn))
		}
		txn.Update()
		writer := posting.NewTxnWriter(ps)
		require.NoError(t, txn.CommitToDisk(writer, commitTs))
		require.NoError(t, writer.Flush())
		txn.UpdateCachedKeys(commitTs)
	}

	// No data for the predicate yet.
	require.False(t, hasEdges(attr, math.MaxUint64))

	// Set one edge: the predicate has data.
	commit(10000, 10003, []*pb.DirectedEdge{{
		Entity: 1, ValueId: 2, Attr: attr,
		ValueType: pb.Posting_UID, Op: pb.DirectedEdge_SET,
	}})
	require.True(t, hasEdges(attr, math.MaxUint64))
	// A uid -> scalar schema change must be refused while data exists.
	err = checkSchema(&pb.SchemaUpdate{
		Predicate: attr, ValueType: pb.Posting_STRING,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Schema change not allowed from scalar to uid")

	// Delete the only edge. The newest version of the key is now a delete
	// delta rather than an empty rollup, so the predicate must no longer
	// count as having data.
	commit(10004, 10006, []*pb.DirectedEdge{{
		Entity: 1, ValueId: 2, Attr: attr,
		ValueType: pb.Posting_UID, Op: pb.DirectedEdge_DEL,
	}})
	require.False(t, hasEdges(attr, math.MaxUint64))
	// The uid -> scalar schema change must now be allowed.
	require.NoError(t, checkSchema(&pb.SchemaUpdate{
		Predicate: attr, ValueType: pb.Posting_STRING,
	}))

	// Setting another edge brings the data back.
	commit(10007, 10009, []*pb.DirectedEdge{{
		Entity: 1, ValueId: 3, Attr: attr,
		ValueType: pb.Posting_UID, Op: pb.DirectedEdge_SET,
	}})
	require.True(t, hasEdges(attr, math.MaxUint64))
	err = checkSchema(&pb.SchemaUpdate{
		Predicate: attr, ValueType: pb.Posting_STRING,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Schema change not allowed from scalar to uid")
}
