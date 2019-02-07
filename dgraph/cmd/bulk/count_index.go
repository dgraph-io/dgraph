/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bulk

import (
	"sort"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

type current struct {
	pred  string
	rev   bool
	track bool
}

type countIndexer struct {
	*state
	db     *badger.DB
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
	writer := posting.NewTxnWriter(c.db)

	for count, uids := range counts {
		sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })

		var pl pb.PostingList
		pl.Pack = codec.Encode(uids, 256)
		data, err := pl.Marshal()
		x.Check(err)
		x.Check(writer.SetAt(
			x.CountKey(pred, uint32(count), rev),
			data, posting.BitCompletePosting, c.state.writeTs))
	}
	x.Check(writer.Flush())
	c.wg.Done()
}

func (c *countIndexer) wait() {
	c.wg.Wait()
}
