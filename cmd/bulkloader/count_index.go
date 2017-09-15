package main

/*
type current struct {
	pred  string
	rev   bool
	track bool
}

type countIndexer struct {
	*state
	cur    current
	counts map[int][]uint64
	wg     sync.WaitGroup
}

// addUid adds the uid from rawKey to a count index if a count index is
// required by the schema. This method expects keys to be passed into it in
// sorted order.
func (c *countIndexer) addUid(rawKey []byte, count int) {
	key := x.Parse(rawKey)
	if !key.IsData() && !key.IsReverse() {
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
		c.cur.track = c.ss.getSchema(key.Attr).GetCount()
	}
	if c.cur.track {
		c.counts[count] = append(c.counts[count], key.Uid)
	}
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
*/
