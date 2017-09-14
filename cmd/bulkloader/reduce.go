package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"io"
	"log"
	"os"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

var mePool = sync.Pool{
	New: func() interface{} {
		return new(protos.MapEntry)
	},
}

func readMapOutput(filename string, mapEntryCh chan<- *protos.MapEntry) {
	fd, err := os.Open(filename)
	x.Check(err)
	defer fd.Close()
	r := bufio.NewReaderSize(fd, 1<<20)

	unmarshalBuf := make([]byte, 1<<10)
	for {
		buf, err := r.Peek(binary.MaxVarintLen64)
		if err == io.EOF {
			break
		}
		x.Check(err)
		sz, n := binary.Uvarint(buf)
		if n <= 0 {
			log.Fatal("Could not read uvarint: %d", n)
		}
		x.Check2(r.Discard(n))

		for cap(unmarshalBuf) < int(sz) {
			unmarshalBuf = make([]byte, sz)
		}
		x.Check2(io.ReadFull(r, unmarshalBuf[:sz]))

		mapEntry := mePool.Get().(*protos.MapEntry)
		x.Check(proto.Unmarshal(unmarshalBuf[:sz], mapEntry))
		mapEntryCh <- mapEntry
	}
	close(mapEntryCh)
}

func shufflePostings(batchCh chan<- []*protos.MapEntry,
	mapEntryChs []chan *protos.MapEntry, prog *progress) {

	var ph postingHeap
	for _, ch := range mapEntryChs {
		heap.Push(&ph, heapNode{mapEntry: <-ch, ch: ch})
	}

	const batchSize = 1e5
	const batchAlloc = batchSize * 11 / 10
	batch := make([]*protos.MapEntry, 0, batchAlloc)
	var prevKey []byte
	for len(ph.nodes) > 0 {
		me := ph.nodes[0].mapEntry
		var ok bool
		ph.nodes[0].mapEntry, ok = <-ph.nodes[0].ch
		if ok {
			heap.Fix(&ph, 0)
		} else {
			heap.Pop(&ph)
		}

		if len(batch) >= batchSize && bytes.Compare(prevKey, me.Key) != 0 {
			batchCh <- batch
			batch = make([]*protos.MapEntry, 0, batchAlloc)
		}
		prevKey = me.Key
		batch = append(batch, me)
	}
	if len(batch) > 0 {
		batchCh <- batch
	}
}

type heapNode struct {
	mapEntry *protos.MapEntry
	ch       <-chan *protos.MapEntry
}

type postingHeap struct {
	nodes []heapNode
}

func (h *postingHeap) Len() int {
	return len(h.nodes)
}
func (h *postingHeap) Less(i, j int) bool {
	return bytes.Compare(h.nodes[i].mapEntry.Key, h.nodes[j].mapEntry.Key) < 0
}
func (h *postingHeap) Swap(i, j int) {
	h.nodes[i], h.nodes[j] = h.nodes[j], h.nodes[i]
}
func (h *postingHeap) Push(x interface{}) {
	h.nodes = append(h.nodes, x.(heapNode))
}
func (h *postingHeap) Pop() interface{} {
	elem := h.nodes[len(h.nodes)-1]
	h.nodes = h.nodes[:len(h.nodes)-1]
	return elem
}

func reduce(batch []*protos.MapEntry, kv *badger.KV, prog *progress, done func()) {
	var currentKey []byte
	var uids []uint64
	pl := new(protos.PostingList)
	var entries []*badger.Entry

	outputPostingList := func() {
		atomic.AddInt64(&prog.reduceKeyCount, 1)

		// For a UID-only posting list, the badger value is a delta packed UID
		// list. The UserMeta indicates to treat the value as a delta packed
		// list when the value is read by dgraph.  For a value posting list,
		// the full protos.Posting type is used (which internally contains the
		// delta packed UID list).
		e := &badger.Entry{Key: currentKey}
		if len(pl.Postings) == 0 {
			e.Value = bp128.DeltaPack(uids)
			e.UserMeta = 0x01
		} else {
			var err error
			pl.Uids = bp128.DeltaPack(uids)
			e.Value, err = pl.Marshal()
			x.Check(err)
		}
		entries = append(entries, e)

		uids = uids[:0]
		pl.Reset()
	}

	for _, mapEntry := range batch {
		atomic.AddInt64(&prog.reduceEdgeCount, 1)

		if bytes.Compare(mapEntry.Key, currentKey) != 0 && currentKey != nil {
			outputPostingList()
		}
		currentKey = mapEntry.Key

		if mapEntry.Posting == nil {
			uids = append(uids, mapEntry.Uid)
		} else {
			uids = append(uids, mapEntry.Posting.Uid)
			pl.Postings = append(pl.Postings, mapEntry.Posting)
		}
	}
	outputPostingList()

	kv.BatchSetAsync(entries, func(err error) {
		x.Check(err)
		for _, e := range entries {
			x.Check(e.Error)
		}
		// Reuse map entries.
		for _, me := range batch {
			me.Reset()
			mePool.Put(me)
		}
		done()
	})
}
