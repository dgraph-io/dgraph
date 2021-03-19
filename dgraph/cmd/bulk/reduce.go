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
	"context"
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

	"github.com/dgraph-io/badger/v3"
	bo "github.com/dgraph-io/badger/v3/options"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
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

			// Dedup the partition keys.
			partitions := make(map[string]struct{})
			for _, mapFile := range mapFiles {
				header, itr := newMapIterator(mapFile)
				for _, k := range header.PartitionKeys {
					if len(k) == 0 {
						continue
					}
					partitions[string(k)] = struct{}{}
				}
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
				splitCh:     make(chan *bpb.KVList, 2*runtime.NumCPU()),
				countBuf:    getBuf(r.opt.TmpDir),
			}

			partitionKeys := make([][]byte, 0, len(partitions))
			for k := range partitions {
				partitionKeys = append(partitionKeys, []byte(k))
			}
			sort.Slice(partitionKeys, func(i, j int) bool {
				return bytes.Compare(partitionKeys[i], partitionKeys[j]) < 0
			})

			r.reduce(partitionKeys, mapItrs, ci)
			ci.wait()

			fmt.Println("Writing split lists back to the main DB now")
			// Write split lists back to the main DB.
			r.writeSplitLists(db, tmpDb, writer)

			x.Check(writer.Flush())

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
	key := r.opt.EncryptionKey
	if !r.opt.EncryptedOut {
		key = nil
	}

	opt := badger.DefaultOptions(dir).
		WithSyncWrites(false).
		WithEncryptionKey(key).
		WithBlockCacheSize(r.opt.BlockCacheSize).
		WithIndexCacheSize(r.opt.IndexCacheSize)

	opt.Compression = bo.None
	opt.ZSTDCompressionLevel = 0
	// Overwrite badger options based on the options provided by the user.
	if compression {
		opt.Compression = r.state.opt.BadgerCompression
		opt.ZSTDCompressionLevel = r.state.opt.BadgerCompressionLevel
	}

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

type mapIterator struct {
	fd     *os.File
	reader *bufio.Reader
	meBuf  []byte
}

func (mi *mapIterator) Next(cbuf *z.Buffer, partitionKey []byte) {
	readMapEntry := func() error {
		if len(mi.meBuf) > 0 {
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
		if cap(mi.meBuf) < int(sz) {
			mi.meBuf = make([]byte, int(sz))
		}
		mi.meBuf = mi.meBuf[:int(sz)]
		x.Check2(io.ReadFull(r, mi.meBuf))
		return nil
	}
	for {
		if err := readMapEntry(); err == io.EOF {
			break
		} else {
			x.Check(err)
		}
		key := MapEntry(mi.meBuf).Key()

		if len(partitionKey) == 0 || bytes.Compare(key, partitionKey) < 0 {
			b := cbuf.SliceAllocate(len(mi.meBuf))
			copy(b, mi.meBuf)
			mi.meBuf = mi.meBuf[:0]
			// map entry is already part of cBuf.
			continue
		}
		// Current key is not part of this batch so track that we have already read the key.
		return
	}
}

func (mi *mapIterator) Close() error {
	return mi.fd.Close()
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
		fd:     fd,
		reader: reader,
	}
	return header, itr
}

type encodeRequest struct {
	cbuf     *z.Buffer
	countBuf *z.Buffer
	wg       *sync.WaitGroup
	listCh   chan *z.Buffer
	splitCh  chan *bpb.KVList
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

func (r *reducer) encode(entryCh chan *encodeRequest, closer *z.Closer) {
	defer closer.Done()

	for req := range entryCh {
		r.toList(req)
		req.wg.Done()
	}
}

const maxSplitBatchLen = 1000

func (r *reducer) writeTmpSplits(ci *countIndexer, wg *sync.WaitGroup) {
	defer wg.Done()
	splitBatchLen := 0

	for kvs := range ci.splitCh {
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
			x.Check(ci.splitWriter.WriteList(batch))
		}
	}
	x.Check(ci.splitWriter.Flush())
}

func (r *reducer) startWriting(ci *countIndexer, writerCh chan *encodeRequest, closer *z.Closer) {
	defer closer.Done()

	// Concurrently write split lists to a temporary badger.
	tmpWg := new(sync.WaitGroup)
	tmpWg.Add(1)
	go r.writeTmpSplits(ci, tmpWg)

	count := func(req *encodeRequest) {
		defer req.countBuf.Release()
		if req.countBuf.IsEmpty() {
			return
		}

		// req.countBuf is already sorted.
		sz := req.countBuf.LenNoPadding()
		ci.countBuf.Grow(sz)

		req.countBuf.SliceIterate(func(slice []byte) error {
			ce := countEntry(slice)
			ci.addCountEntry(ce)
			return nil
		})
	}

	var lastStreamId uint32
	write := func(req *encodeRequest) {
		for kvBuf := range req.listCh {
			x.Check(ci.writer.Write(kvBuf))

			kv := &bpb.KV{}
			err := kvBuf.SliceIterate(func(s []byte) error {
				kv.Reset()
				x.Check(kv.Unmarshal(s))
				if lastStreamId == kv.StreamId {
					return nil
				}
				if lastStreamId > 0 {
					fmt.Printf("Finishing stream id: %d\n", lastStreamId)
					doneKV := &bpb.KV{
						StreamId:   lastStreamId,
						StreamDone: true,
					}

					buf := z.NewBuffer(512)
					defer buf.Release()
					badger.KVToBuffer(doneKV, buf)

					ci.writer.Write(buf)
				}
				lastStreamId = kv.StreamId
				return nil

			})
			x.Check(err)
			kvBuf.Release()
		}
	}

	for req := range writerCh {
		write(req)
		req.wg.Wait()

		count(req)
	}

	// Wait for split lists to be written to the temporary badger.
	close(ci.splitCh)
	tmpWg.Wait()
}

func (r *reducer) writeSplitLists(db, tmpDb *badger.DB, writer *badger.StreamWriter) {
	// baseStreamId is the max ID seen while writing non-split lists.
	baseStreamId := atomic.AddUint32(&r.streamId, 1)
	stream := tmpDb.NewStreamAt(math.MaxUint64)
	stream.LogPrefix = "copying split keys to main DB"
	stream.Send = func(buf *z.Buffer) error {
		kvs, err := badger.BufferToKVList(buf)
		x.Check(err)

		buf.Reset()
		for _, kv := range kvs.Kv {
			kv.StreamId += baseStreamId
			badger.KVToBuffer(kv, buf)
		}
		x.Check(writer.Write(buf))
		return nil
	}
	x.Check(stream.Orchestrate(context.Background()))
}

const limit = 2 << 30

func (r *reducer) throttle() {
	for {
		sz := atomic.LoadInt64(&r.prog.numEncoding)
		if sz < limit {
			return
		}
		time.Sleep(time.Second)
	}
}

func bufferStats(cbuf *z.Buffer) {
	fmt.Printf("Found a buffer of size: %s\n", humanize.IBytes(uint64(cbuf.LenNoPadding())))

	// Just check how many keys do we have in this giant buffer.
	keys := make(map[uint64]int64)
	var numEntries int
	cbuf.SliceIterate(func(slice []byte) error {
		me := MapEntry(slice)
		keys[z.MemHash(me.Key())]++
		numEntries++
		return nil
	})
	keyHist := z.NewHistogramData(z.HistogramBounds(10, 32))
	for _, num := range keys {
		keyHist.Update(num)
	}
	fmt.Printf("Num Entries: %d. Total keys: %d\n Histogram: %s\n",
		numEntries, len(keys), keyHist.String())
}

func getBuf(dir string) *z.Buffer {
	cbuf, err := z.NewBufferWithDir(64<<20, 64<<30, z.UseCalloc, filepath.Join(dir, bufferDir))
	x.Check(err)
	cbuf.AutoMmapAfter(1 << 30)
	return cbuf
}

func (r *reducer) reduce(partitionKeys [][]byte, mapItrs []*mapIterator, ci *countIndexer) {
	cpu := r.opt.NumGoroutines
	fmt.Printf("Num Encoders: %d\n", cpu)
	encoderCh := make(chan *encodeRequest, 2*cpu)
	writerCh := make(chan *encodeRequest, 2*cpu)
	encoderCloser := z.NewCloser(cpu)
	for i := 0; i < cpu; i++ {
		// Start listening to encode entries
		// For time being let's lease 100 stream id for each encoder.
		go r.encode(encoderCh, encoderCloser)
	}
	// Start listening to write the badger list.
	writerCloser := z.NewCloser(1)
	go r.startWriting(ci, writerCh, writerCloser)

	sendReq := func(zbuf *z.Buffer) {
		wg := new(sync.WaitGroup)
		wg.Add(1)
		req := &encodeRequest{
			cbuf:     zbuf,
			wg:       wg,
			listCh:   make(chan *z.Buffer, 3),
			splitCh:  ci.splitCh,
			countBuf: getBuf(r.opt.TmpDir),
		}
		encoderCh <- req
		writerCh <- req
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	buffers := make(chan *z.Buffer, 3)

	go func() {
		// Start collecting buffers.
		hd := z.NewHistogramData(z.HistogramBounds(16, 40))
		cbuf := getBuf(r.opt.TmpDir)
		// Append nil for the last entries.
		partitionKeys = append(partitionKeys, nil)

		for i := 0; i < len(partitionKeys); i++ {
			for _, itr := range mapItrs {
				pkey := partitionKeys[i]
				itr.Next(cbuf, pkey)
			}
			if cbuf.LenNoPadding() < 256<<20 {
				// Pick up more data.
				continue
			}

			hd.Update(int64(cbuf.LenNoPadding()))
			select {
			case <-ticker.C:
				fmt.Printf("Histogram of buffer sizes: %s\n", hd.String())
			default:
			}

			buffers <- cbuf
			cbuf = getBuf(r.opt.TmpDir)
		}
		if !cbuf.IsEmpty() {
			hd.Update(int64(cbuf.LenNoPadding()))
			buffers <- cbuf
		} else {
			cbuf.Release()
		}
		fmt.Printf("Final Histogram of buffer sizes: %s\n", hd.String())
		close(buffers)
	}()

	for cbuf := range buffers {
		if cbuf.LenNoPadding() > limit/2 {
			bufferStats(cbuf)
		}
		r.throttle()

		atomic.AddInt64(&r.prog.numEncoding, int64(cbuf.LenNoPadding()))
		sendReq(cbuf)
	}

	// Close the encodes.
	close(encoderCh)
	encoderCloser.SignalAndWait()

	// Close the writer.
	close(writerCh)
	writerCloser.SignalAndWait()
}

func (r *reducer) toList(req *encodeRequest) {
	cbuf := req.cbuf
	defer func() {
		atomic.AddInt64(&r.prog.numEncoding, -int64(cbuf.LenNoPadding()))
		cbuf.Release()
	}()

	cbuf.SortSlice(func(ls, rs []byte) bool {
		lhs := MapEntry(ls)
		rhs := MapEntry(rs)
		return less(lhs, rhs)
	})

	var currentKey []byte
	pl := new(pb.PostingList)
	writeVersionTs := r.state.writeTs

	kvBuf := z.NewBuffer(260 << 20)
	trackCountIndex := make(map[string]bool)

	var freePostings []*pb.Posting

	getPosting := func() *pb.Posting {
		if sz := len(freePostings); sz > 0 {
			last := freePostings[sz-1]
			freePostings = freePostings[:sz-1]
			return last
		}
		return &pb.Posting{}
	}

	freePosting := func(p *pb.Posting) {
		p.Reset()
		freePostings = append(freePostings, p)
	}

	start, end, num := cbuf.StartOffset(), cbuf.StartOffset(), 0
	appendToList := func() {
		if num == 0 {
			return
		}
		atomic.AddInt64(&r.prog.reduceEdgeCount, int64(num))

		pk, err := x.Parse(currentKey)
		x.Check(err)
		x.AssertTrue(len(pk.Attr) > 0)

		// We might not need to track count index every time.
		if pk.IsData() || pk.IsReverse() {
			doCount, ok := trackCountIndex[pk.Attr]
			if !ok {
				doCount = r.schema.getSchema(pk.Attr).GetCount()
				trackCountIndex[pk.Attr] = doCount
			}
			if doCount {
				// Calculate count entries.
				ck := x.CountKey(pk.Attr, uint32(num), pk.IsReverse())
				dst := req.countBuf.SliceAllocate(countEntrySize(ck))
				marshalCountEntry(dst, ck, pk.Uid)
			}
		}

		enc := codec.Encoder{BlockSize: 256}
		var lastUid uint64
		slice, next := []byte{}, start
		for next >= 0 && (next < end || end == -1) {
			slice, next = cbuf.Slice(next)
			me := MapEntry(slice)

			uid := me.Uid()
			if uid == lastUid {
				continue
			}
			lastUid = uid

			enc.Add(uid)
			if pbuf := me.Plist(); len(pbuf) > 0 {
				p := getPosting()
				x.Check(p.Unmarshal(pbuf))
				pl.Postings = append(pl.Postings, p)
			}
		}

		// We should not do defer FreePack here, because we might be giving ownership of it away if
		// we run Rollup.
		pl.Pack = enc.Done()
		numUids := codec.ExactLen(pl.Pack)

		atomic.AddInt64(&r.prog.reduceKeyCount, 1)

		// For a UID-only posting list, the badger value is a delta packed UID
		// list. The UserMeta indicates to treat the value as a delta packed
		// list when the value is read by dgraph.  For a value posting list,
		// the full pb.Posting type is used (which pb.y contains the
		// delta packed UID list).
		if numUids == 0 {
			codec.FreePack(pl.Pack)
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
			if schema.GetValueType() == pb.Posting_UID && !schema.GetList() && numUids > 1 {
				fmt.Printf("Schema for pred %s specifies that this is not a list but more than  "+
					"one UID has been found. Forcing the schema to be a list to avoid any "+
					"data loss. Please fix the data to your specifications once Dgraph is up.\n",
					parsedKey.Attr)
				r.state.schema.setSchemaAsList(parsedKey.Attr)
			}
		}

		shouldSplit := pl.Size() > (1<<20)/2 && len(pl.Pack.Blocks) > 1
		if shouldSplit {
			// Give ownership of pl.Pack away to list. Rollup would deallocate the Pack.
			l := posting.NewList(y.Copy(currentKey), pl, writeVersionTs)
			kvs, err := l.Rollup(nil)
			x.Check(err)

			for _, kv := range kvs {
				kv.StreamId = r.streamIdFor(pk.Attr)
			}
			badger.KVToBuffer(kvs[0], kvBuf)
			if splits := kvs[1:]; len(splits) > 0 {
				req.splitCh <- &bpb.KVList{Kv: splits}
			}
		} else {
			kv := posting.MarshalPostingList(pl, nil)
			codec.FreePack(pl.Pack)

			kv.Key = y.Copy(currentKey)
			kv.Version = writeVersionTs
			kv.StreamId = r.streamIdFor(pk.Attr)
			badger.KVToBuffer(kv, kvBuf)
		}

		for _, p := range pl.Postings {
			freePosting(p)
		}
		pl.Reset()
	}

	for end >= 0 {
		slice, next := cbuf.Slice(end)
		entry := MapEntry(slice)
		entryKey := entry.Key()

		if !bytes.Equal(entryKey, currentKey) && currentKey != nil {
			appendToList()
			start, num = end, 0 // Start would start from current one.

			if kvBuf.LenNoPadding() > 256<<20 {
				req.listCh <- kvBuf
				kvBuf = z.NewBuffer(260 << 20)
			}
		}
		end = next
		currentKey = append(currentKey[:0], entryKey...)
		num++
	}

	appendToList()
	if kvBuf.LenNoPadding() > 0 {
		req.listCh <- kvBuf
	} else {
		kvBuf.Release()
	}
	close(req.listCh)

	// Sort countBuf before returning to better use the goroutines.
	req.countBuf.SortSlice(func(ls, rs []byte) bool {
		left := countEntry(ls)
		right := countEntry(rs)
		return left.less(right)
	})
}
