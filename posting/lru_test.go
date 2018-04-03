/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package posting

import (
	"fmt"
	"sync"
	"testing"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/stretchr/testify/require"
)

func getPosting() *List {
	l := &List{
		plist: &intern.PostingList{},
	}
	return l
}

func TestLCacheSize(t *testing.T) {
	lcache := newListCache(500)

	for i := 0; i < 10; i++ {
		// Put a posting list of size 2
		l := getPosting()
		lcache.PutIfMissing(fmt.Sprintf("%d", i), l)
		lcache.removeOldest()
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
			lcache.PutIfMissing(fmt.Sprintf("%d", i), l)
			lcache.removeOldest()
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
		lcache.PutIfMissing(fmt.Sprintf("%d", i), l)
		lcache.removeOldest()
	}

	require.Equal(t, lcache.curSize, uint64(5000))
	require.Equal(t, lcache.evicts, uint64(50))
	require.Equal(t, lcache.ll.Len(), 50)

	for i := 0; i < 50; i++ {
		require.Nil(t, lcache.Get(fmt.Sprintf("%d", i)))
	}
}

func TestLCachePutIfMissing(t *testing.T) {
	l := getPosting()
	lcache.PutIfMissing("1", l)
	require.Equal(t, l, lcache.Get("1"))
	l2 := getPosting()
	lcache.PutIfMissing("1", l2)
	require.Equal(t, l, lcache.Get("1"))
}
