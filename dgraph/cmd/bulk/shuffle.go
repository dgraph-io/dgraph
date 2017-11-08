/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package bulk

import (
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"io"
	"log"
	"os"

	"github.com/dgraph-io/badger"
	bo "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

type shuffler struct {
	*state
	output chan<- shuffleOutput
}

func (s *shuffler) run() {
	shardDirs := shardDirs(s.opt.TmpDir)
	x.AssertTrue(len(shardDirs) == s.opt.ReduceShards)
	x.AssertTrue(len(s.opt.shardOutputDirs) == s.opt.ReduceShards)

	thr := x.NewThrottle(s.opt.NumShufflers)
	for i := 0; i < s.opt.ReduceShards; i++ {
		thr.Start()
		go func(i int, db *badger.ManagedDB) {
			mapFiles := filenamesInTree(shardDirs[i])
			shuffleInputChs := make([]chan *protos.MapEntry, len(mapFiles))
			for i, mapFile := range mapFiles {
				shuffleInputChs[i] = make(chan *protos.MapEntry, 1000)
				go readMapOutput(mapFile, shuffleInputChs[i])
			}

			ci := &countIndexer{state: s.state, db: db}
			s.shufflePostings(shuffleInputChs, ci)
			ci.wait()
			thr.Done()
		}(i, s.createBadger(i))
	}
	thr.Wait()
	close(s.output)
}

func (s *shuffler) createBadger(i int) *badger.ManagedDB {
	opt := badger.DefaultOptions
	opt.SyncWrites = false
	opt.TableLoadingMode = bo.MemoryMap
	opt.Dir = s.opt.shardOutputDirs[i]
	opt.ValueDir = opt.Dir
	db, err := badger.OpenManaged(opt)
	x.Check(err)
	s.dbs = append(s.dbs, db)
	return db
}

func readMapOutput(filename string, mapEntryCh chan<- *protos.MapEntry) {
	fd, err := os.Open(filename)
	x.Check(err)
	defer fd.Close()
	r := bufio.NewReaderSize(fd, 16<<10)

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

		me := new(protos.MapEntry)
		x.Check(proto.Unmarshal(unmarshalBuf[:sz], me))
		mapEntryCh <- me
	}
	close(mapEntryCh)
}

func (s *shuffler) shufflePostings(mapEntryChs []chan *protos.MapEntry, ci *countIndexer) {

	var ph postingHeap
	for _, ch := range mapEntryChs {
		heap.Push(&ph, heapNode{mapEntry: <-ch, ch: ch})
	}

	const batchSize = 1000
	const batchAlloc = batchSize * 11 / 10
	batch := make([]*protos.MapEntry, 0, batchAlloc)
	var prevKey []byte
	var plistLen int
	for len(ph.nodes) > 0 {
		me := ph.nodes[0].mapEntry
		var ok bool
		ph.nodes[0].mapEntry, ok = <-ph.nodes[0].ch
		if ok {
			heap.Fix(&ph, 0)
		} else {
			heap.Pop(&ph)
		}

		keyChanged := bytes.Compare(prevKey, me.Key) != 0
		if keyChanged && plistLen > 0 {
			ci.addUid(prevKey, plistLen)
			plistLen = 0
		}

		if len(batch) >= batchSize && bytes.Compare(prevKey, me.Key) != 0 {
			s.output <- shuffleOutput{mapEntries: batch, db: ci.db}
			NumQueuedReduceJobs.Add(1)
			batch = make([]*protos.MapEntry, 0, batchAlloc)
		}
		prevKey = me.Key
		batch = append(batch, me)
		plistLen++
	}
	if len(batch) > 0 {
		s.output <- shuffleOutput{mapEntries: batch, db: ci.db}
		NumQueuedReduceJobs.Add(1)
	}
	if plistLen > 0 {
		ci.addUid(prevKey, plistLen)
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
	return less(h.nodes[i].mapEntry, h.nodes[j].mapEntry)
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
