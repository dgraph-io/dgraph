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
	"bufio"
	"bytes"
	"container/heap"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/dgraph-io/badger"
	bo "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

type shuffler struct {
	*state
}

func (s *shuffler) run() {
	shardDirs := shardDirs(s.opt.TmpDir)
	x.AssertTrue(len(shardDirs) == s.opt.ReduceShards)
	x.AssertTrue(len(s.opt.shardOutputDirs) == s.opt.ReduceShards)

	thr := x.NewThrottle(s.opt.NumShufflers)
	for i := 0; i < s.opt.ReduceShards; i++ {
		thr.Start()
		go func(shardId int, db *badger.DB) {
			mapFiles := filenamesInTree(shardDirs[shardId])
			shuffleInputChs := make([]chan *pb.MapEntry, len(mapFiles))
			for i, mapFile := range mapFiles {
				shuffleInputChs[i] = make(chan *pb.MapEntry, 1000)
				go readMapOutput(mapFile, shuffleInputChs[i])
			}

			ci := &countIndexer{state: s.state, db: db}
			s.shufflePostings(shuffleInputChs, ci)
			ci.wait()
			thr.Done()
		}(i, s.createBadger(i))
	}
	thr.Wait()
}

func (s *shuffler) createBadger(i int) *badger.DB {
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

func readMapOutput(filename string, mapEntryCh chan<- *pb.MapEntry) {
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
			log.Fatalf("Could not read uvarint: %d", n)
		}
		x.Check2(r.Discard(n))

		for cap(unmarshalBuf) < int(sz) {
			unmarshalBuf = make([]byte, sz)
		}
		x.Check2(io.ReadFull(r, unmarshalBuf[:sz]))

		me := new(pb.MapEntry)
		x.Check(proto.Unmarshal(unmarshalBuf[:sz], me))
		mapEntryCh <- me
	}

	close(mapEntryCh)
}

func (s *shuffler) shufflePostings(mapEntryChs []chan *pb.MapEntry, ci *countIndexer) {
	var ph postingHeap
	for _, ch := range mapEntryChs {
		heap.Push(&ph, heapNode{mapEntry: <-ch, ch: ch})
	}

	db := ci.db
	writer := db.NewStreamWriter()
	if err := writer.Prepare(); err != nil {
		panic(err)
	}
	defer func() {
		if err := writer.Flush(); err != nil {
			panic(err)
		}
	}()

	const batchSize = 1000
	const batchAlloc = batchSize * 11 / 10
	batch := make([]*pb.MapEntry, 0, batchAlloc)
	var prevKey []byte
	var slast []byte
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
		if bytes.Compare(me.Key, prevKey) < 0 {
			panic("what")
		}

		keyChanged := !bytes.Equal(prevKey, me.Key)
		// TODO: Bring this back.
		// if keyChanged && plistLen > 0 {
		// 	ci.addUid(prevKey, plistLen)
		// 	plistLen = 0
		// }

		if len(batch) >= batchSize && keyChanged {
			list := reduce(batch)
			for _, kv := range list.Kv {
				if bytes.Compare(kv.Key, slast) <= 0 {
					panic("wrong")
				}
				slast = y.Copy(kv.Key)
			}
			x.Check(writer.Write(list))
			NumQueuedReduceJobs.Add(1)
			batch = make([]*pb.MapEntry, 0, batchAlloc)
		}
		prevKey = me.Key
		batch = append(batch, me)
		plistLen++
	}
	fmt.Println("Done with ph.nodes")
	if len(batch) > 0 {
		// reduce(shuffleOutput{mapEntries: batch, writer: writer})
		// x.Check(job.writer.Write(list))
		NumQueuedReduceJobs.Add(1)
	}
	if plistLen > 0 {
		ci.addUid(prevKey, plistLen)
	}
	fmt.Println("Done shuffling.")
}

type heapNode struct {
	mapEntry *pb.MapEntry
	ch       <-chan *pb.MapEntry
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
