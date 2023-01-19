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

package posting

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

func TestRollupTimestamp(t *testing.T) {
	attr := x.GalaxyAttr("rollup")
	key := x.DataKey(attr, 1)
	// 3 Delta commits.
	addEdgeToUID(t, attr, 1, 2, 1, 2)
	addEdgeToUID(t, attr, 1, 3, 3, 4)
	addEdgeToUID(t, attr, 1, 4, 5, 6)

	l, err := GetNoStore(key, math.MaxUint64)
	require.NoError(t, err)

	uidList, err := l.Uids(ListOptions{ReadTs: 7})
	require.NoError(t, err)
	require.Equal(t, 3, len(uidList.Uids))

	edge := &pb.DirectedEdge{
		Entity: 1,
		Attr:   attr,
		Value:  []byte(x.Star),
		Op:     pb.DirectedEdge_DEL,
	}
	addMutation(t, l, edge, Del, 9, 10, false)

	nl, err := getNew(key, pstore, math.MaxUint64)
	require.NoError(t, err)

	uidList, err = nl.Uids(ListOptions{ReadTs: 11})
	require.NoError(t, err)
	require.Equal(t, 0, len(uidList.Uids))

	// Now check that we don't lost the highest version during a rollup operation, despite the STAR
	// delete marker being the most recent update.
	kvs, err := nl.Rollup(nil)
	require.NoError(t, err)
	require.Equal(t, uint64(10), kvs[0].Version)
}

func TestPostingListRead(t *testing.T) {
	attr := x.GalaxyAttr("emptypl")
	key := x.DataKey(attr, 1)

	assertLength := func(readTs, sz int) {
		nl, err := getNew(key, pstore, math.MaxUint64)
		require.NoError(t, err)
		uidList, err := nl.Uids(ListOptions{ReadTs: uint64(readTs)})
		require.NoError(t, err)
		require.Equal(t, sz, len(uidList.Uids))
	}

	addEdgeToUID(t, attr, 1, 2, 1, 2)
	addEdgeToUID(t, attr, 1, 3, 3, 4)

	writer := NewTxnWriter(pstore)
	require.NoError(t, writer.SetAt(key, []byte{}, BitEmptyPosting, 6))
	require.NoError(t, writer.Flush())
	assertLength(7, 0)

	addEdgeToUID(t, attr, 1, 4, 7, 8)
	assertLength(9, 1)

	var empty pb.PostingList
	data, err := empty.Marshal()
	require.NoError(t, err)

	writer = NewTxnWriter(pstore)
	require.NoError(t, writer.SetAt(key, data, BitCompletePosting, 10))
	require.NoError(t, writer.Flush())
	assertLength(10, 0)

	addEdgeToUID(t, attr, 1, 5, 11, 12)
	addEdgeToUID(t, attr, 1, 6, 13, 14)
	addEdgeToUID(t, attr, 1, 7, 15, 16)
	assertLength(17, 3)
}
