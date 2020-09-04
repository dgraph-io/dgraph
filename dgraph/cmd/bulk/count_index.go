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
	"bytes"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger/v2"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

type current struct {
	pred  []byte
	rev   bool
	track bool
}

type countIndexer struct {
	*reducer
	writer      *badger.StreamWriter
	splitWriter *badger.WriteBatch
	splitCh     chan *badger.KVList
	tmpDb       *badger.DB
	cur         current
	countBuf    *z.Buffer
	wg          sync.WaitGroup
}

// addUid adds the uid from rawKey to a count index if a count index is
// required by the schema. This method expects keys to be passed into it in
// sorted order.
func (c *countIndexer) addCountEntry(ce countEntry) {
	sameIndexKey := bytes.Equal(ce.Attr(), c.cur.pred) && ce.Reverse() == c.cur.rev
	if sameIndexKey && !c.cur.track {
		return
	}

	if !sameIndexKey {
		if c.countBuf.Len() > 0 {
			c.wg.Add(1)
			go c.writeIndex(c.countBuf)
		}
		c.countBuf = z.NewBuffer(1 << 10)
		c.cur.pred = append(c.cur.pred[:0], ce.Attr()...)
		c.cur.rev = ce.Reverse()
		c.cur.track = c.schema.getSchema(string(ce.Attr())).GetCount()
	}
	if c.cur.track {
		dst := c.countBuf.SliceAllocate(len(ce))
		copy(dst, ce)
	}
}

func (c *countIndexer) writeIndex(buf *z.Buffer) {
	defer func() {
		c.wg.Done()
		buf.Release()
	}()
	if buf.Len() == 0 {
		return
	}

	streamId := atomic.AddUint32(&c.streamId, 1)
	list := &bpb.KVList{}

	var pl pb.PostingList
	encoder := codec.Encoder{BlockSize: 256}

	lastCe := countEntry(buf.Slice(0))
	offset := 0
	for offset < buf.Len() {
		ce := countEntry(buf.Slice(offset))
		offset += 4 + len(ce)

		// Sanity checks.
		x.AssertTrue(bytes.Equal(ce.Attr(), lastCe.Attr()))
		x.AssertTrue(ce.Reverse() == lastCe.Reverse())

		if ce.Count() == lastCe.Count() {
			encoder.Add(ce.Uid())
			continue
		}

		pl.Pack = encoder.Done()
		data, err := pl.Marshal()
		x.Check(err)
		codec.FreePack(pl.Pack)

		key := x.CountKey(string(lastCe.Attr()), lastCe.Count(), lastCe.Reverse())
		list.Kv = append(list.Kv, &bpb.KV{
			Key:      key,
			Value:    data,
			UserMeta: []byte{posting.BitCompletePosting},
			Version:  c.state.writeTs,
			StreamId: streamId,
		})
		encoder = codec.Encoder{BlockSize: 256}
		pl.Reset()
		lastCe = ce
	}

	sort.Slice(list.Kv, func(i, j int) bool {
		return bytes.Compare(list.Kv[i].Key, list.Kv[j].Key) < 0
	})
	if err := c.writer.Write(list); err != nil {
		x.Check(err)
	}
}

func (c *countIndexer) wait() {
	if c.countBuf.Len() > 0 {
		c.wg.Add(1)
		go c.writeIndex(c.countBuf)
	} else {
		c.countBuf.Release()
	}
	c.wg.Wait()
}
