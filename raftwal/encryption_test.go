/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package raftwal

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft/v3/raftpb"
)

func TestEntryReadWrite(t *testing.T) {
	key := []byte("badger16byteskey")
	dir := t.TempDir()
	ds, err := InitEncrypted(dir, key)
	require.NoError(t, err)

	// generate some random data
	data := make([]byte, 1+rand.Intn(1000))
	rand.Read(data)

	require.NoError(t, ds.wal.AddEntries([]raftpb.Entry{{Index: 1, Term: 1, Data: data}}))
	entries := ds.wal.allEntries(0, 100, 10000)
	require.Equal(t, 1, len(entries))
	require.Equal(t, uint64(1), entries[0].Index)
	require.Equal(t, uint64(1), entries[0].Term)
	require.Equal(t, data, entries[0].Data)

	// Open the wal file again.
	ds2, err := InitEncrypted(dir, key)
	require.NoError(t, err)
	entries = ds2.wal.allEntries(0, 100, 10000)
	require.Equal(t, 1, len(entries))
	require.Equal(t, uint64(1), entries[0].Index)
	require.Equal(t, uint64(1), entries[0].Term)
	require.Equal(t, data, entries[0].Data)

	// Opening it with a wrong key fails.
	wrongKey := []byte("other16byteskeys")
	_, err = InitEncrypted(dir, wrongKey)
	require.EqualError(t, err, "Encryption key mismatch")

	// Opening it without encryption key fails.
	_, err = InitEncrypted(dir, nil)
	require.EqualError(t, err, "Logfile is encrypted but encryption key is nil")
}

// TestLogRotate writes enough log file entries to cause 1 file rotation.
func TestLogRotate(t *testing.T) {
	dir := t.TempDir()
	el, err := openWal(dir)
	fmt.Println(dir)
	require.NoError(t, err)

	// Generate deterministic entries using a seed.
	const SEED = 1
	rnd := rand.New(rand.NewSource(SEED))

	makeEntry := func(i int) raftpb.Entry {
		// Be careful when changing this value, as it could easily end up filling up
		// the entire tmpfs. Currently, this writes ~1.5GB.
		data := make([]byte, rnd.Intn(1<<16))
		rnd.Read(data)
		return raftpb.Entry{Index: uint64(i + 1), Term: 1, Data: data}
	}

	// Write enough entries to fill ~1.5x logfiles, causing a rotation.
	const totalEntries = (maxNumEntries * 3) / 2
	totalBytes := 0
	for i := range totalEntries {
		entry := makeEntry(i)
		require.NoError(t, el.AddEntries([]raftpb.Entry{entry}))
		totalBytes += len(entry.Data)
	}
	log.Printf("Wrote %d bytes", totalBytes)

	// Reopen the file and retrieve all entries.
	el, err = openWal(dir)
	require.NoError(t, err)
	entries := el.allEntries(0, math.MaxInt64, math.MaxInt64)
	require.Equal(t, totalEntries, len(entries))

	// Use the previous seed to verify the written entries.
	rnd = rand.New(rand.NewSource(SEED))
	for i, gotEntry := range entries {
		expEntry := makeEntry(i)
		require.Equal(t, len(expEntry.Data), len(gotEntry.Data))
		if len(expEntry.Data) > 0 {
			require.Equal(t, expEntry.Data, gotEntry.Data)
		}
		require.Equal(t, expEntry.Index, gotEntry.Index)
		require.Equal(t, expEntry.Term, gotEntry.Term)
		require.Equal(t, expEntry.Type, gotEntry.Type)
	}

	// 1 filled logfile should be present in files,
	// and 1 partially filled logfile should be present in current.
	require.Len(t, el.files, 1)
	require.NotNil(t, el.current)
}

// TestLogGrow writes data of sufficient size to grow the log file.
func TestLogGrow(t *testing.T) {
	test := func(t *testing.T, key []byte) {
		dir := t.TempDir()
		ds, err := InitEncrypted(dir, key)
		require.NoError(t, err)

		const numEntries = (maxNumEntries * 3) / 2

		// 5KB * 30000 is ~ 150MB, this will cause the log file to grow.
		var entries []raftpb.Entry
		for i := range numEntries {
			data := make([]byte, 5<<10)
			rand.Read(data)
			entry := raftpb.Entry{Index: uint64(i + 1), Term: 1, Data: data}
			entries = append(entries, entry)
		}
		require.NoError(t, ds.wal.AddEntries(entries))

		// Reopen the file and retrieve all entries.
		ds, err = InitEncrypted(dir, key)
		require.NoError(t, err)
		readEntries := ds.wal.allEntries(0, math.MaxInt64, math.MaxInt64)
		require.Equal(t, numEntries, len(readEntries))

		for i, gotEntry := range readEntries {
			expEntry := entries[i]
			require.Equal(t, expEntry.Data, gotEntry.Data)
			require.Equal(t, expEntry.Index, gotEntry.Index)
			require.Equal(t, expEntry.Term, gotEntry.Term)
			require.Equal(t, expEntry.Type, gotEntry.Type)
		}
	}
	t.Run("without encryption", func(t *testing.T) { test(t, nil) })
	t.Run("with encryption", func(t *testing.T) { test(t, []byte("badger16byteskey")) })
}
