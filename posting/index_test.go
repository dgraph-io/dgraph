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
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const schemaStr = `
name:string @index(term) .
`

func uids(pl *protos.PostingList) []uint64 {
	l := &List{}
	l.plist = pl
	r := l.Uids(ListOptions{})
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

func addMutation(t *testing.T, l *List, edge *protos.DirectedEdge, op uint32,
	startTs uint64, commitTs uint64, index bool) {
	if op == Del {
		edge.Op = protos.DirectedEdge_DEL
	} else if op == Set {
		edge.Op = protos.DirectedEdge_SET
	} else {
		x.Fatalf("Unhandled op: %v", op)
	}
	txn := &Txn{StartTs: startTs}
	if index {
		require.NoError(t, l.AddMutationWithIndex(context.Background(), edge, txn))
	} else {
		ok, err := l.AddMutation(context.Background(), txn, edge)
		require.NoError(t, err)
		require.True(t, ok)
	}
	require.NoError(t, txn.CommitDeltas())
	require.NoError(t, l.CommitMutation(context.Background(), txn.StartTs, commitTs))
	require.NoError(t, commitMutations([][]byte{l.key}, commitTs))
}

const schemaVal = `
name:string @index(term) .
dob:dateTime @index(year) .
friend:uid @reverse .
	`

func TestTokensTable(t *testing.T) {
	err := schema.ParseBytes([]byte(schemaVal), 1)
	require.NoError(t, err)

	key := x.DataKey("name", 1)
	l, err := getNew(key, ps)
	require.NoError(t, err)
	defer ps.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})

	edge := &protos.DirectedEdge{
		Value:  []byte("david"),
		Label:  "testing",
		Attr:   "name",
		Entity: 157,
	}
	addMutation(t, l, edge, Set, 1, 2, true)
	_, err = l.SyncIfDirty(false)
	x.Check(err)

	key = x.IndexKey("name", "\x01david")
	time.Sleep(10 * time.Millisecond)

	var pl protos.PostingList
	txn := ps.NewTransaction(false)
	_, err = txn.Get(key)
	require.NoError(t, err)
	ps.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		require.NoError(t, err)
		val, err := item.Value()
		require.NoError(t, err)
		UnmarshalOrCopy(val, item.UserMeta(), &pl)
		return nil
	})

	require.EqualValues(t, []string{"\x01david"}, tokensForTest("name"))

	ps.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		require.NoError(t, err)
		val, err := item.Value()
		require.NoError(t, err)
		UnmarshalOrCopy(val, item.UserMeta(), &pl)
		return nil
	})

	require.EqualValues(t, []string{"\x01david"}, tokensForTest("name"))
	deletePl(t)
}

// tokensForTest returns keys for a table. This is just for testing / debugging.
func tokensForTest(attr string) []string {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	txn := pstore.NewTransaction(false)
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
	edge := &protos.DirectedEdge{
		Value:  []byte(value),
		Label:  "testing",
		Attr:   attr,
		Entity: src,
		Op:     protos.DirectedEdge_SET,
	}
	l := Get(x.DataKey(attr, src))
	// No index entries added here as we do not call AddMutationWithIndex.
	addMutation(t, l, edge, Set, startTs, commitTs, false)
}

// addEdgeToUID adds uid edge with reverse edge
func addEdgeToUID(t *testing.T, attr string, src uint64,
	dst uint64, startTs, commitTs uint64) {
	edge := &protos.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      protos.DirectedEdge_SET,
	}
	l := Get(x.DataKey(attr, src))
	// No index entries added here as we do not call AddMutationWithIndex.
	addMutation(t, l, edge, Set, startTs, commitTs, false)
}

// addEdgeToUID adds uid edge with reverse edge
func addReverseEdge(t *testing.T, attr string, src uint64,
	dst uint64, startTs, commitTs uint64) {
	edge := &protos.DirectedEdge{
		ValueId: dst,
		Label:   "testing",
		Attr:    attr,
		Entity:  src,
		Op:      protos.DirectedEdge_SET,
	}
	txn := &Txn{StartTs: startTs}
	txn.addReverseMutation(context.Background(), edge)
	require.NoError(t, txn.CommitDeltas())
	l := Get(x.ReverseKey(attr, dst))
	require.NoError(t, l.CommitMutation(context.Background(), txn.StartTs, commitTs))
	require.NoError(t, commitMutations([][]byte{l.key}, commitTs))
}

/*
func TestRebuildIndex(t *testing.T) {
	schema.ParseBytes([]byte(schemaVal), 1)
	addEdgeToValue(t, "name", 1, "Michonne", uint64(1), uint64(2))
	addEdgeToValue(t, "name", 20, "David", uint64(3), uint64(4))

	// Create some fake wrong entries for data store.
	ps.Update(func(txn *badger.Txn) error {
		txn.Set(x.IndexKey("name", "wrongname1"), []byte("nothing"), 0x00)
		txn.Set(x.IndexKey("name", "wrongname2"), []byte("nothing"), 0x00)
		return nil
	})

	require.NoError(t, DeleteIndex(context.Background(), "name"))
	tx := &Txn{StartTs: 5}
	require.NoError(t, RebuildIndex(context.Background(), "name", tx))

	// Check index entries in data store.
	txn := ps.NewTransaction(false)
	defer txn.Discard()
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	pk := x.ParsedKey{Attr: "name"}
	prefix := pk.IndexPrefix()
	var idxKeys []string
	var idxVals []*protos.PostingList
	for it.Seek(prefix); it.Valid(); it.Next() {
		item := it.Item()
		key := item.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		idxKeys = append(idxKeys, string(key))
		pl := new(protos.PostingList)
		val, err := item.Value()
		require.NoError(t, err)
		UnmarshalOrCopy(val, item.UserMeta(), pl)
		idxVals = append(idxVals, pl)
	}
	require.Len(t, idxKeys, 2)
	require.Len(t, idxVals, 2)
	require.EqualValues(t, idxKeys[0], x.IndexKey("name", "\x01david"))
	require.EqualValues(t, idxKeys[1], x.IndexKey("name", "\x01michonne"))

	uids1 := uids(idxVals[0])
	uids2 := uids(idxVals[1])
	require.Len(t, uids1, 1)
	require.Len(t, uids2, 1)
	require.EqualValues(t, 20, uids1[0])
	require.EqualValues(t, 1, uids2[0])

	l1 := Get(x.DataKey("name", 1))
	deletePl(t)
	ps.Update(func(txn *badger.Txn) error {
		return txn.Delete(l1.key)
	})
	l2 := Get(x.DataKey("name", 20))
	deletePl(t)
	ps.Update(func(txn *badger.Txn) error {
		return txn.Delete(l2.key)
	})
}

func TestRebuildReverseEdges(t *testing.T) {
	addEdgeToUID(t, "friend", 1, 23, uint64(1), uint64(2))
	addEdgeToUID(t, "friend", 1, 24, uint64(3), uint64(4))
	addEdgeToUID(t, "friend", 2, 23, uint64(5), uint64(6))

	tx := &Txn{StartTs: 5}
	require.NoError(t, RebuildReverseEdges(context.Background(), "friend", tx))

	// Check index entries in data store.
	txn := ps.NewTransaction(false)
	defer txn.Discard()
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	pk := x.ParsedKey{Attr: "friend"}
	prefix := pk.ReversePrefix()
	var revKeys []string
	var revVals []*protos.PostingList
	for it.Seek(prefix); it.Valid(); it.Next() {
		item := it.Item()
		key := item.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		revKeys = append(revKeys, string(key))
		pl := new(protos.PostingList)
		val, err := item.Value()
		require.NoError(t, err)
		UnmarshalOrCopy(val, item.UserMeta(), pl)
		revVals = append(revVals, pl)
	}
	require.Len(t, revKeys, 2)
	require.Len(t, revVals, 2)

	uids0 := uids(revVals[0])
	uids1 := uids(revVals[1])
	require.Len(t, uids0, 2)
	require.Len(t, uids1, 1)
	require.EqualValues(t, 1, uids0[0])
	require.EqualValues(t, 2, uids0[1])
	require.EqualValues(t, 1, uids1[0])
}
*/
