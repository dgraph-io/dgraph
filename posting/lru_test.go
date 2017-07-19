/*
Copyright 2013 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package posting

import (
	"sync"
	"testing"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/stretchr/testify/require"
)

func getPosting() *List {
	l := &List{
		plist:  &protos.PostingList{},
		water:  marks.Get(1),
		mlayer: getNewSL(),
	}
	l.incr()
	return l
}

func TestLCacheSize(t *testing.T) {
	lcache := newListCache(500)

	for i := 0; i < 10; i++ {
		// Put a posting list of size 2
		l := getPosting()
		lcache.PutIfMissing(uint64(i), l)
		if i < 5 {
			require.Equal(t, lcache.curSize, uint64((i+1)*100))
		} else {
			require.Equal(t, lcache.curSize, uint64(500))
		}
	}

	require.Equal(t, lcache.evicts, uint64(5))
	require.Equal(t, lcache.ll.Len(), 5)
}

func TestLCacheSizeParallel(t *testing.T) {
	lcache := newListCache(5000)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		// Put a posting list of size 2
		go func(i int) {
			l := getPosting()
			lcache.PutIfMissing(uint64(i), l)
			wg.Done()
		}(i)
	}

	wg.Wait()
	require.Equal(t, lcache.curSize, uint64(5000))
	require.Equal(t, lcache.evicts, uint64(50))
	require.Equal(t, lcache.ll.Len(), 50)
}

func TestLCacheEviction(t *testing.T) {
	lcache := newListCache(5000)

	for i := 0; i < 100; i++ {
		l := getPosting()
		// Put a posting list of size 2
		lcache.PutIfMissing(uint64(i), l)
	}

	require.Equal(t, lcache.curSize, uint64(5000))
	require.Equal(t, lcache.evicts, uint64(50))
	require.Equal(t, lcache.ll.Len(), 50)

	for i := 0; i < 50; i++ {
		require.Nil(t, lcache.Get(uint64(i)))
	}
}

func TestLCachePutIfMissing(t *testing.T) {
	l := getPosting()
	lcache.PutIfMissing(1, l)
	require.Equal(t, l, lcache.Get(1))
	l2 := getPosting()
	lcache.PutIfMissing(1, l2)
	require.Equal(t, l, lcache.Get(1))
}
