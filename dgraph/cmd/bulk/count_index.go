/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/golang/glog"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/hypermodeinc/dgraph/v25/codec"
	"github.com/hypermodeinc/dgraph/v25/posting"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/x"
)

// type countEntry struct {
// uid uint64
// key []byte
// }

type countEntry []byte

func countEntrySize(key []byte) int {
	return 8 + 4 + len(key)
}
func marshalCountEntry(dst, key []byte, uid uint64) {
	binary.BigEndian.PutUint64(dst[0:8], uid)

	binary.BigEndian.PutUint32(dst[8:12], uint32(len(key)))
	n := copy(dst[12:], key)
	x.AssertTrue(len(dst) == n+12)
}
func (ci countEntry) Uid() uint64 {
	return binary.BigEndian.Uint64(ci[0:8])
}
func (ci countEntry) Key() []byte {
	sz := binary.BigEndian.Uint32(ci[8:12])
	return ci[12 : 12+sz]
}
func (ci countEntry) less(oe countEntry) bool {
	lk, rk := ci.Key(), oe.Key()
	if cmp := bytes.Compare(lk, rk); cmp != 0 {
		return cmp < 0
	}
	return ci.Uid() < oe.Uid()
}

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
		if c.countBuf.LenNoPadding() > 0 {
			c.wg.Add(1)
			go c.writeIndex(c.countBuf)
			c.countBuf = getBuf(c.opt.TmpDir)
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
		if err := buf.Release(); err != nil {
			glog.Warningf("error in releasing buffer: %v", err)
		}

	}()
	if buf.IsEmpty() {
		return
	}

	streamId := atomic.AddUint32(&c.streamId, 1)
	buf.SortSlice(func(ls, rs []byte) bool {
		left := countEntry(ls)
		right := countEntry(rs)
		return left.less(right)
	})

	tmp, _ := buf.Slice(buf.StartOffset())
	lastCe := countEntry(tmp)
	{
		pk, err := x.Parse(lastCe.Key())
		x.Check(err)
		fmt.Printf("Writing count index for %q rev=%v\n", pk.Attr, pk.IsReverse())
	}

	alloc := z.NewAllocator(8<<20, "CountIndexer.WriteIndex")
	defer alloc.Release()

	var pl pb.PostingList
	encoder := codec.Encoder{BlockSize: 256, Alloc: alloc}

	outBuf := z.NewBuffer(5<<20, "CountIndexer.Buffer.WriteIndex")
	defer func() {
		if err := outBuf.Release(); err != nil {
			glog.Warningf("error in releasing buffer: %v", err)
		}
	}()
	encode := func() {
		pl.Pack = encoder.Done()
		if codec.ExactLen(pl.Pack) == 0 {
			return
		}

		kv := posting.MarshalPostingList(&pl, nil)
		kv.Key = append([]byte{}, lastCe.Key()...)
		kv.Version = c.state.writeTs
		kv.StreamId = streamId
		badger.KVToBuffer(kv, outBuf)

		alloc.Reset()
		encoder = codec.Encoder{BlockSize: 256, Alloc: alloc}
		pl.Reset()

		// flush out the buffer.
		if outBuf.LenNoPadding() > 4<<20 {
			x.Check(c.writer.Write(outBuf))
			outBuf.Reset()
		}
	}

	if err := buf.SliceIterate(func(slice []byte) error {
		ce := countEntry(slice)
		if !bytes.Equal(lastCe.Key(), ce.Key()) {
			encode()
		}
		encoder.Add(ce.Uid())
		lastCe = ce
		return nil
	}); err != nil {
		glog.Errorf("error while counting in buf: %v\n", err)
		x.Check(err)
	}
	encode()
	x.Check(c.writer.Write(outBuf))
}

func (c *countIndexer) wait() {
	if c.countBuf.LenNoPadding() > 0 {
		c.wg.Add(1)
		go c.writeIndex(c.countBuf)
	} else {
		if err := c.countBuf.Release(); err != nil {
			glog.Warningf("error in releasing buffer: %v", err)
		}
	}
	c.wg.Wait()
}
