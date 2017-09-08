package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"io"
	"log"
	"os"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

func readMapOutput(filename string, postingCh chan<- *protos.FlatPosting) {
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

		flatPosting := new(protos.FlatPosting)
		x.Check(proto.Unmarshal(unmarshalBuf[:sz], flatPosting))
		postingCh <- flatPosting
	}
	close(postingCh)
}

func shufflePostings(batchCh chan<- []*protos.FlatPosting,
	postingChs []chan *protos.FlatPosting, prog *progress) {

	var ph postingHeap
	for _, ch := range postingChs {
		heap.Push(&ph, heapNode{posting: <-ch, ch: ch})
	}

	const batchSize = 1e5
	const batchAlloc = batchSize * 11 / 10
	batch := make([]*protos.FlatPosting, 0, batchAlloc)
	var prevKey []byte
	for len(ph.nodes) > 0 {
		p := ph.nodes[0].posting
		var ok bool
		ph.nodes[0].posting, ok = <-ph.nodes[0].ch
		if ok {
			heap.Fix(&ph, 0)
		} else {
			heap.Pop(&ph)
		}

		if len(batch) >= batchSize && bytes.Compare(prevKey, p.Key) != 0 {
			batchCh <- batch
			batch = make([]*protos.FlatPosting, 0, batchAlloc)
		}
		prevKey = p.Key

		batch = append(batch, p)
	}
	if len(batch) > 0 {
		batchCh <- batch
	}
	close(batchCh)
}

type heapNode struct {
	posting *protos.FlatPosting
	ch      <-chan *protos.FlatPosting
}

type postingHeap struct {
	nodes []heapNode
}

func (h *postingHeap) Len() int {
	return len(h.nodes)
}
func (h *postingHeap) Less(i, j int) bool {
	return bytes.Compare(h.nodes[i].posting.Key, h.nodes[j].posting.Key) < 0
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
