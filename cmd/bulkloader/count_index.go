package main

import (
	"sort"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/x"
)

type countIndex struct {
	// maps from predicate to count to posting list
	postingLists map[string]map[int][]uint64

	// Cache most recently used predicate data to avoid map lookups.
	pred      string
	predLevel map[int][]uint64
}

func (c *countIndex) add(pred string, count int, uid uint64, reverse bool) {
	if c.pred != pred {
		var ok bool
		c.predLevel, ok = c.postingLists[pred]
		if !ok {
			c.predLevel = make(map[int][]uint64)
			c.postingLists[pred] = c.predLevel
		}
	}
	if reverse {
		// Reverse edges are encoded using a negative edge count.
		count = -count
	}
	c.predLevel[count] = append(c.predLevel[count], uid)
}

func (c *countIndex) merge(other *countIndex) {
	for pred, otherPredLevel := range other.postingLists {
		thisPredLevel, ok := c.postingLists[pred]
		if !ok {
			c.postingLists[pred] = otherPredLevel
			continue
		}
		for count, uidList := range otherPredLevel {
			thisPredLevel[count] = append(thisPredLevel[count], uidList...)
		}
	}
}

func (c *countIndex) write(kv *badger.KV) {
	// TODO: Should be able to do this in parallel.
	// TODO: Should batch up writes.
	for pred, predLevel := range c.postingLists {
		for count, uids := range predLevel {
			key := x.CountKey(pred, uint32(abs(count)), count < 0)
			sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
			val := bp128.DeltaPack(uids)
			kv.Set(key, val, 0x01)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
