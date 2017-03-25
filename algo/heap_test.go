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

package algo

import (
	"container/heap"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPush(t *testing.T) {
	h := &uint64Heap{}
	heap.Init(h)

	e := elem{val: 5}
	heap.Push(h, e)
	e.val = 3
	heap.Push(h, e)
	e.val = 4
	heap.Push(h, e)

	require.Equal(t, h.Len(), 3)
	require.EqualValues(t, (*h)[0].val, 3)

	e.val = 10
	(*h)[0] = e
	heap.Fix(h, 0)
	require.EqualValues(t, (*h)[0].val, 4)

	e.val = 11
	(*h)[0] = e
	heap.Fix(h, 0)
	require.EqualValues(t, (*h)[0].val, 5)

	e = heap.Pop(h).(elem)
	require.EqualValues(t, e.val, 5)

	e = heap.Pop(h).(elem)
	require.EqualValues(t, e.val, 10)

	e = heap.Pop(h).(elem)
	require.EqualValues(t, e.val, 11)

	require.Equal(t, h.Len(), 0)
}
