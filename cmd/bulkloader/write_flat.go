package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

func writeMappedFiles(dir string, postingsCh <-chan *protos.FlatPosting, prog *progress) []string {

	var filenames []string
	var fileNum int
	var postings []*protos.FlatPosting
	var wg sync.WaitGroup
	var sz int

	processBatch := func() {
		wg.Add(1)
		filename := filepath.Join(dir, fmt.Sprintf("map_%06d.bin", fileNum))
		fileNum++
		filenames = append(filenames, filename)
		ps := postings
		postings = nil
		sz = 0
		go func() {
			sortAndWrite(filename, ps, prog)
			wg.Done()
		}()
	}

	for posting := range postingsCh {
		postings = append(postings, posting)
		sz += posting.Size()
		if sz > 256<<20 {
			processBatch()
		}
	}
	if len(postings) > 0 {
		processBatch()
	}

	wg.Wait()
	return filenames
}

func sortAndWrite(filename string, postings []*protos.FlatPosting, prog *progress) {
	sort.Slice(postings, func(i, j int) bool {
		return bytes.Compare(postings[i].Key, postings[j].Key) < 0
	})

	var buf proto.Buffer
	for _, posting := range postings {
		x.Check(buf.EncodeMessage(posting))
	}

	x.Check(x.WriteFileSync(filename, buf.Bytes(), 0644))
}

func readFlatFile(filename string, postingCh chan<- *protos.FlatPosting) {
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
			log.Fatal("Could not read varint: %d", n)
		}
		x.Check2(r.Discard(n))

		for len(unmarshalBuf) < int(sz) {
			unmarshalBuf = make([]byte, 2*len(unmarshalBuf))
		}
		x.Check2(io.ReadFull(r, unmarshalBuf[:sz]))

		flatPosting := new(protos.FlatPosting)
		x.Check(proto.Unmarshal(unmarshalBuf[:sz], flatPosting))
		postingCh <- flatPosting
	}
	close(postingCh)
}

func shuffleFlatFiles(batchCh chan []*protos.FlatPosting,
	postingChs []chan *protos.FlatPosting, prog *progress) {

	var ph postingHeap
	for _, ch := range postingChs {
		heap.Push(&ph, heapNode{head: <-ch, ch: ch})
	}

	const batchSize = 1e5
	const batchAlloc = batchSize * 11 / 10
	batch := make([]*protos.FlatPosting, 0, batchAlloc)
	var prevKey []byte
	for len(ph.data) > 0 {
		p := ph.data[0].head
		var ok bool
		ph.data[0].head, ok = <-ph.data[0].ch
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
		atomic.AddInt64(&prog.shuffleEdgeCount, 1)
	}
	if len(batch) > 0 {
		batchCh <- batch
	}
}

type heapNode struct {
	head *protos.FlatPosting
	ch   <-chan *protos.FlatPosting
}

type postingHeap struct {
	data []heapNode
}

func (h *postingHeap) Len() int {
	return len(h.data)
}
func (h *postingHeap) Less(i, j int) bool {
	return bytes.Compare(h.data[i].head.Key, h.data[j].head.Key) < 0
}
func (h *postingHeap) Swap(i, j int) {
	h.data[i], h.data[j] = h.data[j], h.data[i]
}
func (h *postingHeap) Push(x interface{}) {
	h.data = append(h.data, x.(heapNode))
}
func (h *postingHeap) Pop() interface{} {
	elem := h.data[len(h.data)-1]
	h.data = h.data[:len(h.data)-1]
	return elem
}
