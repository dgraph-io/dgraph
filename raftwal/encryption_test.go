/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package raftwal

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/x"

	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft/raftpb"
)

func TestEntryReadWrite(t *testing.T) {
	x.WorkerConfig.EncryptionKey = []byte("badger16byteskey")
	dir, err := ioutil.TempDir("", "raftwal")
	require.NoError(t, err)
	el, err := openWal(dir)
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	require.NoError(t, el.AddEntries([]raftpb.Entry{{Index: 1, Term: 1, Data: []byte("abc")}}))
	entries := el.allEntries(0, 100, 10000)
	require.Equal(t, 1, len(entries))
	require.Equal(t, uint64(1), entries[0].Index)
	require.Equal(t, uint64(1), entries[0].Term)
	require.Equal(t, "abc", string(entries[0].Data))

	// Open the wal file again.
	el2, err := openWal(dir)
	require.NoError(t, err)
	entries = el2.allEntries(0, 100, 10000)
	require.Equal(t, 1, len(entries))
	require.Equal(t, uint64(1), entries[0].Index)
	require.Equal(t, uint64(1), entries[0].Term)
	require.Equal(t, "abc", string(entries[0].Data))

	// Try opening it with a wrong key.
	x.WorkerConfig.EncryptionKey = []byte("other16byteskeys")
	_, err = openWal(dir)
	require.EqualError(t, err, "Encryption key mismatch")
}
