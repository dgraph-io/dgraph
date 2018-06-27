/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
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
