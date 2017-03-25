/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package posting

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const schemaStr = `
name:string @index
`

func TestIndexingInt(t *testing.T) {
	schema.ParseBytes([]byte("age:int @index"), 1)
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("10")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x6, 0x1, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexingIntNegative(t *testing.T) {
	schema.ParseBytes([]byte("age:int @index"), 1)
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("-10")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x6, 0x0, 0xff, 0xff, 0xff, 0xf6}, []byte(a[0]))
}

func TestIndexingFloat(t *testing.T) {
	schema.ParseBytes([]byte("age:float @index"), 1)
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("10.43")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x7, 0x1, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexingDate(t *testing.T) {
	schema.ParseBytes([]byte("age:date @index"), 1)
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("0010-01-01")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x3, 0x1, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexingTime(t *testing.T) {
	schema.ParseBytes([]byte("age:datetime @index"), 1)
	a, err := IndexTokens("age", types.Val{types.StringID, []byte("0010-01-01T01:01:01.000000001")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x4, 0x1, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexing(t *testing.T) {
	schema.ParseBytes([]byte("name:string @index"), 1)
	a, err := IndexTokens("name", types.Val{types.StringID, []byte("abc")})
	require.NoError(t, err)
	require.EqualValues(t, "\x01abc", string(a[0]))
}

func addMutationWithIndex(t *testing.T, l *List, edge *taskp.DirectedEdge, op uint32) {
	if op == Del {
		edge.Op = taskp.DirectedEdge_DEL
	} else if op == Set {
		edge.Op = taskp.DirectedEdge_SET
	} else {
		x.Fatalf("Unhandled op: %v", op)
	}
	require.NoError(t, l.AddMutationWithIndex(context.Background(), edge))
}

const schemaVal = `
name:string @index
dob:date @index
friend:uid @reverse
	`

func TestTokensTable(t *testing.T) {
	schema.ParseBytes([]byte(schemaVal), 1)

	key := x.DataKey("name", 1)
	l := getNew(key, ps)

	edge := &taskp.DirectedEdge{
		Value:  []byte("david"),
		Label:  "testing",
		Attr:   "name",
		Entity: 157,
	}
	addMutationWithIndex(t, l, edge, Set)

	key = x.IndexKey("name", "david")
	slice, err := ps.Get(key)
	require.NoError(t, err)

	var pl typesp.PostingList
	x.Check(pl.Unmarshal(slice.Data()))

	require.EqualValues(t, []string{"\x01david"}, tokensForTest("name"))

	CommitLists(10, 1)
	time.Sleep(time.Second)

	slice, err = ps.Get(key)
	require.NoError(t, err)
	x.Check(pl.Unmarshal(slice.Data()))

	require.EqualValues(t, []string{"\x01david"}, tokensForTest("name"))
	deletePl(t, l)
}

// tokensForTest returns keys for a table. This is just for testing / debugging.
func tokensForTest(attr string) []string {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	it := pstore.NewIterator()
	defer it.Close()

	var out []string
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		k := x.Parse(it.Key().Data())
		x.AssertTrue(k.IsIndex())
		out = append(out, k.Term)
	}
	return out
}

// addEdgeToValue adds edge without indexing.
func addEdgeToValue(t *testing.T, attr string, src uint64,
	value string) {
	edge := &taskp.DirectedEdge{
		Value:  []byte(value),
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     taskp.DirectedEdge_SET,
	}
	l, _ := GetOrCreate(x.DataKey(attr, src), 1)
	// No index entries added here as we do not call AddMutationWithIndex.
	ok, err := l.AddMutation(context.Background(), edge)
	require.NoError(t, err)
	require.True(t, ok)
}

// addEdgeToUID adds uid edge with reverse edge
func addEdgeToUID(t *testing.T, attr string, src uint64,
	dst uint64) {
	edge := &taskp.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      taskp.DirectedEdge_SET,
	}
	l, _ := GetOrCreate(x.DataKey(attr, src), 1)
	// No index entries added here as we do not call AddMutationWithIndex.
	ok, err := l.AddMutation(context.Background(), edge)
	require.NoError(t, err)
	require.True(t, ok)
}

// addEdgeToUID adds uid edge with reverse edge
func addReverseEdge(t *testing.T, attr string, src uint64,
	dst uint64) {
	edge := &taskp.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      taskp.DirectedEdge_SET,
	}
	addReverseMutation(context.Background(), edge)
}

func TestRebuildIndex(t *testing.T) {
	schema.ParseBytes([]byte(schemaVal), 1)
	addEdgeToValue(t, "name", 1, "Michonne")
	addEdgeToValue(t, "name", 20, "David")

	// RebuildIndex requires the data to be committed to data store.
	CommitLists(10, 1)
	for len(syncCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Create some fake wrong entries for data store.
	ps.SetOne(x.IndexKey("name", "wrongname1"), []byte("nothing"))
	ps.SetOne(x.IndexKey("name", "wrongname2"), []byte("nothing"))

	require.NoError(t, RebuildIndex(context.Background(), "name"))

	// Let's force a commit.
	CommitLists(10, 1)
	for len(syncCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Check index entries in data store.
	it := ps.NewIterator()
	defer it.Close()
	pk := x.ParsedKey{Attr: "name"}
	prefix := pk.IndexPrefix()
	var idxKeys []string
	var idxVals []*typesp.PostingList
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		idxKeys = append(idxKeys, string(it.Key().Data()))
		pl := new(typesp.PostingList)
		require.NoError(t, pl.Unmarshal(it.Value().Data()))
		idxVals = append(idxVals, pl)
	}
	require.Len(t, idxKeys, 2)
	require.Len(t, idxVals, 2)
	require.EqualValues(t, x.IndexKey("name", "\x01david"), idxKeys[0])
	require.EqualValues(t, x.IndexKey("name", "\x01michonne"), idxKeys[1])
	require.Len(t, idxVals[0].Postings, 1)
	require.Len(t, idxVals[1].Postings, 1)
	require.EqualValues(t, idxVals[0].Postings[0].Uid, 20)
	require.EqualValues(t, idxVals[1].Postings[0].Uid, 1)

	l1, _ := GetOrCreate(x.DataKey("name", 1), 1)
	deletePl(t, l1)
	l2, _ := GetOrCreate(x.DataKey("name", 20), 1)
	deletePl(t, l2)
}

func TestRebuildReverseEdges(t *testing.T) {
	addEdgeToUID(t, "friend", 1, 23)
	addEdgeToUID(t, "friend", 1, 24)
	addEdgeToUID(t, "friend", 2, 23)

	// RebuildIndex requires the data to be committed to data store.
	CommitLists(10, 1)
	for len(syncCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Create some fake wrong entries for data store.
	addEdgeToUID(t, "friend", 1, 100)

	require.NoError(t, RebuildReverseEdges(context.Background(), "friend"))

	// Let's force a commit.
	CommitLists(10, 1)
	for len(syncCh) > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Check index entries in data store.
	it := ps.NewIterator()
	defer it.Close()
	pk := x.ParsedKey{Attr: "friend"}
	prefix := pk.ReversePrefix()
	var revKeys []string
	var revVals []*typesp.PostingList
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		revKeys = append(revKeys, string(it.Key().Data()))
		pl := new(typesp.PostingList)
		require.NoError(t, pl.Unmarshal(it.Value().Data()))
		revVals = append(revVals, pl)
	}
	require.Len(t, revKeys, 2)
	require.Len(t, revVals, 2)
	require.Len(t, revVals[0].Postings, 2)
	require.Len(t, revVals[1].Postings, 1)
	require.EqualValues(t, revVals[0].Postings[0].Uid, 1)
	require.EqualValues(t, revVals[0].Postings[1].Uid, 2)
	require.EqualValues(t, revVals[1].Postings[0].Uid, 1)
}
