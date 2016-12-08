/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package store

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"testing"

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

	val, err := s.Get(k)
	require.NoError(t, err)
	require.EqualValues(t, val.Data(), "neo")

	require.NoError(t, s.SetOne(k, []byte("the one")))
	val, err = s.Get(k)
	require.NoError(t, err)
	require.EqualValues(t, val.Data(), "the one")
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
	val, err := s.Get(k)
	require.NoError(t, err)
	require.EqualValues(t, val.Data(), "trinity")

	s.SetSnapshot(snapshot)
	// After setting snapshot, we expect to get neo.
	val, err = s.Get(k)
	require.NoError(t, err)
	require.EqualValues(t, val.Data(), "neo")

	s.SetSnapshot(nil)
	// After clearing snapshot, we expect to get trinity again.
	val, err = s.Get(k)
	require.NoError(t, err)
	require.EqualValues(t, val.Data(), "trinity")
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
	val, err := s.Get(key)
	require.NoError(t, err)
	require.EqualValues(t, val.Data(), "neo")

	// Do checkpointing. Checkpoint should contain neo.
	checkpoint, err := s.NewCheckpoint()
	require.NoError(t, err)

	pathCheckpoint := path.Join(dbPath, "checkpoint") // Do not mkdir yet.
	checkpoint.Save(pathCheckpoint)
	checkpoint.Destroy()

	// Update original store to contain trinity.
	require.NoError(t, s.SetOne(key, []byte("trinity")))

	// Original store should contain trinity.
	val, err = s.Get(key)
	require.NoError(t, err)
	require.EqualValues(t, val.Data(), "trinity")

	// Open checkpoint and check that it contains neo, not trinity.
	s2, err := NewStore(pathCheckpoint)
	require.NoError(t, err)

	val, err = s2.Get(key)
	require.NoError(t, err)
	require.EqualValues(t, val.Data(), "neo")
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
		valSlice, err := s.Get(key)
		if valSlice == nil {
			b.Error("Missing value")
		}
		if err != nil {
			b.Error(err)
		}
		if len(valSlice.Data()) != valSize {
			b.Errorf("Value size expected: %d. Found: %d", valSize, len(valSlice.Data()))
		}
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
