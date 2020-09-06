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
	"fmt"
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
	pred  string
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
	pk, err := x.Parse(ce.Key())
	x.Check(err)

	sameIndexKey := pk.Attr == c.cur.pred && pk.IsReverse() == c.cur.rev
	if sameIndexKey && !c.cur.track {
		return
	}

	if !sameIndexKey {
		if c.countBuf.Len() > 0 {
			c.wg.Add(1)
			go c.writeIndex(c.countBuf)
			c.countBuf = z.NewBuffer(1 << 10)
		}
		c.cur.pred = pk.Attr
		c.cur.rev = pk.IsReverse()
		c.cur.track = c.schema.getSchema(pk.Attr).GetCount()
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
	var listSz int

	offsets := buf.SliceOffsets(nil)
	sort.Slice(offsets, func(i, j int) bool {
		left := countEntry(buf.Slice(offsets[i]))
		right := countEntry(buf.Slice(offsets[j]))
		return left.less(right)
	})

	lastCe := countEntry(buf.Slice(offsets[0]))
	{
		pk, err := x.Parse(lastCe.Key())
		x.Check(err)
		fmt.Printf("Writing count index for %q rev=%v\n", pk.Attr, pk.IsReverse())
	}

	var pl pb.PostingList
	encoder := codec.Encoder{BlockSize: 256}

	encode := func() {
		pl.Pack = encoder.Done()
		if codec.ExactLen(pl.Pack) == 0 {
			return
		}
		data, err := pl.Marshal()
		x.Check(err)
		codec.FreePack(pl.Pack)

		kv := &bpb.KV{
			Key:      append([]byte{}, lastCe.Key()...),
			Value:    data,
			UserMeta: []byte{posting.BitCompletePosting},
			Version:  c.state.writeTs,
			StreamId: streamId,
		}
		list.Kv = append(list.Kv, kv)
		listSz += kv.Size()
		encoder = codec.Encoder{BlockSize: 256}
		pl.Reset()

		// Flush out the buffer.
		if listSz > 4<<20 {
			x.Check(c.writer.Write(list))
			listSz = 0
			list = &bpb.KVList{}
		}
	}

	for _, offset := range offsets {
		ce := countEntry(buf.Slice(offset))
		if !bytes.Equal(lastCe.Key(), ce.Key()) {
			encode()
		}
		encoder.Add(ce.Uid())
		lastCe = ce
	}
	encode()
	x.Check(c.writer.Write(list))
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
