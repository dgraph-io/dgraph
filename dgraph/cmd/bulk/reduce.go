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
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync/atomic"

	"github.com/davecgh/go-spew/spew"
	"github.com/dgraph-io/badger/v2"
	bo "github.com/dgraph-io/badger/v2/options"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
)

type reducer struct {
	*state
	streamId uint32
}

const batchSize = 10000
const batchAlloc = batchSize * 11 / 10

func (r *reducer) run() error {
	dirs := readShardDirs(filepath.Join(r.opt.TmpDir, reduceShardDir))
	x.AssertTrue(len(dirs) == r.opt.ReduceShards)
	x.AssertTrue(len(r.opt.shardOutputDirs) == r.opt.ReduceShards)
	fmt.Println(r.opt.ReduceShards, r.opt.NumReducers)

	thr := y.NewThrottle(r.opt.NumReducers)
	for i := 0; i < r.opt.ReduceShards; i++ {
		if err := thr.Do(); err != nil {
			return err
		}
		go func(shardId int, db *badger.DB) {
			defer thr.Done(nil)

			mapFiles := filenamesInTree(dirs[shardId])
			var mapItrs []*mapIterator
			partitionKeys := make([][]byte, 0)
			tmpMap := make(map[string]struct{})
			for _, mapFile := range mapFiles {
				header, itr := newMapIterator(mapFile)
				for _, key := range header.PartitionKeys {
					if _, ok := tmpMap[string(key)]; !ok {
						partitionKeys = append(partitionKeys, key)
						tmpMap[string(key)] = struct{}{}
					}
				}
				mapItrs = append(mapItrs, itr)
			}
			tmpMap = nil

			writer := db.NewStreamWriter()
			if err := writer.Prepare(); err != nil {
				x.Check(err)
			}

			ci := &countIndexer{reducer: r, writer: writer}
			sort.Slice(partitionKeys, func(i, j int) bool {
				return bytes.Compare(partitionKeys[i], partitionKeys[j]) < 0
			})
			// Start bactching for the given keys
			for _, itr := range mapItrs {
				go itr.startBatchingForKeys(partitionKeys)
			}
			r.reduce(partitionKeys, mapItrs, ci)
			ci.wait()

			if err := writer.Flush(); err != nil {
				x.Check(err)
			}
			for _, itr := range mapItrs {
				if err := itr.Close(); err != nil {
					fmt.Printf("Error while closing iterator: %v", err)
				}
			}
		}(i, r.createBadger(i))
	}
	return thr.Finish()
}

func (r *reducer) createBadger(i int) *badger.DB {
	if r.opt.BadgerKeyFile != "" {
		// need to set zero addr in WorkerConfig before doing license check.
		x.WorkerConfig.ZeroAddr = r.opt.ZeroAddr
		// non-nil key file
		if !worker.EnterpriseEnabled() {
			// not licensed --> crash.
			log.Fatal("Enterprise License needed for the Encryption feature.")
		} else {
			// licensed --> OK.
			log.Printf("Encryption feature enabled. Using encryption key file: %v", r.opt.BadgerKeyFile)
		}
	}

	opt := badger.DefaultOptions(r.opt.shardOutputDirs[i]).WithSyncWrites(false).
		WithTableLoadingMode(bo.MemoryMap).WithValueThreshold(1 << 10 /* 1 KB */).
		WithLogger(nil).WithMaxCacheSize(1 << 20).
		WithEncryptionKey(enc.ReadEncryptionKeyFile(r.opt.BadgerKeyFile))

	// TOOD(Ibrahim): Remove this once badger is updated.
	opt.ZSTDCompressionLevel = 1

	db, err := badger.OpenManaged(opt)
	x.Check(err)

	// zero out the key from memory.
	opt.EncryptionKey = nil

	r.dbs = append(r.dbs, db)
	return db
}

type mapIterator struct {
	fd      *os.File
	reader  *bufio.Reader
	tmpBuf  []byte
	current *iteratorEntry
	batchCh chan *iteratorEntry
}

type iteratorEntry struct {
	partitionKey []byte
	batch        []*pb.MapEntry
}

func (mi *mapIterator) startBatchingForKeys(partitionsKeys [][]byte) {
	for _, key := range partitionsKeys {
		mi.pushToBatchCh(key)
	}
	mi.pushToBatchCh(nil)
	close(mi.batchCh)
}

func (mi *mapIterator) pushToBatchCh(key []byte) {
	batch := make([]*pb.MapEntry, 0, batchAlloc)
	r := mi.reader
	for {
		// LOSING one item.
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

		if cap(mi.tmpBuf) < int(sz) {
			mi.tmpBuf = make([]byte, sz)
		}
		x.Check2(io.ReadFull(r, mi.tmpBuf[:sz]))

		me := new(pb.MapEntry)
		x.Check(proto.Unmarshal(mi.tmpBuf[:sz], me))

		isSmaller := bytes.Compare(me.Key, key) <= 0
		if key == nil || isSmaller {
			batch = append(batch, me)
		} else if !isSmaller {
			break
		}
	}
	mi.batchCh <- &iteratorEntry{
		partitionKey: key,
		batch:        batch,
	}
}

func (mi *mapIterator) Close() error {
	return mi.fd.Close()
}

func (mi *mapIterator) Current() *iteratorEntry {
	return mi.current
}

func (mi *mapIterator) Next() bool {
	mi.current = <-mi.batchCh
	return mi.current != nil
}

func newMapIterator(filename string) (*pb.MapperHeader, *mapIterator) {
	fd, err := os.Open(filename)
	x.Check(err)
	gzReader, err := gzip.NewReader(fd)
	x.Check(err)

	// Read the header size.
	reader := bufio.NewReaderSize(gzReader, 16<<10)
	headerLenBuf := make([]byte, 8)
	x.Check2(io.ReadFull(reader, headerLenBuf))
	headerLen := binary.BigEndian.Uint64(headerLenBuf)
	// Reader the map header.
	headerBuf := make([]byte, headerLen)

	x.Check2(io.ReadFull(reader, headerBuf))
	header := &pb.MapperHeader{}
	err = header.Unmarshal(headerBuf)
	x.Check(err)

	itr := &mapIterator{fd: fd, reader: reader, batchCh: make(chan *iteratorEntry, 1000)}
	return header, itr
}

type encodeRequest struct {
	entries []*pb.MapEntry
	seqNo   uint64
}

type writerRequest struct {
	list  *bpb.KVList
	seqNo uint64
}

func (r *reducer) encodeMapEntry(tid int, entryCh chan *encodeRequest, writerCh chan *writerRequest,
	closer *y.Closer) {
	defer closer.Done()

	list := &bpb.KVList{}
	for req := range entryCh {
		for _, kv := range req.entries {
			fmt.Println(tid, req.seqNo, kv.Key)
		}
		r.toList(req.entries, list)
		writerCh <- &writerRequest{
			list:  list,
			seqNo: req.seqNo,
		}
	}
}

func (r *reducer) startWriting(writer *badger.StreamWriter, writerCh chan *writerRequest,
	closer *y.Closer) {
	defer closer.Done()

	preds := make(map[string]uint32)
	seqMap := make(map[uint64]*writerRequest)
	setStreamId := func(kv *bpb.KV) {
		pk, err := x.Parse(kv.Key)
		x.Check(err)
		x.AssertTrue(len(pk.Attr) > 0)

		// We don't need to consider the data prefix, count prefix, etc. because each predicate
		// contains sorted keys, the way they are produced.
		streamId := preds[pk.Attr]
		if streamId == 0 {
			streamId = atomic.AddUint32(&r.streamId, 1)
			preds[pk.Attr] = streamId
		}

		kv.StreamId = streamId
	}
	nextIdx := uint64(1)
	spew.Config.MaxDepth = 1
	for req := range writerCh {
		seqMap[req.seqNo] = req
		// fmt.Printf("SeqNo: %d First key: %+v Version: %d\nLast Key: %+v Version: %d\n", req.seqNo, req.list.GetKv()[0].Key, req.list.GetKv()[0].GetVersion(), req.list.GetKv()[len(req.list.GetKv())-1].Key, req.list.GetKv()[len(req.list.GetKv())-1].GetVersion())
		for {
			nextReq, ok := seqMap[nextIdx]
			if !ok {
				break
			}
			// fmt.Println("Next SeqNo", nextReq.seqNo, "NextIdx", nextIdx)
			lastKey := []byte{0}
			for _, kv := range nextReq.list.Kv {
				y.AssertTrue(bytes.Compare(lastKey, kv.GetKey()) < 0)
				lastKey = kv.GetKey()
				// fmt.Println("SeqNo:", nextReq.seqNo, "Key from dgraph:", kv.GetKey(), "Version: ", kv.GetVersion())
				setStreamId(kv)
			}
			x.Check(writer.Write(nextReq.list))
			nextIdx++
		}
	}
}

func (r *reducer) encodeAndWrite(
	writer *badger.StreamWriter, entryCh chan []*pb.MapEntry, closer *y.Closer) {
	defer closer.Done()

	var listSize int
	list := &bpb.KVList{}

	preds := make(map[string]uint32)
	setStreamId := func(kv *bpb.KV) {
		pk, err := x.Parse(kv.Key)
		x.Check(err)
		x.AssertTrue(len(pk.Attr) > 0)

		// We don't need to consider the data prefix, count prefix, etc. because each predicate
		// contains sorted keys, the way they are produced.
		streamId := preds[pk.Attr]
		if streamId == 0 {
			streamId = atomic.AddUint32(&r.streamId, 1)
			preds[pk.Attr] = streamId
		}

		kv.StreamId = streamId
	}

	// Once we have processed all records from single stream, we can mark that stream as done.
	// This will close underlying table builder in Badger for stream. Since we preallocate 1 MB
	// of memory for each table builder, this can result in memory saving in case we have large
	// number of streams.
	// This change limits maximum number of open streams to number of streams created in a single
	// write call. This can also be optimised if required.
	// addDone := func(doneSteams []uint32, l *bpb.KVList) {
	// 	for _, streamId := range doneSteams {
	// 		l.Kv = append(l.Kv, &bpb.KV{StreamId: streamId, StreamDone: true})
	// 	}
	// }

	// var doneStreams []uint32
	// var prevSID uint32
	for batch := range entryCh {
		sort.Slice(batch, func(i, j int) bool {
			return less(batch[i], batch[j])
		})
		listSize += r.toList(batch, list)
		if listSize > 4<<20 {
			for _, kv := range list.Kv {
				setStreamId(kv)
				// if prevSID != 0 && (prevSID != kv.StreamId) {
				// 	doneStreams = append(doneStreams, prevSID)
				// }
				// prevSID = kv.StreamId
			}
			// addDone(doneStreams, list)
			x.Check(writer.Write(list))
			//doneStreams = doneStreams[:0]
			list = &bpb.KVList{}
			listSize = 0
		}
	}
	if len(list.Kv) > 0 {
		for _, kv := range list.Kv {
			setStreamId(kv)
		}
		x.Check(writer.Write(list))
	}
}

func (r *reducer) reduce(partitionKeys [][]byte, mapItrs []*mapIterator, ci *countIndexer) {
	encoderCh := make(chan *encodeRequest, 10)
	writerCh := make(chan *writerRequest, 10)
	encoderCloser := y.NewCloser(runtime.NumCPU())

	for i := 0; i < runtime.NumCPU(); i++ {
		// Start listening to encode entries
		go r.encodeMapEntry(i, encoderCh, writerCh, encoderCloser)
	}

	// Start lisenting to write the badger list.
	writerCloser := y.NewCloser(1)
	go r.startWriting(ci.writer, writerCh, writerCloser)
	seqNo := uint64(0)
	batch := make([]*pb.MapEntry, 0, batchAlloc)
	var lastKey []byte
	for i := 0; i < len(partitionKeys); i++ {
		fmt.Println(i, ":", partitionKeys[i])
		for _, itr := range mapItrs {
			if !itr.Next() {
				continue
			}
			res := itr.Current()
			// fmt.Println("Batch first", res.batch[0].Key)
			y.AssertTrue(bytes.Equal(res.partitionKey, partitionKeys[i]))
			batch = append(batch, res.batch...)
		}
		// fmt.Printf("collected into batch. batchlen: %d\n", len(batch))
		sort.Slice(batch, func(p, q int) bool {
			return less(batch[p], batch[q])
		})
		fmt.Println("*************paritition key:", partitionKeys[i])
		// for _, b := range batch {
		// 	fmt.Println("b", seqNo, b.Key)
		// }
		if len(batch) == 0 {
			continue
		}
		y.AssertTrue(bytes.Compare(lastKey, batch[len(batch)-1].Key) < 0)
		// for _, b := range batch {
		// 	fmt.Println("key:", b.Key)
		// }
		// fmt.Printf("First Key: %+v\nSecondKey: %+v\nLastt Key: %+v\n\n", batch[0].Key, batch[1].Key, batch[(len(batch))-1].Key)
		seqNo++
		encoderCh <- &encodeRequest{entries: batch, seqNo: seqNo}
		batch = make([]*pb.MapEntry, 0, batchAlloc)
	}

	// // Drain the last batch
	// var count int
	// for i := 0; i < 2; i++ {
	// 	count := 0
	// 	for _, itr := range mapItrs {
	// 		itr.Next()
	// 		res := itr.Current()
	// 		if res == nil {
	// 			continue
	// 		}
	// 		count++
	// 		y.AssertTrue(res.partitionKey == nil)
	// 		batch = append(batch, res.batch...)
	// 	}
	// 	seqNo++
	// 	encoderCh <- &encodeRequest{entries: batch, seqNo: seqNo}
	// 	fmt.Println("*****************", count)
	// }

	// y.AssertTrue(count <= 1)

	// Close the encoders
	close(encoderCh)
	encoderCloser.SignalAndWait()

	// Close the writer
	close(writerCh)
	writerCloser.SignalAndWait()
}

type heapNode struct {
	mapEntry *pb.MapEntry
	itr      *mapIterator
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

func (h *postingHeap) Push(val interface{}) {
	h.nodes = append(h.nodes, val.(heapNode))
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

	userMeta := []byte{posting.BitCompletePosting}
	writeVersionTs := r.state.writeTs

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

		// If the schema is of type uid and not a list but we have more than one uid in this
		// list, we cannot enforce the constraint without losing data. Inform the user and
		// force the schema to be a list so that all the data can be found when Dgraph is started.
		// The user should fix their data once Dgraph is up.
		parsedKey, err := x.Parse(currentKey)
		x.Check(err)
		if parsedKey.IsData() {
			schema := r.state.schema.getSchema(parsedKey.Attr)
			if schema.GetValueType() == pb.Posting_UID && !schema.GetList() && len(uids) > 1 {
				fmt.Printf("Schema for pred %s specifies that this is not a list but more than  "+
					"one UID has been found. Forcing the schema to be a list to avoid any "+
					"data loss. Please fix the data to your specifications once Dgraph is up.\n",
					parsedKey.Attr)
				r.state.schema.setSchemaAsList(parsedKey.Attr)
			}
		}

		pl.Pack = codec.Encode(uids, 256)
		val, err := pl.Marshal()
		x.Check(err)
		kv := &bpb.KV{
			Key:      y.Copy(currentKey),
			Value:    val,
			UserMeta: userMeta,
			Version:  writeVersionTs,
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
}
