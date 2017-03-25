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

package posting

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashBasic(t *testing.T) {
	a := newShardedListMap(32)
	require.Equal(t, a.Size(), 0)

	l1 := new(List)
	require.Equal(t, a.PutIfMissing(123, l1), l1)

	l2 := new(List)
	require.Equal(t, a.PutIfMissing(123, l2), l1)

	l := a.Get(123)
	require.Equal(t, l, l1)

	a.Each(func(k uint64, l *List) {
		require.EqualValues(t, k, 123)
		require.Equal(t, l, l1)
	})

	a.EachWithDelete(func(k uint64, l *List) {
		require.EqualValues(t, k, 123)
		require.Equal(t, l, l1)
	})

	require.Equal(t, a.Size(), 0)
}

// Good for testing for race conditions.
func TestHashConcurrent(t *testing.T) {
	a := newShardedListMap(1)
	var lists []*List
	for i := 0; i < 1000; i++ {
		lists = append(lists, new(List))
	}

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a.PutIfMissing(uint64(i), lists[i])
		}(i)
	}
	wg.Wait()

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l := a.Get(uint64(i))
			require.Equal(t, l, lists[i])
		}(i)
	}
	wg.Wait()
}
