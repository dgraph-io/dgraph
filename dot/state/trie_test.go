// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package state

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/ChainSafe/gossamer/lib/trie"

	"github.com/ChainSafe/chaindb"
	"github.com/stretchr/testify/require"
)

func newTestTrieStateWithMemDB(t *testing.T) *TrieState {
	db := chaindb.NewMemDatabase()

	// make sure non-trie-prefixed keys are ignored by the TrieState
	err := db.Put([]byte("noot"), []byte("washere"))
	require.NoError(t, err)

	ts, err := NewTrieState(db, trie.NewEmptyTrie())
	require.NoError(t, err)
	return ts
}

func newTestTrieStateWithBadgerDB(t *testing.T) *TrieState {
	fp, _ := ioutil.TempDir("/tmp", "test-datadir-*")
	t.Cleanup(func() {
		os.RemoveAll(fp)
	})
	db, err := chaindb.NewBadgerDB(fp)
	require.NoError(t, err)

	// make sure non-trie-prefixed keys are ignored by the TrieState
	err = db.Put([]byte("noot"), []byte("washere"))
	require.NoError(t, err)

	ts, err := NewTrieState(db, trie.NewEmptyTrie())
	require.NoError(t, err)
	return ts
}

func TestNewTrieState(t *testing.T) {
	testFunc := func(ts *TrieState) {
		entries := ts.t.Entries()
		iter := ts.db.NewIterator()
		dbEntries := make(map[string][]byte)

		for iter.Next() {
			key := iter.Key()
			dbEntries[string(key)] = iter.Value()
		}

		require.Equal(t, entries, dbEntries)
	}

	ts := newTestTrieStateWithMemDB(t)
	testFunc(ts)
	ts = newTestTrieStateWithBadgerDB(t)
	testFunc(ts)
}

var testCases = []string{
	"asdf",
	"ghjk",
	"qwerty",
	"uiopl",
	"zxcv",
	"bnm",
}

func TestTrieState_Commit(t *testing.T) {
	testFunc := func(ts *TrieState) {
		expected := make(map[string][]byte)

		for _, tc := range testCases {
			err := ts.db.Put([]byte(tc), []byte(tc))
			require.NoError(t, err)
			expected[tc] = []byte(tc)
		}

		err := ts.Commit()
		require.NoError(t, err)
		require.Equal(t, expected, ts.t.Entries())
	}

	ts := newTestTrieStateWithMemDB(t)
	testFunc(ts)
	ts = newTestTrieStateWithBadgerDB(t)
	testFunc(ts)
}

func TestTrieState_Free(t *testing.T) {
	testFunc := func(ts *TrieState) {
		for _, tc := range testCases {
			err := ts.db.Put([]byte(tc), []byte(tc))
			require.NoError(t, err)
		}

		err := ts.Free()
		require.NoError(t, err)
		iter := ts.db.NewIterator()
		require.False(t, iter.Next())
	}

	ts := newTestTrieStateWithMemDB(t)
	testFunc(ts)
	ts = newTestTrieStateWithBadgerDB(t)
	testFunc(ts)
}

func TestTrieState_SetGet(t *testing.T) {
	testFunc := func(ts *TrieState) {
		for _, tc := range testCases {
			err := ts.Set([]byte(tc), []byte(tc))
			require.NoError(t, err)
		}

		// change a trie value to simulate runtime corruption
		err := ts.t.Put([]byte(testCases[0]), []byte("noot"))
		require.NoError(t, err)

		for _, tc := range testCases {
			res, err := ts.Get([]byte(tc))
			require.NoError(t, err)
			require.Equal(t, []byte(tc), res)
		}
	}

	ts := newTestTrieStateWithMemDB(t)
	testFunc(ts)
	ts = newTestTrieStateWithBadgerDB(t)
	testFunc(ts)
}

func TestTrieState_Delete(t *testing.T) {
	testFunc := func(ts *TrieState) {
		for _, tc := range testCases {
			err := ts.Set([]byte(tc), []byte(tc))
			require.NoError(t, err)
		}

		err := ts.Delete([]byte(testCases[0]))
		require.NoError(t, err)
		has, err := ts.Has([]byte(testCases[0]))
		require.NoError(t, err)
		require.False(t, has)
	}

	ts := newTestTrieStateWithMemDB(t)
	testFunc(ts)
	ts = newTestTrieStateWithBadgerDB(t)
	testFunc(ts)
}

func TestTrieState_Root(t *testing.T) {
	testFunc := func(ts *TrieState) {
		for _, tc := range testCases {
			err := ts.Set([]byte(tc), []byte(tc))
			require.NoError(t, err)
		}

		expected := ts.MustRoot()

		// change a trie value to simulate runtime corruption
		err := ts.t.Put([]byte(testCases[0]), []byte("noot"))
		require.NoError(t, err)
		require.Equal(t, expected, ts.MustRoot())
	}

	ts := newTestTrieStateWithMemDB(t)
	testFunc(ts)
	ts = newTestTrieStateWithBadgerDB(t)
	testFunc(ts)
}
