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
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/stretchr/testify/require"
)

func postingLen2() *List {
	l := &List{
		plist: &protos.PostingList{
			Commit: 1,
		},
		water: marks.Get(1),
	}
	l.incr()
	return l
}

func TestLCacheSize(t *testing.T) {
	lcache := newListCache(100)

	for i := 0; i < 150; i++ {
		// Put a posting list of size 2
		l := postingLen2()
		lcache.PutIfMissing(uint64(i), l)
		if i < 50 {
			require.Equal(t, lcache.curSize, uint64((i+1)*2))
		} else {
			require.Equal(t, lcache.curSize, uint64(100))
		}
	}

	require.Equal(t, lcache.evicts, uint64(100))
	require.Equal(t, lcache.ll.Len(), 50)
}

func TestLCacheSizeParallel(t *testing.T) {
	lcache := newListCache(100)

	var wg sync.WaitGroup
	for i := 0; i < 150; i++ {
		wg.Add(1)
		// Put a posting list of size 2
		go func(i int) {
			l := postingLen2()
			lcache.PutIfMissing(uint64(i), l)
			wg.Done()
		}(i)
	}

	wg.Wait()
	require.Equal(t, lcache.curSize, uint64(100))
	require.Equal(t, lcache.evicts, uint64(100))
	require.Equal(t, lcache.ll.Len(), 50)
}

func TestLCacheEviction(t *testing.T) {
	lcache := newListCache(100)

	for i := 0; i < 150; i++ {
		l := postingLen2()
		// Put a posting list of size 2
		lcache.PutIfMissing(uint64(i), l)
	}

	require.Equal(t, lcache.curSize, uint64(100))
	require.Equal(t, lcache.evicts, uint64(100))
	require.Equal(t, lcache.ll.Len(), 50)

	time.Sleep(time.Second)
	for i := 0; i < 100; i++ {
		require.Nil(t, lcache.Get(uint64(i)))
	}
}
