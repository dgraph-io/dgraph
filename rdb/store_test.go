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

package rdb

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"

	"github.com/dgraph-io/dgraph/store"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	path, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(path)

	s, err := NewStore(path)
	require.NoError(t, err)

	k := []byte("mykey")
	require.NoError(t, s.SetOne(k, []byte("neo")))

	val, freeVal, err := s.Get(k)
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, val, "neo")

	require.NoError(t, s.SetOne(k, []byte("the one")))
	val, freeVal, err = s.Get(k)
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, val, "the one")
}

func TestSnapshot(t *testing.T) {
	path, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(path)

	s, err := NewStore(path)
	require.NoError(t, err)

	k := []byte("mykey")
	require.NoError(t, s.SetOne(k, []byte("neo")))

	snapshot := s.NewSnapshot() // Snapshot will contain neo, not trinity.
	require.NoError(t, s.SetOne(k, []byte("trinity")))

	// Before setting snapshot, do a get. Expect to get trinity.
	val, freeVal, err := s.Get(k)
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, val, "trinity")

	s.SetSnapshot(snapshot)
	// After setting snapshot, we expect to get neo.
	val, freeVal, err = s.Get(k)
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, val, "neo")

	s.SetSnapshot(nil)
	// After clearing snapshot, we expect to get trinity again.
	val, freeVal, err = s.Get(k)
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, val, "trinity")
}

func TestCheckpoint(t *testing.T) {
	dbPath, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dbPath)

	s, err := NewStore(dbPath)
	require.NoError(t, err)

	key := []byte("mykey")
	require.NoError(t, s.SetOne(key, []byte("neo")))

	// Make sure neo did get written as we expect.
	val, freeVal, err := s.Get(key)
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, val, "neo")

	// Do checkpointing. Checkpoint should contain neo.
	checkpoint, err := s.NewCheckpoint()
	require.NoError(t, err)

	pathCheckpoint := path.Join(dbPath, "checkpoint") // Do not mkdir yet.
	checkpoint.Save(pathCheckpoint)
	checkpoint.Destroy()

	// Update original store to contain trinity.
	require.NoError(t, s.SetOne(key, []byte("trinity")))

	// Original store should contain trinity.
	val, freeVal, err = s.Get(key)
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, val, "trinity")

	// Open checkpoint and check that it contains neo, not trinity.
	s2, err := NewStore(pathCheckpoint)
	require.NoError(t, err)

	val, freeVal, err = s2.Get(key)
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, val, "neo")
}

func TestBasic(t *testing.T) {
	dbPath, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dbPath)

	var s store.Store
	s, err = NewStore(dbPath)
	defer s.Close()
	require.NoError(t, err)

	wb := s.NewWriteBatch()
	wb.SetOne([]byte("hey"), []byte("world"))
	wb.SetOne([]byte("aaa"), []byte("bbb"))
	require.NoError(t, s.WriteBatch(wb))

	val, freeVal, err := s.Get([]byte("no"))
	defer freeVal()
	require.NoError(t, err)
	require.Nil(t, val)

	val, freeVal, err = s.Get([]byte("aaa"))
	defer freeVal()
	require.NoError(t, err)
	require.EqualValues(t, "bbb", string(val))

	it := s.NewIterator(false)
	defer it.Close()
	var keys []string
	for it.Rewind(); it.Valid(); it.Next() {
		keys = append(keys, string(it.Key()))
	}
	require.EqualValues(t, []string{"aaa", "hey"}, keys)

	it = s.NewIterator(true)
	defer it.Close()
	keys = keys[:0]
	for it.Rewind(); it.Valid(); it.Next() {
		keys = append(keys, string(it.Key()))
	}
	require.EqualValues(t, []string{"hey", "aaa"}, keys)
}

func benchmarkGet(valSize int, b *testing.B) {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		b.Error(err)
		return
	}
	defer os.RemoveAll(path)

	s, err := NewStore(path)
	if err != nil {
		b.Error(err)
		return
	}
	buf := make([]byte, valSize)

	nkeys := 100
	for i := 0; i < nkeys; i++ {
		key := []byte(fmt.Sprintf("key_%d", i))
		if err := s.SetOne(key, buf); err != nil {
			b.Error(err)
			return
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := rand.Int() % nkeys
		key := []byte(fmt.Sprintf("key_%d", k))
		valSlice, freeVal, err := s.Get(key)
		if valSlice == nil {
			b.Error("Missing value")
		}
		if err != nil {
			b.Error(err)
		}
		if len(valSlice) != valSize {
			b.Errorf("Value size expected: %d. Found: %d", valSize, len(valSlice))
		}
		freeVal()
	}
	b.StopTimer()
}

func BenchmarkGet_valsize1024(b *testing.B)  { benchmarkGet(1024, b) }
func BenchmarkGet_valsize10KB(b *testing.B)  { benchmarkGet(10240, b) }
func BenchmarkGet_valsize500KB(b *testing.B) { benchmarkGet(1<<19, b) }
func BenchmarkGet_valsize1MB(b *testing.B)   { benchmarkGet(1<<20, b) }

func benchmarkSet(valSize int, b *testing.B) {
	path, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		b.Error(err)
		return
	}
	defer os.RemoveAll(path)

	s, err := NewStore(path)
	if err != nil {
		b.Error(err)
		return
	}
	buf := make([]byte, valSize)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key_%d", i))
		if err := s.SetOne(key, buf); err != nil {
			b.Error(err)
			return
		}
	}
	b.StopTimer()
}

func BenchmarkSet_valsize1024(b *testing.B)  { benchmarkSet(1024, b) }
func BenchmarkSet_valsize10KB(b *testing.B)  { benchmarkSet(10240, b) }
func BenchmarkSet_valsize500KB(b *testing.B) { benchmarkSet(1<<19, b) }
func BenchmarkSet_valsize1MB(b *testing.B)   { benchmarkSet(1<<20, b) }
