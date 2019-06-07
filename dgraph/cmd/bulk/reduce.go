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
	"sync/atomic"

	"github.com/dgraph-io/badger"
	bo "github.com/dgraph-io/badger/options"
	bpb "github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

type reducer struct {
	*state
}

func (r *reducer) run() error {
	shardDirs := shardDirs(r.opt.TmpDir)
	x.AssertTrue(len(shardDirs) == r.opt.ReduceShards)
	x.AssertTrue(len(r.opt.shardOutputDirs) == r.opt.ReduceShards)

	thr := y.NewThrottle(r.opt.NumReducers)
	for i := 0; i < r.opt.ReduceShards; i++ {
		if err := thr.Do(); err != nil {
			return err
		}
		go func(shardId int, db *badger.DB) {
			defer thr.Done(nil)

			mapFiles := filenamesInTree(shardDirs[shardId])
			mapEntryChs := make([]chan *pb.MapEntry, len(mapFiles))
			for i, mapFile := range mapFiles {
				mapEntryChs[i] = make(chan *pb.MapEntry, 1000)
				go readMapOutput(mapFile, mapEntryChs[i])
			}

			writer := db.NewStreamWriter()
			if err := writer.Prepare(); err != nil {
				panic(err)
			}
			defer func() {
				fmt.Println("Calling writer.Flush")
				if err := writer.Flush(); err != nil {
					panic(err)
				}
			}()

			ci := &countIndexer{state: r.state, writer: writer}
			r.reduce(mapEntryChs, ci)
			ci.wait()
		}(i, r.createBadger(i))
	}
	return thr.Finish()
}

func (s *reducer) createBadger(i int) *badger.DB {
	opt := badger.DefaultOptions
	opt.SyncWrites = false
	opt.TableLoadingMode = bo.MemoryMap
	opt.ValueThreshold = 1 << 10 // 1 KB.
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

type entryBatch struct {
	entries []*pb.MapEntry
}

func (r *reducer) encodeAndWrite(
	writer *badger.StreamWriter, entryCh chan []*pb.MapEntry, closer *y.Closer) {
	defer closer.Done()

	var listSize int
	list := &bpb.KVList{}
	for batch := range entryCh {
		listSize += r.toList(batch, list)
		if listSize > 4<<20 {
			x.Check(writer.Write(list))
			list = &bpb.KVList{}
			listSize = 0
		}
	}
	if len(list.Kv) > 0 {
		x.Check(writer.Write(list))
	}
}

func (r *reducer) reduce(mapEntryChs []chan *pb.MapEntry, ci *countIndexer) {
	entryCh := make(chan []*pb.MapEntry, 100)
	closer := y.NewCloser(1)
	defer closer.SignalAndWait()
	defer close(entryCh)

	var ph postingHeap
	for _, ch := range mapEntryChs {
		heap.Push(&ph, heapNode{mapEntry: <-ch, ch: ch})
	}

	writer := ci.writer
	go r.encodeAndWrite(writer, entryCh, closer)

	const batchSize = 100000
	const batchAlloc = batchSize * 11 / 10
	batch := make([]*pb.MapEntry, 0, batchAlloc)
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

		keyChanged := !bytes.Equal(prevKey, me.Key)
		if keyChanged && plistLen > 0 {
			ci.addUid(prevKey, plistLen)
			plistLen = 0
		}

		if len(batch) >= batchSize && keyChanged {
			entryCh <- batch
			batch = make([]*pb.MapEntry, 0, batchAlloc)
		}
		prevKey = me.Key
		batch = append(batch, me)
		plistLen++
	}
	if len(batch) > 0 {
		entryCh <- batch
	}
	if plistLen > 0 {
		ci.addUid(prevKey, plistLen)
	}
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

func (r *reducer) toList(mapEntries []*pb.MapEntry, list *bpb.KVList) int {
	var currentKey []byte
	var uids []uint64
	pl := new(pb.PostingList)
	var size int

	appendToList := func() {
		atomic.AddInt64(&r.prog.reduceKeyCount, 1)

		// For a UID-only posting list, the badger value is a delta packed UID
		// list. The UserMeta indicates to treat the value as a delta packed
		// list when the value is read by dgraph.  For a value posting list,
		// the full pb.Posting type is used (which pb.y contains the
		// delta packed UID list).
		if len(uids) == 0 {
			return
		}
		pl.Pack = codec.Encode(uids, 256)
		val, err := pl.Marshal()
		x.Check(err)
		kv := &bpb.KV{
			Key:      y.Copy(currentKey),
			Value:    val,
			UserMeta: []byte{posting.BitCompletePosting},
			Version:  1, // Should probably be writeTs TODO
		}
		size += kv.Size()
		list.Kv = append(list.Kv, kv)
		uids = uids[:0]
		pl.Reset()
	}

	for _, mapEntry := range mapEntries {
		atomic.AddInt64(&r.prog.reduceEdgeCount, 1)

		if !bytes.Equal(mapEntry.Key, currentKey) && currentKey != nil {
			appendToList()
		}
		currentKey = mapEntry.Key

		uid := mapEntry.Uid
		if mapEntry.Posting != nil {
			uid = mapEntry.Posting.Uid
		}
		if len(uids) > 0 && uids[len(uids)-1] == uid {
			continue
		}
		uids = append(uids, uid)
		if mapEntry.Posting != nil {
			pl.Postings = append(pl.Postings, mapEntry.Posting)
		}
	}
	appendToList()
	return size

	// NumBadgerWrites.Add(1)

	// TODO: Bring this back.
	// for _, kv := range list.Kv {
	// 	pk := x.Parse(kv.Key)
	// 	fmt.Printf("pk: %+v\n", pk)
	// }
	// x.Check(job.writer.Write(list))

	// x.Check(txn.CommitAt(r.state.writeTs, func(err error) {
	// 	x.Check(err)
	// 	NumBadgerWrites.Add(-1)
	// }))
}
