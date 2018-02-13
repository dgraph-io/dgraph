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
	"bytes"
	"context"
	"math"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const schemaStr = `
name:string @index(term) .
`

func uids(l *List, readTs uint64) []uint64 {
	r, err := l.Uids(ListOptions{ReadTs: readTs})
	x.Check(err)
	return r.Uids
}

func TestIndexingInt(t *testing.T) {
	schema.ParseBytes([]byte("age:int @index(int) ."), 1)
	a, err := indexTokens("age", "", types.Val{types.StringID, []byte("10")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x6, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexingIntNegative(t *testing.T) {
	schema.ParseBytes([]byte("age:int @index(int) ."), 1)
	a, err := indexTokens("age", "", types.Val{types.StringID, []byte("-10")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x6, 0x0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xf6}, []byte(a[0]))
}

func TestIndexingFloat(t *testing.T) {
	schema.ParseBytes([]byte("age:float @index(float) ."), 1)
	a, err := indexTokens("age", "", types.Val{types.StringID, []byte("10.43")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x7, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexingTime(t *testing.T) {
	schema.ParseBytes([]byte("age:dateTime @index(year) ."), 1)
	a, err := indexTokens("age", "", types.Val{types.StringID, []byte("0010-01-01T01:01:01.000000001")})
	require.NoError(t, err)
	require.EqualValues(t, []byte{0x4, 0x0, 0xa}, []byte(a[0]))
}

func TestIndexing(t *testing.T) {
	schema.ParseBytes([]byte("name:string @index(term) ."), 1)
	a, err := indexTokens("name", "", types.Val{types.StringID, []byte("abc")})
	require.NoError(t, err)
	require.EqualValues(t, "\x01abc", string(a[0]))
}

func TestIndexingMultiLang(t *testing.T) {
	schema.ParseBytes([]byte("name:string @index(fulltext) ."), 1)

	// ensure that default tokenizer is suitable for English
	a, err := indexTokens("name", "", types.Val{types.StringID, []byte("stemming")})
	require.NoError(t, err)
	require.EqualValues(t, "\x08stem", string(a[0]))

	// ensure that Finnish tokenizer is used
	a, err = indexTokens("name", "fi", types.Val{types.StringID, []byte("edeltäneessä")})
	require.NoError(t, err)
	require.EqualValues(t, "\x08edeltän", string(a[0]))

	// ensure that German tokenizer is used
	a, err = indexTokens("name", "de", types.Val{types.StringID, []byte("Auffassungsvermögen")})
	require.NoError(t, err)
	require.EqualValues(t, "\x08auffassungsvermog", string(a[0]))

	// ensure that default tokenizer works differently than German
	a, err = indexTokens("name", "", types.Val{types.StringID, []byte("Auffassungsvermögen")})
	require.NoError(t, err)
	require.EqualValues(t, "\x08auffassungsvermögen", string(a[0]))
}

func TestIndexingInvalidLang(t *testing.T) {
	schema.ParseBytes([]byte("name:string @index(fulltext) ."), 1)

	// there is no tokenizer for "xx" language
	_, err := indexTokens("name", "xx", types.Val{types.StringID, []byte("error")})
	require.Error(t, err)
}

func addMutation(t *testing.T, l *List, edge *intern.DirectedEdge, op uint32,
	startTs uint64, commitTs uint64, index bool) {
	if op == Del {
		edge.Op = intern.DirectedEdge_DEL
	} else if op == Set {
		edge.Op = intern.DirectedEdge_SET
	} else {
		x.Fatalf("Unhandled op: %v", op)
	}
	txn := &Txn{
		StartTs: startTs,
		Indices: []uint64{1},
	}
	txn = Txns().PutOrMergeIndex(txn)
	if index {
		require.NoError(t, l.AddMutationWithIndex(context.Background(), edge, txn))
	} else {
		ok, err := l.AddMutation(context.Background(), txn, edge)
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.NoError(t, txn.CommitMutations(context.Background(), commitTs))
}

const schemaVal = `
name:string @index(term) .
name2:string @index(term) .
dob:dateTime @index(year) .
friend:uid @reverse .
	`

// TODO(Txn): We can't read index key on disk if it was written in same txn.
func TestTokensTable(t *testing.T) {
	err := schema.ParseBytes([]byte(schemaVal), 1)
	require.NoError(t, err)

	key := x.DataKey("name", 1)
	l, err := getNew(key, ps)
	require.NoError(t, err)
	lcache.PutIfMissing(string(l.key), l)

	edge := &intern.DirectedEdge{
		Value:  []byte("david"),
		Label:  "testing",
		Attr:   "name",
		Entity: 157,
	}
	addMutation(t, l, edge, Set, 1, 2, true)
	merged, err := l.SyncIfDirty(false)
	require.True(t, merged)
	require.NoError(t, err)

	key = x.IndexKey("name", "\x01david")
	time.Sleep(10 * time.Millisecond)

	txn := ps.NewTransactionAt(3, false)
	_, err = txn.Get(key)
	require.NoError(t, err)

	require.EqualValues(t, []string{"\x01david"}, tokensForTest("name"))
}

// tokensForTest returns keys for a table. This is just for testing / debugging.
func tokensForTest(attr string) []string {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	var out []string
	for it.Seek(prefix); it.Valid(); it.Next() {
		key := it.Item().Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		k := x.Parse(key)
		x.AssertTrue(k.IsIndex())
		out = append(out, k.Term)
	}
	return out
}

// addEdgeToValue adds edge without indexing.
func addEdgeToValue(t *testing.T, attr string, src uint64,
	value string, startTs, commitTs uint64) {
	edge := &intern.DirectedEdge{
		Value:  []byte(value),
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     intern.DirectedEdge_SET,
	}
	l, err := Get(x.DataKey(attr, src))
	require.NoError(t, err)
	// No index entries added here as we do not call AddMutationWithIndex.
	addMutation(t, l, edge, Set, startTs, commitTs, false)
}

// addEdgeToUID adds uid edge with reverse edge
func addEdgeToUID(t *testing.T, attr string, src uint64,
	dst uint64, startTs, commitTs uint64) {
	edge := &intern.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      intern.DirectedEdge_SET,
	}
	l, err := Get(x.DataKey(attr, src))
	require.NoError(t, err)
	// No index entries added here as we do not call AddMutationWithIndex.
	addMutation(t, l, edge, Set, startTs, commitTs, false)
}

// addEdgeToUID adds uid edge with reverse edge
func addReverseEdge(t *testing.T, attr string, src uint64,
	dst uint64, startTs, commitTs uint64) {
	edge := &intern.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      intern.DirectedEdge_SET,
	}
	txn := Txn{
		StartTs: startTs,
	}
	txn.addReverseMutation(context.Background(), edge)
	require.NoError(t, txn.CommitMutations(context.Background(), commitTs))
}

func TestRebuildIndex(t *testing.T) {
	schema.ParseBytes([]byte(schemaVal), 1)
	addEdgeToValue(t, "name2", 91, "Michonne", uint64(1), uint64(2))
	addEdgeToValue(t, "name2", 92, "David", uint64(3), uint64(4))

	{
		txn := ps.NewTransactionAt(1, true)
		require.NoError(t, txn.Set(x.IndexKey("name2", "wrongname21"), []byte("nothing")))
		require.NoError(t, txn.Set(x.IndexKey("name2", "wrongname22"), []byte("nothing")))
		require.NoError(t, txn.CommitAt(1, nil))
	}

	require.NoError(t, DeleteIndex(context.Background(), "name2"))
	RebuildIndex(context.Background(), "name2", 5)
	CommitLists(func(key []byte) bool {
		pk := x.Parse(key)
		if pk.Attr == "name2" {
			return true
		}
		return false
	})

	// Check index entries in data store.
	txn := ps.NewTransactionAt(6, false)
	defer txn.Discard()
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	pk := x.ParsedKey{Attr: "name2"}
	prefix := pk.IndexPrefix()
	var idxKeys []string
	var idxVals []*List
	for it.Seek(prefix); it.Valid(); it.Next() {
		item := it.Item()
		key := item.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		if item.UserMeta()&BitEmptyPosting == BitEmptyPosting {
			continue
		}
		idxKeys = append(idxKeys, string(key))
		l, err := Get(key)
		require.NoError(t, err)
		idxVals = append(idxVals, l)
	}
	require.Len(t, idxKeys, 2)
	require.Len(t, idxVals, 2)
	require.EqualValues(t, idxKeys[0], x.IndexKey("name2", "\x01david"))
	require.EqualValues(t, idxKeys[1], x.IndexKey("name2", "\x01michonne"))

	uids1 := uids(idxVals[0], 6)
	uids2 := uids(idxVals[1], 6)
	require.Len(t, uids1, 1)
	require.Len(t, uids2, 1)
	require.EqualValues(t, 92, uids1[0])
	require.EqualValues(t, 91, uids2[0])
}

func TestRebuildReverseEdges(t *testing.T) {
	schema.ParseBytes([]byte(schemaVal), 1)
	addEdgeToUID(t, "friend", 1, 23, uint64(10), uint64(11))
	addEdgeToUID(t, "friend", 1, 24, uint64(12), uint64(13))
	addEdgeToUID(t, "friend", 2, 23, uint64(14), uint64(15))

	// TODO: Remove after fixing sync marks.
	RebuildReverseEdges(context.Background(), "friend", 16)
	CommitLists(func(key []byte) bool {
		pk := x.Parse(key)
		if pk.Attr == "friend" {
			return true
		}
		return false
	})

	// Check index entries in data store.
	txn := ps.NewTransactionAt(17, false)
	defer txn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := txn.NewIterator(iterOpts)
	defer it.Close()
	pk := x.ParsedKey{Attr: "friend"}
	prefix := pk.ReversePrefix()
	var revKeys []string
	var revVals []*List
	var prevKey []byte
	it.Seek(prefix)
	for it.ValidForPrefix(prefix) {
		item := it.Item()
		key := item.Key()
		if bytes.Equal(key, prevKey) {
			it.Next()
			continue
		}
		prevKey := make([]byte, len(key))
		copy(prevKey, key)
		revKeys = append(revKeys, string(key))
		l, err := ReadPostingList(key, it)
		require.NoError(t, err)
		revVals = append(revVals, l)
	}
	require.Len(t, revKeys, 2)
	require.Len(t, revVals, 2)

	uids0 := uids(revVals[0], 17)
	uids1 := uids(revVals[1], 17)
	require.Len(t, uids0, 2)
	require.Len(t, uids1, 1)
	require.EqualValues(t, 1, uids0[0])
	require.EqualValues(t, 2, uids0[1])
	require.EqualValues(t, 1, uids1[0])
}
