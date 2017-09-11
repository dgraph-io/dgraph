package main

import (
	"sort"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

type countIndexer struct {
	pred   string
	rev    bool
	schema *protos.SchemaUpdate
	counts map[int][]uint64
	kv     *badger.KV
	wg     sync.WaitGroup
	ss     *schemaStore
}

func (c *countIndexer) add(rawKey []byte, count int) {
	key := x.Parse(rawKey)
	if !key.IsData() && !key.IsReverse() {
		return
	}
	if key.IsReverse() != c.rev || key.Attr != c.pred {
		if len(c.counts) > 0 {
			c.wg.Add(1)
			go c.writeIndex(c.pred, c.rev, c.counts)
		}
		c.pred = key.Attr
		c.rev = key.IsReverse()
		c.counts = make(map[int][]uint64)
		c.schema = c.ss.getSchema(key.Attr)
	}
	// If the schema is not set explicitly but instead discovered from the RDF
	// data, c.schema may be nil (it may not have been discovered yet). This is
	// okay, since the default for GetCount is false (discovered schemas cannot
	// have indexes).
	if !c.schema.GetCount() {
		return
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
