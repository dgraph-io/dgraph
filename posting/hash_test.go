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

	l, found := a.Get(123)
	require.True(t, found)
	require.Equal(t, l, l1)

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
			l, _ := a.Get(uint64(i))
			require.Equal(t, l, lists[i])
		}(i)
	}
	wg.Wait()
}
