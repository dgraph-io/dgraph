package main

import (
	"sort"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

type current struct {
	pred  string
	rev   bool
	track bool
}

type countIndexer struct {
	*state
	db     *badger.ManagedDB
	cur    current
	counts map[int][]uint64
	wg     sync.WaitGroup
}

// addUid adds the uid from rawKey to a count index if a count index is
// required by the schema. This method expects keys to be passed into it in
// sorted order.
func (c *countIndexer) addUid(rawKey []byte, count int) {
	key := x.Parse(rawKey)
	if key == nil || (!key.IsData() && !key.IsReverse()) {
		return
	}
	sameIndexKey := key.Attr == c.cur.pred && key.IsReverse() == c.cur.rev
	if sameIndexKey && !c.cur.track {
		return
	}

	if !sameIndexKey {
		if len(c.counts) > 0 {
			c.wg.Add(1)
			go c.writeIndex(c.cur.pred, c.cur.rev, c.counts)
		}
		if len(c.counts) > 0 || c.counts == nil {
			c.counts = make(map[int][]uint64)
		}
		c.cur.pred = key.Attr
		c.cur.rev = key.IsReverse()
		c.cur.track = c.schema.getSchema(key.Attr).GetCount()
	}
	if c.cur.track {
		c.counts[count] = append(c.counts[count], key.Uid)
	}
}

func (c *countIndexer) writeIndex(pred string, rev bool, counts map[int][]uint64) {
	txn := c.db.NewTransactionAt(c.state.writeTs, true)
	for count, uids := range counts {
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
		x.Check(txn.Set(
			x.CountKey(pred, uint32(count), rev),
			bp128.DeltaPack(uids),
			posting.BitCompletePosting|posting.BitUidPosting,
		))
	}
	x.Check(txn.CommitAt(c.state.writeTs, nil))
	c.wg.Done()
}

func (c *countIndexer) wait() {
	c.wg.Wait()
}
