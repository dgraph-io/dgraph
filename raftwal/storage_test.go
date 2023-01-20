/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Modified by Dgraph Labs to test DiskStorage.

package raftwal

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft"
	"go.etcd.io/etcd/raft/raftpb"

	"github.com/dgraph-io/dgraph/x"
)

func TestStorageTerm(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ds := Init(dir)

	ents := []raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}}
	tests := []struct {
		i uint64

		werr   error
		wterm  uint64
		wpanic bool
	}{
		{2, raft.ErrCompacted, 0, false},
		{3, nil, 3, false},
		{4, nil, 4, false},
		{5, nil, 5, false},
		{6, raft.ErrUnavailable, 0, false},
	}

	var snap raftpb.Snapshot
	snap.Metadata.Index = 3
	snap.Metadata.Term = 3

	require.NoError(t, ds.reset(ents))
	for i, tt := range tests {
		func() {
			defer func() {
				if r := recover(); r != nil {
					if !tt.wpanic {
						t.Errorf("%d: panic = %v, want %v", i, true, tt.wpanic)
					}
				}
			}()

			term, err := ds.Term(tt.i)
			if err != tt.werr {
				t.Errorf("#%d: err = %v, want %v", i, err, tt.werr)
			}
			if term != tt.wterm {
				t.Errorf("#%d: term = %d, want %d", i, term, tt.wterm)
			}
		}()
	}
}

func TestStorageEntries(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ds := Init(dir)

	ents := []raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}, {Index: 6, Term: 6}}
	tests := []struct {
		lo, hi, maxsize uint64

		werr     error
		wentries []raftpb.Entry
	}{
		{2, 6, math.MaxUint64, raft.ErrCompacted, nil},
		// {3, 4, math.MaxUint64, raft.ErrCompacted, nil},
		{4, 5, math.MaxUint64, nil, []raftpb.Entry{{Index: 4, Term: 4}}},
		{4, 6, math.MaxUint64, nil, []raftpb.Entry{{Index: 4, Term: 4}, {Index: 5, Term: 5}}},
		{4, 7, math.MaxUint64, nil, []raftpb.Entry{{Index: 4, Term: 4}, {Index: 5, Term: 5}, {Index: 6, Term: 6}}},
		// even if maxsize is zero, the first entry should be returned
		{4, 7, 0, nil, []raftpb.Entry{{Index: 4, Term: 4}}},
		// limit to 2
		{4, 7, uint64(ents[1].Size() + ents[2].Size()), nil, []raftpb.Entry{{Index: 4, Term: 4}, {Index: 5, Term: 5}}},
		// limit to 2
		{4, 7, uint64(ents[1].Size() + ents[2].Size() + ents[3].Size()/2), nil, []raftpb.Entry{{Index: 4, Term: 4}, {Index: 5, Term: 5}}},
		{4, 7, uint64(ents[1].Size() + ents[2].Size() + ents[3].Size() - 1), nil, []raftpb.Entry{{Index: 4, Term: 4}, {Index: 5, Term: 5}}},
		// all
		{4, 7, uint64(ents[1].Size() + ents[2].Size() + ents[3].Size()), nil, []raftpb.Entry{{Index: 4, Term: 4}, {Index: 5, Term: 5}, {Index: 6, Term: 6}}},
	}

	for i, tt := range tests {
		require.NoError(t, ds.reset(ents))
		// fi, _ := ds.FirstIndex()
		// t.Logf("first index: %d\n", fi)

		entries, err := ds.Entries(tt.lo, tt.hi, tt.maxsize)
		if err != tt.werr {
			t.Errorf("#%d: err = %v, want %v", i, err, tt.werr)
		}
		if !reflect.DeepEqual(entries, tt.wentries) {
			t.Errorf("#%d: entries = %v, want %v", i, entries, tt.wentries)
		}
	}
}

func TestStorageLastIndex(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ds := Init(dir)

	ents := []raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}}
	require.NoError(t, ds.reset(ents))

	last, err := ds.LastIndex()
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if last != 5 {
		t.Errorf("term = %d, want %d", last, 5)
	}

	ds.reset([]raftpb.Entry{{Index: 6, Term: 5}})
	last, err = ds.LastIndex()
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if last != 6 {
		t.Errorf("last = %d, want %d", last, 5)
	}
}

func TestStorageFirstIndex(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ds := Init(dir)

	ents := []raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}}
	require.NoError(t, ds.reset(ents))

	first, err := ds.FirstIndex()
	require.NoError(t, err)
	require.Equal(t, uint64(3), first)
}

func TestStorageCreateSnapshot(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ds := Init(dir)
	ents := []raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}}
	cs := &raftpb.ConfState{Nodes: []uint64{1, 2, 3}}
	data := []byte("data")

	tests := []struct {
		i uint64

		werr  error
		wsnap raftpb.Snapshot
	}{
		{4, nil, raftpb.Snapshot{Data: data, Metadata: raftpb.SnapshotMetadata{Index: 4, Term: 4, ConfState: *cs}}},
		{5, nil, raftpb.Snapshot{Data: data, Metadata: raftpb.SnapshotMetadata{Index: 5, Term: 5, ConfState: *cs}}},
	}

	// TODO: Add compacted test here.
	for i, tt := range tests {
		require.NoError(t, ds.reset(ents))
		err := ds.CreateSnapshot(tt.i, cs, data)
		if err != tt.werr {
			t.Errorf("#%d: err = %v, want %v", i, err, tt.werr)
		}
		snap, err := ds.Snapshot()
		require.NoError(t, err)
		if !reflect.DeepEqual(snap, tt.wsnap) {
			t.Errorf("#%d: snap = %+v, want %+v", i, snap, tt.wsnap)
		}
	}
}

func TestStorageAppend(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	ds := Init(dir)

	ents := []raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}}
	tests := []struct {
		entries []raftpb.Entry

		werr     error
		wentries []raftpb.Entry
	}{
		{
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}},
			nil,
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}},
		},
		{
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 6}, {Index: 5, Term: 6}},
			nil,
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 6}, {Index: 5, Term: 6}},
		},
		{
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}, {Index: 6, Term: 5}},
			nil,
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}, {Index: 6, Term: 5}},
		},
		// truncate incoming entries, truncate the existing entries and append
		{
			[]raftpb.Entry{{Index: 2, Term: 3}, {Index: 3, Term: 3}, {Index: 4, Term: 5}},
			nil,
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 5}},
		},
		// truncate the existing entries and append
		{
			[]raftpb.Entry{{Index: 4, Term: 5}},
			nil,
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 5}},
		},
		// direct append
		{
			[]raftpb.Entry{{Index: 6, Term: 5}},
			nil,
			[]raftpb.Entry{{Index: 3, Term: 3}, {Index: 4, Term: 4}, {Index: 5, Term: 5}, {Index: 6, Term: 5}},
		},
	}

	for i, tt := range tests {
		require.NoError(t, ds.reset(ents))
		err := ds.addEntries(tt.entries)
		if err != tt.werr {
			t.Errorf("#%d: err = %v, want %v", i, err, tt.werr)
		}
		all := ds.wal.allEntries(0, math.MaxUint64, math.MaxUint64)
		if !reflect.DeepEqual(all, tt.wentries) {
			t.Errorf("#%d: entries = %v, want %v", i, all, tt.wentries)
		}
	}
}

func TestMetaFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "badger-test")
	require.NoError(t, err)

	mf, err := newMetaFile(dir)
	require.NoError(t, err)
	id := mf.Uint(RaftId)
	require.Zero(t, id)

	mf.SetUint(RaftId, 10)
	id = mf.Uint(RaftId)
	require.NoError(t, err)
	require.Equal(t, uint64(10), id)

	hs, err := mf.HardState()
	require.NoError(t, err)
	require.Zero(t, hs)

	hs = raftpb.HardState{
		Term:   10,
		Vote:   20,
		Commit: 30,
	}
	require.NoError(t, mf.StoreHardState(&hs))

	hs1, err := mf.HardState()
	require.NoError(t, err)
	require.Equal(t, hs1, hs)

	sp, err := mf.snapshot()
	require.NoError(t, err)
	require.Zero(t, sp)

	sp = raftpb.Snapshot{
		Data: []byte("foo"),
		Metadata: raftpb.SnapshotMetadata{
			Term:  200,
			Index: 12,
		},
	}
	require.NoError(t, mf.StoreSnapshot(&sp))

	sp1, err := mf.snapshot()
	require.NoError(t, err)
	require.Equal(t, sp, sp1)
}

func TestEntryFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "raftwal")
	require.NoError(t, err)
	el, err := openWal(dir)
	require.NoError(t, err)
	require.Equal(t, uint64(1), el.firstIndex())
	require.Zero(t, el.LastIndex())

	e, err := el.seekEntry(2)
	require.Error(t, err)
	require.NotNil(t, e)

	require.NoError(t, el.AddEntries([]raftpb.Entry{{Index: 1, Term: 1, Data: []byte("abc")}}))
	entries := el.allEntries(0, 100, 10000)
	require.Equal(t, 1, len(entries))
	require.Equal(t, uint64(1), entries[0].Index)
	require.Equal(t, uint64(1), entries[0].Term)
	require.Equal(t, "abc", string(entries[0].Data))
}

func TestTruncateStorage(t *testing.T) {
	dir, err := ioutil.TempDir("", "raftwal")
	require.NoError(t, err)
	ds, err := InitEncrypted(dir, nil)
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	const numEntries = uint64(maxNumEntries*3) / 2
	const numTruncated = numEntries / 2

	// Insert entries.
	for i := uint64(0); i < numEntries; i++ {
		var typ raftpb.EntryType
		if i%3 == 0 {
			typ = raftpb.EntryConfChange
		} else {
			typ = raftpb.EntryNormal
		}

		idx := i + 1
		entry := raftpb.Entry{
			Term:  idx,
			Index: idx,
			Type:  typ,
			Data:  []byte(fmt.Sprintf("entry %d", idx)),
		}
		ds.addEntries([]raftpb.Entry{entry})
	}

	// Verify all entries.
	entries := ds.wal.allEntries(0, math.MaxUint64, math.MaxUint64)
	require.Len(t, entries, int(numEntries))
	for i := uint64(0); i < numEntries; i++ {
		idx := i + 1
		require.Equal(t, entries[i].Data, []byte(fmt.Sprintf("entry %d", idx)))
	}

	// Truncate entries.
	ds.TruncateEntriesUntil(numTruncated)

	// Verify all entries.
	entries = ds.wal.allEntries(0, math.MaxUint64, math.MaxUint64)
	require.Len(t, entries, int(numEntries))
	for i := uint64(0); i < numEntries; i++ {
		idx := i + 1
		if idx < numTruncated && i%3 != 0 {
			require.Empty(t, entries[i].Data)
		} else {
			require.Equal(t, entries[i].Data, []byte(fmt.Sprintf("entry %d", idx)))
		}
	}

	// Insert more entries.
	const numEntriesNew = numEntries + maxNumEntries
	for i := numEntries; i < numEntriesNew; i++ {
		idx := i + 1
		entry := raftpb.Entry{
			Term:  idx,
			Index: idx,
			Type:  raftpb.EntryNormal,
			Data:  []byte(fmt.Sprintf("entry %d", idx)),
		}
		ds.addEntries([]raftpb.Entry{entry})
	}

	// Verify all entries.
	entries = ds.wal.allEntries(0, math.MaxUint64, math.MaxUint64)
	require.Len(t, entries, int(numEntriesNew))
	for i := uint64(0); i < numEntriesNew; i++ {
		idx := i + 1
		if idx < numTruncated && i%3 != 0 {
			require.Empty(t, entries[i].Data)
		} else {
			require.Equal(t, entries[i].Data, []byte(fmt.Sprintf("entry %d", idx)))
		}
	}
}

func TestStorageOnlySnap(t *testing.T) {
	test := func(t *testing.T, key []byte) {
		x.WorkerConfig.EncryptionKey = key
		dir, err := ioutil.TempDir("", "raftwal")
		require.NoError(t, err)
		ds, err := InitEncrypted(dir, key)
		require.NoError(t, err)
		t.Logf("Creating dir: %s\n", dir)

		buf := make([]byte, 128)
		rand.Read(buf)
		N := uint64(1000)

		snap := &raftpb.Snapshot{}
		snap.Metadata.Index = N
		snap.Metadata.ConfState = raftpb.ConfState{}
		snap.Data = buf

		require.NoError(t, ds.meta.StoreSnapshot(snap))

		out, err := ds.Snapshot()
		require.NoError(t, err)
		require.Equal(t, N, out.Metadata.Index)

		fi, err := ds.FirstIndex()
		require.NoError(t, err)
		require.Equal(t, N+1, fi)

		li, err := ds.LastIndex()
		require.NoError(t, err)
		require.Equal(t, N, li)
	}
	t.Run("without encryption", func(t *testing.T) { test(t, nil) })
	t.Run("with encryption", func(t *testing.T) { test(t, []byte("badger16byteskey")) })
}

func TestStorageBig(t *testing.T) {
	test := func(t *testing.T, key []byte) {
		dir, err := ioutil.TempDir("", "raftwal")
		require.NoError(t, err)
		ds, err := InitEncrypted(dir, key)
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		ent := raftpb.Entry{
			Term: 1,
			Type: raftpb.EntryNormal,
		}

		addEntries := func(start, end uint64) {
			t.Logf("adding entries: %d -> %d\n", start, end)
			for idx := start; idx <= end; idx++ {
				ent.Index = idx
				require.NoError(t, ds.wal.AddEntries([]raftpb.Entry{ent}))
				li, err := ds.LastIndex()
				require.NoError(t, err)
				require.Equal(t, idx, li)
			}
		}

		N := uint64(100000)
		addEntries(1, N)
		num := ds.NumEntries()
		require.Equal(t, int(N), num)

		check := func(start, end uint64) {
			ents, err := ds.Entries(start, end, math.MaxInt64)
			require.NoError(t, err)
			require.Equal(t, int(end-start), len(ents))
			for i, e := range ents {
				require.Equal(t, start+uint64(i), e.Index)
			}
		}
		_, err = ds.Entries(0, 1, math.MaxInt64)
		require.Equal(t, raft.ErrCompacted, err)

		check(3, N)
		check(10000, 20000)
		check(20000, 33000)
		check(33000, 45000)
		check(45000, N)

		// Around file boundaries.
		check(1, N)
		check(30000, N)
		check(30001, N)
		check(60000, N)
		check(60001, N)
		check(60000, 90000)
		check(N, N+1)

		_, err = ds.Entries(N+1, N+10, math.MaxInt64)
		require.Error(t, raft.ErrUnavailable, err)

		// Jump back a few files.
		addEntries(N/3, N)
		check(3, N)
		check(10000, 20000)
		check(20000, 33000)
		check(33000, 45000)
		check(45000, N)
		check(N, N+1)

		buf := make([]byte, 128)
		rand.Read(buf)

		cs := &raftpb.ConfState{}
		require.NoError(t, ds.CreateSnapshot(N-100, cs, buf))
		fi, err := ds.FirstIndex()
		require.NoError(t, err)
		require.Equal(t, N-100+1, fi)

		snap, err := ds.Snapshot()
		require.NoError(t, err)
		require.Equal(t, N-100, snap.Metadata.Index)
		require.Equal(t, buf, snap.Data)

		require.Equal(t, 0, len(ds.wal.files))

		files, err := getLogFiles(dir)
		require.NoError(t, err)
		require.Equal(t, 1, len(files))

		// Jumping back.
		ent.Index = N - 50
		require.NoError(t, ds.wal.AddEntries([]raftpb.Entry{ent}))

		start := N - 100 + 1
		ents := ds.wal.allEntries(start, math.MaxInt64, math.MaxInt64)
		require.Equal(t, 50, len(ents))
		for idx, ent := range ents {
			require.Equal(t, int(start)+idx, int(ent.Index))
		}

		ent.Index = N
		require.NoError(t, ds.wal.AddEntries([]raftpb.Entry{ent}))
		ents = ds.wal.allEntries(start, math.MaxInt64, math.MaxInt64)
		require.Equal(t, 51, len(ents))
		for idx, ent := range ents {
			if idx == 50 {
				require.Equal(t, N, ent.Index)
			} else {
				require.Equal(t, int(start)+idx, int(ent.Index))
			}
		}
		require.NoError(t, ds.Sync())

		ks, err := InitEncrypted(dir, key)
		require.NoError(t, err)
		ents = ks.wal.allEntries(start, math.MaxInt64, math.MaxInt64)
		require.Equal(t, 51, len(ents))
		for idx, ent := range ents {
			if idx == 50 {
				require.Equal(t, N, ent.Index)
			} else {
				require.Equal(t, int(start)+idx, int(ent.Index))
			}
		}
	}
	t.Run("without encryption", func(t *testing.T) { test(t, nil) })
	t.Run("with encryption", func(t *testing.T) { test(t, []byte("badger16byteskey")) })
}
