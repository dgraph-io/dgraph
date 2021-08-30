/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package algo

import (
	"testing"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

func newList(data []uint64) *pb.List {
	return &pb.List{SortedUids: data}
}

func TestApplyFilterUint(t *testing.T) {
	l := []uint64{1, 2, 3, 4, 5}
	u := newList(l)
	ApplyFilter(u, func(a uint64, idx int) bool { return (l[idx] % 2) == 1 })
	require.Equal(t, []uint64{1, 3, 5}, codec.GetUids(u))
}
