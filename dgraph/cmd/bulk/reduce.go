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
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

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
	"github.com/dgraph-io/ristretto/z"
	"github.com/dustin/go-humanize"
)

type reducer struct {
	*state
	streamId  uint32
	mu        sync.RWMutex
	streamIds map[string]uint32
}

const batchSize = 10000
const maxSplitBatchLen = 1000
const batchAlloc = batchSize * 11 / 10

func (r *reducer) run() error {
	dirs := readShardDirs(filepath.Join(r.opt.TmpDir, reduceShardDir))
	x.AssertTrue(len(dirs) == r.opt.ReduceShards)
	x.AssertTrue(len(r.opt.shardOutputDirs) == r.opt.ReduceShards)

	thr := y.NewThrottle(r.opt.NumReducers)
	for i := 0; i < r.opt.ReduceShards; i++ {
		if err := thr.Do(); err != nil {
			return err
		}
		go func(shardId int, db *badger.DB, tmpDb *badger.DB) {
			defer thr.Done(nil)

			mapFiles := filenamesInTree(dirs[shardId])
			var mapItrs []*mapIterator
			partitionKeys := [][]byte{}
			for _, mapFile := range mapFiles {
				header, itr := newMapIterator(mapFile)
				partitionKeys = append(partitionKeys, header.PartitionKeys...)
				mapItrs = append(mapItrs, itr)
			}

			writer := db.NewStreamWriter()
			x.Check(writer.Prepare())
			// Split lists are written to a separate DB first to avoid ordering issues.
			splitWriter := tmpDb.NewManagedWriteBatch()

			ci := &countIndexer{
				reducer:     r,
				writer:      writer,
				splitWriter: splitWriter,
				tmpDb:       tmpDb,
			}
			sort.Slice(partitionKeys, func(i, j int) bool {
				return bytes.Compare(partitionKeys[i], partitionKeys[j]) < 0
			})

			// Start batching for the given keys.
			for _, itr := range mapItrs {
				go itr.startBatching(partitionKeys)
			}
			r.reduce(partitionKeys, mapItrs, ci)
			ci.wait()

			x.Check(writer.Flush())
			fmt.Println("Writing split lists back to the main DB now")

			// Write split lists back to the main DB.
			r.writeSplitLists(db, tmpDb)

			for _, itr := range mapItrs {
				if err := itr.Close(); err != nil {
					fmt.Printf("Error while closing iterator: %v", err)
				}
			}
		}(i, r.createBadger(i), r.createTmpBadger())
	}
	return thr.Finish()
}

func (r *reducer) createBadgerInternal(dir string, compression bool) *badger.DB {
	if r.opt.BadgerKeyFile != "" {
		// Need to set zero addr in WorkerConfig before checking the license.
		x.WorkerConfig.ZeroAddr = []string{r.opt.ZeroAddr}

		if !worker.EnterpriseEnabled() {
			// Crash since the enterprise license is not enabled..
			log.Fatal("Enterprise License needed for the Encryption feature.")
		} else {
			log.Printf("Encryption feature enabled. Using encryption key file: %v", r.opt.BadgerKeyFile)
		}
	}

	opt := badger.DefaultOptions(dir).WithSyncWrites(false).
		WithTableLoadingMode(bo.MemoryMap).WithValueThreshold(1 << 10 /* 1 KB */).
		WithLogger(nil).WithBlockCacheSize(256 << 20).
		WithEncryptionKey(enc.ReadEncryptionKeyFile(r.opt.BadgerKeyFile)).WithCompression(bo.None)

	// Overwrite badger options based on the options provided by the user.
	r.setBadgerOptions(&opt, compression)

	db, err := badger.OpenManaged(opt)
	x.Check(err)

	// Zero out the key from memory.
	opt.EncryptionKey = nil
	return db
}

func (r *reducer) createBadger(i int) *badger.DB {
	db := r.createBadgerInternal(r.opt.shardOutputDirs[i], true)
	r.dbs = append(r.dbs, db)
	return db
}

func (r *reducer) createTmpBadger() *badger.DB {
	tmpDir, err := ioutil.TempDir(r.opt.TmpDir, "split")
	x.Check(err)
	// Do not enable compression in temporary badger to improve performance.
	db := r.createBadgerInternal(tmpDir, false)
	r.tmpDbs = append(r.tmpDbs, db)
	return db
}

func (r *reducer) setBadgerOptions(opt *badger.Options, compression bool) {
	if !compression {
		opt.Compression = bo.None
		opt.ZSTDCompressionLevel = 0
		return
	}
	// Set the compression level.
	opt.ZSTDCompressionLevel = r.state.opt.BadgerCompressionLevel
	if r.state.opt.BadgerCompressionLevel < 1 {
		x.Fatalf("Invalid compression level: %d. It should be greater than zero",
			r.state.opt.BadgerCompressionLevel)
	}
}

type mapIterator struct {
	fd       *os.File
	reader   *bufio.Reader
	batchCh  chan *iteratorEntry
	freelist chan *iteratorEntry
}

type iteratorEntry struct {
	partitionKey []byte
	cbuf         *z.Buffer
}

func (mi *mapIterator) release(ie *iteratorEntry) {
	ie.cbuf.Release()
}

func (mi *mapIterator) startBatching(partitionsKeys [][]byte) {
	var ie *iteratorEntry
	prevKeyExist := false
	var meBuf, key []byte
	var cbuf *z.Buffer
	// readKey reads the next map entry key.
	readMapEntry := func() error {
		if prevKeyExist {
			return nil
		}
		r := mi.reader
		sizeBuf, err := r.Peek(binary.MaxVarintLen64)
		if err != nil {
			return err
		}
		sz, n := binary.Uvarint(sizeBuf)
		if n <= 0 {
			log.Fatalf("Could not read uvarint: %d", n)
		}
		x.Check2(r.Discard(n))
		if cap(meBuf) < int(sz) {
			meBuf = make([]byte, int(sz))
		}
		meBuf = meBuf[:int(sz)]
		x.Check2(io.ReadFull(r, meBuf))
		key = MapEntry(meBuf).Key()
		return nil
	}

	for _, pKey := range partitionsKeys {
		ie = &iteratorEntry{partitionKey: pKey, cbuf: z.NewBuffer(64)}
		for {
			err := readMapEntry()
			if err == io.EOF {
				break
			}
			x.Check(err)

			if bytes.Compare(key, ie.partitionKey) < 0 {
				b := ie.cbuf.SliceAllocate(len(meBuf))
				copy(b, meBuf)
				prevKeyExist = false
				// map entry is already part of cBuf.
				continue
			}
			// Current key is not part of this batch so track that we have already read the key.
			prevKeyExist = true
			break
		}
		mi.batchCh <- ie
	}

	// Drain the last items.
	cbuf = z.NewBuffer(64)
	for {
		err := readMapEntry()
		if err == io.EOF {
			break
		}
		x.Check(err)
		b := cbuf.SliceAllocate(len(meBuf))
		copy(b, meBuf)
		prevKeyExist = false
	}
	mi.batchCh <- &iteratorEntry{
		cbuf:         cbuf,
		partitionKey: nil,
	}
}

func (mi *mapIterator) Close() error {
	return mi.fd.Close()
}

func (mi *mapIterator) Next() *iteratorEntry {
	return <-mi.batchCh
}

func newMapIterator(filename string) (*pb.MapHeader, *mapIterator) {
	fd, err := os.Open(filename)
	x.Check(err)
	gzReader, err := gzip.NewReader(fd)
	x.Check(err)

	// Read the header size.
	reader := bufio.NewReaderSize(gzReader, 16<<10)
	headerLenBuf := make([]byte, 4)
	x.Check2(io.ReadFull(reader, headerLenBuf))
	headerLen := binary.BigEndian.Uint32(headerLenBuf)
	// Reader the map header.
	headerBuf := make([]byte, headerLen)

	x.Check2(io.ReadFull(reader, headerBuf))
	header := &pb.MapHeader{}
	err = header.Unmarshal(headerBuf)
	x.Check(err)

	itr := &mapIterator{
		fd:       fd,
		reader:   reader,
		batchCh:  make(chan *iteratorEntry, 3),
		freelist: make(chan *iteratorEntry, 64),
	}
	return header, itr
}

type countIndexEntry struct {
	key   []byte
	count int
}

type encodeRequest struct {
	cbuf      *z.Buffer
	countKeys []*countIndexEntry
	wg        *sync.WaitGroup
	list      *bpb.KVList
	splitList *bpb.KVList
	offsets   []int
}

func (r *reducer) streamIdFor(pred string) uint32 {
	r.mu.RLock()
	if id, ok := r.streamIds[pred]; ok {
		r.mu.RUnlock()
		return id
	}
	r.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.streamIds[pred]; ok {
		return id
	}
	streamId := atomic.AddUint32(&r.streamId, 1)
	r.streamIds[pred] = streamId
	return streamId
}

func (r *reducer) encode(entryCh chan *encodeRequest, closer *y.Closer) {
	defer closer.Done()

	// Create a slice of offsets.
	// var offset []int
	// var sortedOffsets []int
	var offsets []int
	for req := range entryCh {
		req.offsets = offsets
		req.list = &bpb.KVList{}
		req.splitList = &bpb.KVList{}

		countKeys := r.toList(req)
		offsets = req.offsets

		for _, kv := range req.list.Kv {
			pk, err := x.Parse(kv.Key)
			x.Check(err)
			x.AssertTrue(len(pk.Attr) > 0)
			kv.StreamId = r.streamIdFor(pk.Attr)
		}
		req.countKeys = countKeys
		req.wg.Done()

		// for {
		// 	var ms runtime.MemStats
		// 	runtime.ReadMemStats(&ms)
		// 	gomem := int64(ms.HeapInuse / (1 << 20))
		// 	cmem := z.NumAllocsMB()
		// 	if gomem+cmem < 16384 { // TODO: Make this a flag.
		// 		break
		// 	}
		// 	fmt.Printf("Sleeping to allow memory usage to reduce before processing more requests."+
		// 		" Gomem: %d Cmem: %d\n", gomem, cmem)
		// 	time.Sleep(15 * time.Second)
		// }
	}
}

func (r *reducer) writeTmpSplits(ci *countIndexer, kvsCh chan *bpb.KVList, wg *sync.WaitGroup) {
	defer wg.Done()
	splitBatchLen := 0

	for kvs := range kvsCh {
		if kvs == nil || len(kvs.Kv) == 0 {
			continue
		}

		for i := 0; i < len(kvs.Kv); i += maxSplitBatchLen {
			// Flush the write batch when the max batch length is reached to prevent the
			// value log from growing over the allowed limit.
			if splitBatchLen >= maxSplitBatchLen {
				x.Check(ci.splitWriter.Flush())
				ci.splitWriter = ci.tmpDb.NewManagedWriteBatch()
				splitBatchLen = 0
			}

			batch := &bpb.KVList{}
			if i+maxSplitBatchLen >= len(kvs.Kv) {
				batch.Kv = kvs.Kv[i:]
			} else {
				batch.Kv = kvs.Kv[i : i+maxSplitBatchLen]
			}
			splitBatchLen += len(batch.Kv)
			x.Check(ci.splitWriter.Write(batch))
		}
	}
	x.Check(ci.splitWriter.Flush())
}

func (r *reducer) startWriting(ci *countIndexer, writerCh chan *encodeRequest, closer *y.Closer) {
	defer closer.Done()

	// Concurrently write split lists to a temporary badger.
	tmpWg := new(sync.WaitGroup)
	tmpWg.Add(1)
	splitCh := make(chan *bpb.KVList, 2*runtime.NumCPU())
	go r.writeTmpSplits(ci, splitCh, tmpWg)

	for req := range writerCh {
		req.wg.Wait()

		for _, countKey := range req.countKeys {
			ci.addUid(countKey.key, countKey.count)
		}
		// Wait for it to be encoded.
		start := time.Now()

		x.Check(ci.writer.Write(req.list))
		if req.splitList != nil && len(req.splitList.Kv) > 0 {
			splitCh <- req.splitList
		}

		if dur := time.Since(start).Round(time.Millisecond); dur > time.Second {
			fmt.Printf("writeCh: Time taken to write req: %v\n",
				time.Since(start).Round(time.Millisecond))
		}
	}

	// Wait for split lists to be written to the temporary badger.
	close(splitCh)
	tmpWg.Wait()
}

func (r *reducer) writeSplitLists(db, tmpDb *badger.DB) {
	txn := tmpDb.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	itr := txn.NewIterator(badger.DefaultIteratorOptions)
	defer itr.Close()

	writer := db.NewManagedWriteBatch()
	splitBatchLen := 0

	for itr.Rewind(); itr.Valid(); itr.Next() {
		// Flush the write batch when the max batch length is reached to prevent the
		// value log from growing over the allowed limit.
		if splitBatchLen >= maxSplitBatchLen {
			x.Check(writer.Flush())
			writer = db.NewManagedWriteBatch()
			splitBatchLen = 0
		}
		item := itr.Item()

		valCopy, err := item.ValueCopy(nil)
		x.Check(err)
		kv := &bpb.KV{
			Key:       item.KeyCopy(nil),
			Value:     valCopy,
			UserMeta:  []byte{item.UserMeta()},
			Version:   item.Version(),
			ExpiresAt: item.ExpiresAt(),
		}
		x.Check(writer.Write(&bpb.KVList{Kv: []*bpb.KV{kv}}))
		splitBatchLen += 1
	}
	x.Check(writer.Flush())
}

func (r *reducer) reduce(partitionKeys [][]byte, mapItrs []*mapIterator, ci *countIndexer) {
	cpu := r.opt.NumGoroutines
	fmt.Printf("Num Encoders: %d\n", cpu)
	encoderCh := make(chan *encodeRequest, 2*cpu)
	writerCh := make(chan *encodeRequest, 2*cpu)
	encoderCloser := y.NewCloser(cpu)
	for i := 0; i < cpu; i++ {
		// Start listening to encode entries
		// For time being let's lease 100 stream id for each encoder.
		go r.encode(encoderCh, encoderCloser)
	}
	// Start listening to write the badger list.
	writerCloser := y.NewCloser(1)
	go r.startWriting(ci, writerCh, writerCloser)

	throttle := func() {
		for {
			sz := atomic.LoadInt64(&r.prog.numEncoding)
			if sz < 512<<20 {
				return
			}
			fmt.Printf("Not sending out more encoder load. Num Bytes being encoded: %d\n", sz)
			time.Sleep(10 * time.Second)
		}
	}

	numSkipped := 0
	for i := 0; i < len(partitionKeys); i++ {
		throttle()
		cbuf := z.NewBuffer(4 << 20)
		for _, itr := range mapItrs {
			res := itr.Next()
			y.AssertTrue(bytes.Equal(res.partitionKey, partitionKeys[i]))
			cbuf.Write(res.cbuf.Bytes())
			itr.release(res)
		}
		if cbuf.Len() < 2<<30 {
			numSkipped++
			if numSkipped%1000 == 999 {
				fmt.Printf("Skipped %d buffers\n", numSkipped)
			}
			cbuf.Release()
			continue
		}
		fmt.Printf("Encoding a buffer of size: %s\n", humanize.Bytes(uint64(cbuf.Len())))
		atomic.AddInt64(&r.prog.numEncoding, int64(cbuf.Len()))

		wg := new(sync.WaitGroup)
		wg.Add(1)
		req := &encodeRequest{cbuf: cbuf, wg: wg}
		encoderCh <- req
		writerCh <- req
	}

	throttle()
	fmt.Println("Draining the last batch")
	cbuf := z.NewBuffer(1 << 20)
	for _, itr := range mapItrs {
		res := itr.Next()
		y.AssertTrue(res.partitionKey == nil)
		cbuf.Write(res.cbuf.Bytes())
		itr.release(res)
	}
	atomic.AddInt64(&r.prog.numEncoding, int64(cbuf.Len()))

	wg := new(sync.WaitGroup)
	wg.Add(1)
	req := &encodeRequest{cbuf: cbuf, wg: wg}
	encoderCh <- req
	writerCh <- req

	// Close the encodes.
	close(encoderCh)
	encoderCloser.SignalAndWait()

	// Close the writer.
	close(writerCh)
	writerCloser.SignalAndWait()
}

type MapEntry []byte

// type mapEntry struct {
// 	uid   uint64 // if plist is filled, then corresponds to plist's uid.
// 	key   []byte
// 	plist []byte
// }

func mapEntrySize(key []byte, p *pb.Posting) int {
	return 8 + 4 + 4 + len(key) + p.Size()
}

func marshalMapEntry(dst []byte, uid uint64, key []byte, p *pb.Posting) {
	if p != nil {
		uid = p.Uid
	}
	binary.BigEndian.PutUint64(dst[0:8], uid)
	binary.BigEndian.PutUint32(dst[8:12], uint32(len(key)))

	psz := p.Size()
	binary.BigEndian.PutUint32(dst[12:16], uint32(psz))

	n := copy(dst[16:], key)

	if psz > 0 {
		pbuf := dst[16+n:]
		_, err := p.MarshalToSizedBuffer(pbuf[:psz])
		x.Check(err)
	}

	x.AssertTrue(len(dst) == 16+n+psz)
}

func (me MapEntry) Size() int {
	return len(me)
}

func (me MapEntry) Uid() uint64 {
	return binary.BigEndian.Uint64(me[0:8])
}

func (me MapEntry) Key() []byte {
	sz := binary.BigEndian.Uint32(me[8:12])
	return me[16 : 16+sz]
}

func (me MapEntry) Plist() []byte {
	ksz := binary.BigEndian.Uint32(me[8:12])
	sz := binary.BigEndian.Uint32(me[12:16])
	start := 16 + ksz
	return me[start : start+sz]
}

func (r *reducer) toList(req *encodeRequest) []*countIndexEntry {
	cbuf := req.cbuf
	defer func() {
		atomic.AddInt64(&r.prog.numEncoding, -int64(cbuf.Len()))
		cbuf.Release()
	}()

	list := req.list
	splitList := req.splitList
	req.offsets = cbuf.SliceOffsets(req.offsets[:0])
	sort.Slice(req.offsets, func(i, j int) bool {
		lhs := MapEntry(cbuf.Slice(req.offsets[i]))
		rhs := MapEntry(cbuf.Slice(req.offsets[j]))
		return bytes.Compare(lhs.Key(), rhs.Key()) < 0
	})

	var currentKey []byte
	var uids []uint64
	pl := new(pb.PostingList)

	userMeta := []byte{posting.BitCompletePosting}
	writeVersionTs := r.state.writeTs

	countEntries := []*countIndexEntry{}
	var currentBatch []int

	appendToList := func() {
		if len(currentBatch) == 0 {
			return
		}

		// Calculate count entries.
		countEntries = append(countEntries, &countIndexEntry{
			key:   y.Copy(currentKey),
			count: len(currentBatch),
		})

		// Now make a list and write it to badger.
		sort.Slice(currentBatch, func(i, j int) bool {
			lhs := MapEntry(cbuf.Slice(currentBatch[i]))
			rhs := MapEntry(cbuf.Slice(currentBatch[j]))
			return less(lhs, rhs)
		})

		for _, offset := range currentBatch {
			me := MapEntry(cbuf.Slice(offset))
			uid := me.Uid()
			if len(uids) > 0 && uids[len(uids)-1] == uid {
				continue
			}
			// TODO: Potentially could be doing codec.Encoding right here.
			uids = append(uids, uid)
			if pbuf := me.Plist(); len(pbuf) > 0 {
				p := &pb.Posting{}
				x.Check(p.Unmarshal(pbuf))
				pl.Postings = append(pl.Postings, p)
			}
		}

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
		defer codec.FreePack(pl.Pack)

		shouldSplit := pl.Size() > (1<<20)/2 && len(pl.Pack.Blocks) > 1
		if shouldSplit {
			l := posting.NewList(y.Copy(currentKey), pl, writeVersionTs)
			kvs, err := l.Rollup()
			x.Check(err)
			list.Kv = append(list.Kv, kvs[0])
			splitList.Kv = append(splitList.Kv, kvs[1:]...)
		} else {
			val, err := pl.Marshal()
			x.Check(err)
			kv := &bpb.KV{
				Key:      y.Copy(currentKey),
				Value:    val,
				UserMeta: userMeta,
				Version:  writeVersionTs,
			}
			list.Kv = append(list.Kv, kv)
		}

		uids = uids[:0]
		pl.Reset()
		// Reset the current batch.
		currentBatch = currentBatch[:0]
	}

	for _, offset := range req.offsets {
		atomic.AddInt64(&r.prog.reduceEdgeCount, 1)
		entry := MapEntry(cbuf.Slice(offset))
		entryKey := entry.Key()

		if !bytes.Equal(entryKey, currentKey) && currentKey != nil {
			appendToList()
		}

		currentKey = append(currentKey[:0], entryKey...)
		currentBatch = append(currentBatch, offset)
	}

	appendToList()
	return countEntries
}
