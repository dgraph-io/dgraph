package main

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

func writePostings(dir string, postingsCh <-chan *protos.FlatPosting, prog *progress) int {

	var fileNum int
	var postings []*protos.FlatPosting
	var wg sync.WaitGroup
	var sz int

	processBatch := func() {
		wg.Add(1)
		filename := filepath.Join(dir, fmt.Sprintf("%06d.bin", fileNum))
		fileNum++
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
	return fileNum
}

func sortAndWrite(filename string, postings []*protos.FlatPosting, prog *progress) {
	sort.Slice(postings, func(i, j int) bool {
		return bytes.Compare(postings[i].Key, postings[j].Key) < 0
	})

	var buf proto.Buffer
	for _, posting := range postings {
		x.Check(buf.EncodeMessage(posting))
	}

	fd, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	x.Checkf(err, "Could not open tmp file.")
	x.Check2(fd.Write(buf.Bytes()))
	x.Check(fd.Sync())
	x.Check(fd.Close())
}

func readFlatFile(dir string, fid int, postingCh chan<- *protos.FlatPosting) {
	filename := filepath.Join(dir, fmt.Sprintf("%06d.bin", fid))
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
		n, err = io.ReadFull(r, unmarshalBuf[:sz])
		x.Check(err)

		flatPosting := new(protos.FlatPosting)
		x.Check(proto.Unmarshal(unmarshalBuf[:sz], flatPosting))
		postingCh <- flatPosting
	}
	close(postingCh)
}

func shuffleFlatFiles(dir string, postingChs []chan *protos.FlatPosting) {
	var fileNum int
	var wg sync.WaitGroup
	var prevKey []byte

	var ph postingHeap
	for _, ch := range postingChs {
		heap.Push(&ph, heapNode{<-ch, ch})
	}

	var buf proto.Buffer
	for len(ph.data) > 0 {
		msg := ph.data[0].head
		var ok bool
		ph.data[0].head, ok = <-ph.data[0].ch
		if ok {
			heap.Fix(&ph, 0)
		} else {
			heap.Pop(&ph)
		}

		x.Check(buf.EncodeMessage(msg))

		if len(buf.Bytes()) > 32<<20 && bytes.Compare(prevKey, msg.Key) != 0 {
			filename := filepath.Join(dir, fmt.Sprintf("merged_%6d.bin", fileNum))
			fileNum++
			wg.Add(1)
			go func(buf []byte) {
				ioutil.WriteFile(filename, buf, 0644)
				wg.Done()
			}(buf.Bytes())
			buf.SetBuf(nil)
		}
		prevKey = msg.Key
	}
	wg.Wait()
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
