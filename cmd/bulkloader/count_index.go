package main

import "github.com/dgraph-io/badger"

type countIndex struct {
	// maps from predicate to count to posting list
	postingLists map[string]map[int][]uint64

	// Cache most recently used predicate data to avoid map lookups.
	pred      string
	predLevel map[int][]uint64
}

func (c *countIndex) add(pred string, count int, uid uint64) {
	if c.pred != pred {
		var ok bool
		c.predLevel, ok = c.postingLists[pred]
		if !ok {
			c.predLevel = make(map[int][]uint64)
			c.postingLists[pred] = c.predLevel
		}
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
	// TODO sorting first. This should be easily done in parallel.
}
