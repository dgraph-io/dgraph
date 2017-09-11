package main

import (
	"sort"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/x"
)

type current struct {
	pred  string
	rev   bool
	track bool
}

type countIndexer struct {
	curr   current
	counts map[int][]uint64
	wg     sync.WaitGroup
	*state
}

// addUid adds the uid from rawKey to a count index if a count index is
// required by the schema. This method expects keys to be passed into it in
// sorted order.
func (c *countIndexer) addUid(rawKey []byte, count int) {
	key := x.Parse(rawKey)
	if !key.IsData() && !key.IsReverse() {
		return
	}
	sameIndexKey := key.Attr == c.curr.pred && key.IsReverse() == c.curr.rev
	if sameIndexKey && !c.curr.track {
		return
	}

	if !sameIndexKey {
		if len(c.counts) > 0 {
			c.wg.Add(1)
			go c.writeIndex(c.curr.pred, c.curr.rev, c.counts)
			c.counts = make(map[int][]uint64)
		}
		c.curr.pred = key.Attr
		c.curr.rev = key.IsReverse()
		c.curr.track = c.ss.getSchema(key.Attr).GetCount()
	}
	c.counts[count] = append(c.counts[count], key.Uid)
}

func (c *countIndexer) writeIndex(pred string, rev bool, counts map[int][]uint64) {
	entries := make([]*badger.Entry, 0, len(counts))
	for count, uids := range counts {
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		entries = append(entries, &badger.Entry{
			Key:      x.CountKey(pred, uint32(count), rev),
			Value:    bp128.DeltaPack(uids),
			UserMeta: 0x01,
		})
	}
	x.Check(c.kv.BatchSet(entries))
	for _, e := range entries {
		x.Check(e.Error)
	}
	c.wg.Done()
}

func (c *countIndexer) wait() {
	c.wg.Wait()
}
