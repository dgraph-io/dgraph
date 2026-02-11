/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package bulk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"github.com/klauspost/compress/s2"
	"google.golang.org/protobuf/proto"

	"github.com/dgraph-io/badger/v4"
	bo "github.com/dgraph-io/badger/v4/options"
	bpb "github.com/dgraph-io/badger/v4/pb"
	"github.com/dgraph-io/badger/v4/y"
	"github.com/dgraph-io/dgraph/v25/codec"
	"github.com/dgraph-io/dgraph/v25/posting"
	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/schema"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/types"
	"github.com/dgraph-io/dgraph/v25/x"
	"github.com/dgraph-io/ristretto/v2/z"
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

	// Check if we have vector indexes - if so, create SHARED vectorTmpDb
	// This avoids the global pstore race condition by calling posting.Init() ONCE
	vectorIndexSpecs := r.schema.getVectorIndexSpecs()
	var sharedVectorDb *badger.DB
	var predToOutputShard map[string]int
	var predToShardMu sync.Mutex

	if len(vectorIndexSpecs) > 0 {
		fmt.Printf("Creating shared vector database for %d vector predicate(s)\n", len(vectorIndexSpecs))

		// Create single shared vectorTmpDb
		sharedVectorDb = r.createVectorTmpBadger()

		// Initialize posting and schema ONCE (avoids race condition!)
		posting.Init(sharedVectorDb, 0, false)
		schema.Init(sharedVectorDb)
		for pred, sch := range r.schema.schemaMap {
			schema.State().Set(pred, sch)
		}

		// Track which predicates belong to which output shard
		predToOutputShard = make(map[string]int)
	}

	thr := y.NewThrottle(r.opt.NumReducers)
	for i := range r.opt.ReduceShards {
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

			// Create vector indexer using shared DB (if vectors exist)
			var vi *vectorIndexer
			if sharedVectorDb != nil && len(vectorIndexSpecs) > 0 {
				fmt.Printf("Initializing vector indexer for shard %d with %d predicate(s)\n",
					shardId, len(vectorIndexSpecs))
				vi = newVectorIndexerShared(r, sharedVectorDb, vectorIndexSpecs,
					shardId, &predToShardMu, predToOutputShard)
			}

			partitionKeys := make([][]byte, 0, len(partitions))
			for k := range partitions {
				partitionKeys = append(partitionKeys, []byte(k))
			}
			sort.Slice(partitionKeys, func(i, j int) bool {
				return bytes.Compare(partitionKeys[i], partitionKeys[j]) < 0
			})

			r.reduce(partitionKeys, mapItrs, ci, vi)
			ci.wait()
			if vi != nil {
				vi.wait()
			}

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
	if err := thr.Finish(); err != nil {
		return err
	}

	// After all shards complete, copy vector data to correct output DBs
	if sharedVectorDb != nil && len(predToOutputShard) > 0 {
		fmt.Println("Copying vector data to output shards...")
		r.copyVectorDataToShards(sharedVectorDb, predToOutputShard)
	}

	return nil
}

func (r *reducer) createBadgerInternal(dir string, compression bool) *badger.DB {
	key := r.opt.EncryptionKey
	if !r.opt.EncryptedOut {
		key = nil
	}

	opt := r.state.opt.Badger.
		WithDir(dir).WithValueDir(dir).
		WithSyncWrites(false).
		WithEncryptionKey(key)

	opt.Compression = bo.None
	opt.ZSTDCompressionLevel = 0
	// Overwrite badger options based on the options provided by the user.
	if compression {
		opt.Compression = r.state.opt.Badger.Compression
		opt.ZSTDCompressionLevel = r.state.opt.Badger.ZSTDCompressionLevel
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
	tmpDir, err := os.MkdirTemp(r.opt.TmpDir, "split")
	x.Check(err)
	// Do not enable compression in temporary badger to improve performance.
	db := r.createBadgerInternal(tmpDir, false)
	r.tmpDbs = append(r.tmpDbs, db)
	return db
}

func (r *reducer) createVectorTmpBadger() *badger.DB {
	tmpDir, err := os.MkdirTemp(r.opt.TmpDir, "vector")
	x.Check(err)
	db := r.createBadgerInternal(tmpDir, false)
	r.vectorTmpDb = db
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
	r := s2.NewReader(fd)

	// Read the header size.
	reader := bufio.NewReaderSize(r, 16<<10)
	headerLenBuf := make([]byte, 4)
	x.Check2(io.ReadFull(reader, headerLenBuf))
	headerLen := binary.BigEndian.Uint32(headerLenBuf)
	// Reader the map header.
	headerBuf := make([]byte, headerLen)

	x.Check2(io.ReadFull(reader, headerBuf))
	header := &pb.MapHeader{}
	err = proto.Unmarshal(headerBuf, header)
	x.Check(err)

	itr := &mapIterator{
		fd:     fd,
		reader: reader,
	}
	return header, itr
}

type encodeRequest struct {
	cbuf      *z.Buffer
	countBuf  *z.Buffer
	vectorBuf *z.Buffer      // Buffer for vector entries to be indexed
	vi        *vectorIndexer // Vector indexer for routing vector predicates to tmpDb
	wg        *sync.WaitGroup
	listCh    chan *z.Buffer
	splitCh   chan *bpb.KVList
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
			// flush the write batch when the max batch length is reached to prevent the
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

func (r *reducer) startWriting(ci *countIndexer, vi *vectorIndexer, writerCh chan *encodeRequest, closer *z.Closer) {
	defer closer.Done()

	// Concurrently write split lists to a temporary badger.
	tmpWg := new(sync.WaitGroup)
	tmpWg.Add(1)
	go r.writeTmpSplits(ci, tmpWg)

	count := func(req *encodeRequest) {
		defer func() {
			if err := req.countBuf.Release(); err != nil {
				glog.Warningf("error in releasing buffer: %v", err)
			}
		}()
		if req.countBuf.IsEmpty() {
			return
		}

		// req.countBuf is already sorted.
		sz := req.countBuf.LenNoPadding()
		ci.countBuf.Grow(sz)

		if err := req.countBuf.SliceIterate(func(slice []byte) error {
			ce := countEntry(slice)
			ci.addCountEntry(ce)
			return nil
		}); err != nil {
			glog.Errorf("error while iterating over buf: %v", err)
			x.Check(err)
		}
	}

	// Process vector entries and insert into HNSW index
	vector := func(req *encodeRequest) {
		if req.vectorBuf == nil {
			return
		}
		defer func() {
			if err := req.vectorBuf.Release(); err != nil {
				glog.Warningf("error releasing vector buffer: %v", err)
			}
		}()
		if req.vectorBuf.IsEmpty() {
			return
		}

		// Iterate through vector entries and insert into HNSW
		if err := req.vectorBuf.SliceIterate(func(slice []byte) error {
			ve := unmarshalVectorEntry(slice)
			if ve == nil {
				// Skip malformed entries (already logged in unmarshalVectorEntry)
				return nil
			}
			// Insert vector into HNSW and generate entries
			vi.addVectorEntry(ve)
			return nil
		}); err != nil {
			glog.Errorf("error processing vectors: %v", err)
			x.Check(err)
		}
	}

	var lastStreamId uint32
	write := func(req *encodeRequest) {
		for kvBuf := range req.listCh {
			x.Check(ci.writer.Write(kvBuf))

			kv := &bpb.KV{}
			err := kvBuf.SliceIterate(func(s []byte) error {
				kv.Reset()
				x.Check(proto.Unmarshal(s, kv))
				if lastStreamId == kv.StreamId {
					return nil
				}
				if lastStreamId > 0 {
					fmt.Printf("Finishing stream id: %d\n", lastStreamId)
					doneKV := &bpb.KV{
						StreamId:   lastStreamId,
						StreamDone: true,
					}

					buf := z.NewBuffer(512, "Reducer.Write")
					defer func() {
						if err := buf.Release(); err != nil {
							glog.Warningf("error in releasing buffer: %v", err)
						}
					}()
					badger.KVToBuffer(doneKV, buf)

					if err := ci.writer.Write(buf); err != nil {
						glog.Warningf("error in releasing buffer: %v", err)
					}
				}
				lastStreamId = kv.StreamId
				return nil

			})
			x.Check(err)
			if err := kvBuf.Release(); err != nil {
				glog.Warningf("error in releasing buffer: %v", err)
			}
		}
	}

	for req := range writerCh {
		write(req)
		req.wg.Wait()

		count(req)
		if vi != nil {
			vector(req)
		}
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

// copyVectorDataToShards copies vector data from the shared vectorTmpDb to the correct output DBs.
// It uses the predToOutputShard map to determine which predicate goes to which shard.
func (r *reducer) copyVectorDataToShards(vectorDb *badger.DB, predToShard map[string]int) {
	if len(predToShard) == 0 {
		return
	}

	// Group predicates by output shard
	shardPreds := make(map[int][]string)
	for pred, shardId := range predToShard {
		shardPreds[shardId] = append(shardPreds[shardId], pred)
	}

	// Copy each shard's predicates to its output DB
	for shardId, preds := range shardPreds {
		if shardId >= len(r.dbs) {
			glog.Errorf("Invalid shard ID %d for predicates %v (only %d DBs)", shardId, preds, len(r.dbs))
			continue
		}

		destDb := r.dbs[shardId]

		// Collect all predicate keys (base + HNSW suffixes) and sort for efficient iteration
		var allPreds []string
		for _, pred := range preds {
			allPreds = append(allPreds, pred)
			allPreds = append(allPreds, hnsw.ConcatStrings(pred, hnsw.VecKeyword)) // __vector_
			allPreds = append(allPreds, hnsw.ConcatStrings(pred, hnsw.VecEntry))   // __vector_entry
			allPreds = append(allPreds, hnsw.ConcatStrings(pred, hnsw.VecDead))    // __vector_dead
		}
		sort.Strings(allPreds)

		fmt.Printf("Copying %d vector predicates to shard %d\n", len(preds), shardId)

		// Copy each predicate
		totalCount := 0
		for _, pred := range allPreds {
			count := r.copyPredicateWithBatchCount(destDb, vectorDb, pred)
			totalCount += count
		}

		fmt.Printf("Copied %d total KV entries for %d predicates to shard %d\n",
			totalCount, len(preds), shardId)
	}
}

// copyPredicateWithBatchCount is like copyPredicateWithBatch but returns the count of entries copied.
func (r *reducer) copyPredicateWithBatchCount(destDb *badger.DB, srcDb *badger.DB, pred string) int {
	wb := destDb.NewManagedWriteBatch()
	defer wb.Cancel()

	stream := srcDb.NewStreamAt(math.MaxUint64)
	stream.LogPrefix = fmt.Sprintf("copying predicate %s", pred)
	stream.Prefix = x.PredicatePrefix(pred)
	stream.NumGo = 1

	var count int
	stream.Send = func(buf *z.Buffer) error {
		kvs, err := badger.BufferToKVList(buf)
		if err != nil {
			return err
		}

		for _, kv := range kvs.Kv {
			userMeta := byte(0)
			if len(kv.UserMeta) > 0 {
				userMeta = kv.UserMeta[0]
			}

			entry := &badger.Entry{
				Key:      kv.Key,
				Value:    kv.Value,
				UserMeta: userMeta,
			}

			if err := wb.SetEntryAt(entry, kv.Version); err != nil {
				return fmt.Errorf("error writing to batch: %w", err)
			}
			count++
		}
		return nil
	}

	if err := stream.Orchestrate(context.Background()); err != nil {
		glog.Warningf("Error streaming predicate %s: %v", pred, err)
		return 0
	}

	if err := wb.Flush(); err != nil {
		glog.Errorf("Error flushing batch for predicate %s: %v", pred, err)
		return 0
	}

	return count
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
	if err := cbuf.SliceIterate(func(slice []byte) error {
		me := MapEntry(slice)
		keys[z.MemHash(me.Key())]++
		numEntries++
		return nil
	}); err != nil {
		glog.Errorf("error while iterating over buf: %v", err)
		x.Check(err)
	}

	keyHist := z.NewHistogramData(z.HistogramBounds(10, 32))
	for _, num := range keys {
		keyHist.Update(num)
	}
	fmt.Printf("Num Entries: %d. Total keys: %d\n Histogram: %s\n",
		numEntries, len(keys), keyHist.String())
}

func getBuf(dir string) *z.Buffer {
	return z.NewBuffer(64<<20, "Reducer.GetBuf").
		WithAutoMmap(1<<30, filepath.Join(dir, bufferDir)).
		WithMaxSize(0)
}

func (r *reducer) reduce(partitionKeys [][]byte, mapItrs []*mapIterator, ci *countIndexer, vi *vectorIndexer) {
	cpu := r.opt.NumGoroutines
	fmt.Printf("Num Encoders: %d\n", cpu)
	encoderCh := make(chan *encodeRequest, 2*cpu)
	writerCh := make(chan *encodeRequest, 2*cpu)
	encoderCloser := z.NewCloser(cpu)
	for range cpu {
		// Start listening to encode entries
		// For time being let's lease 100 stream id for each encoder.
		go r.encode(encoderCh, encoderCloser)
	}
	// Start listening to write the badger list.
	writerCloser := z.NewCloser(1)
	go r.startWriting(ci, vi, writerCh, writerCloser)

	sendReq := func(zbuf *z.Buffer) {
		wg := new(sync.WaitGroup)
		wg.Add(1)
		req := &encodeRequest{
			cbuf:     zbuf,
			wg:       wg,
			listCh:   make(chan *z.Buffer, 3),
			splitCh:  ci.splitCh,
			countBuf: getBuf(r.opt.TmpDir),
			vi:       vi,
		}
		// Only allocate vectorBuf when we have vector predicates to index
		if vi != nil {
			req.vectorBuf = getBuf(r.opt.TmpDir)
		}
		encoderCh <- req
		writerCh <- req
	}

	ticker := time.Tick(time.Minute)

	buffers := make(chan *z.Buffer, 3)

	go func() {
		// Start collecting buffers.
		hd := z.NewHistogramData(z.HistogramBounds(16, 40))
		cbuf := getBuf(r.opt.TmpDir)
		// Append nil for the last entries.
		partitionKeys = append(partitionKeys, nil)

		for i := range partitionKeys {
			pkey := partitionKeys[i]
			for _, itr := range mapItrs {
				itr.Next(cbuf, pkey)
			}
			if cbuf.LenNoPadding() < 256<<20 {
				// Pick up more data.
				continue
			}

			hd.Update(int64(cbuf.LenNoPadding()))
			select {
			case <-ticker:
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
			if err := cbuf.Release(); err != nil {
				glog.Warningf("error in releasing buffer: %v", err)
			}
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
		if err := cbuf.Release(); err != nil {
			glog.Warningf("error in releasing buffer: %v", err)
		}
	}()

	cbuf.SortSlice(func(ls, rs []byte) bool {
		lhs := MapEntry(ls)
		rhs := MapEntry(rs)
		return less(lhs, rhs)
	})

	var currentKey []byte
	pl := new(pb.PostingList)
	writeVersionTs := r.state.writeTs

	kvBuf := z.NewBuffer(260<<20, "Reducer.Buffer.ToList")
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

	alloc := z.NewAllocator(16<<20, "Reducer.ToList")
	defer func() {
		// We put alloc.Release in defer because we reassign alloc for split posting lists.
		alloc.Release()
	}()

	start, end, num := cbuf.StartOffset(), cbuf.StartOffset(), 0
	trackVectorIndex := make(map[string]bool) // Track predicates with vector indexes

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

		alloc.Reset()
		enc := codec.Encoder{BlockSize: 256, Alloc: alloc}
		var lastUid uint64
		var slice []byte
		next := start
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
				x.Check(proto.Unmarshal(pbuf, p))
				pl.Postings = append(pl.Postings, p)

				// Extract vectors for vector indexing
				if pk.IsData() && req.vectorBuf != nil {
					doVector, ok := trackVectorIndex[pk.Attr]
					if !ok {
						// Check if this predicate has a vector index
						doVector = r.schema.hasVectorIndex(pk.Attr)
						trackVectorIndex[pk.Attr] = doVector
					}
					if doVector {
						// Check if this is a vector value (vfloat type)
						if types.TypeID(p.ValType) == types.VFloatID && len(p.Value) > 0 {
							vector := types.BytesAsFloatArray(p.Value)
							if len(vector) > 0 {
								ve := &vectorEntry{
									pred:   pk.Attr,
									uid:    pk.Uid,
									vector: vector,
								}
								// Marshal to buffer
								dst := req.vectorBuf.SliceAllocate(vectorEntrySize(ve))
								marshalVectorEntry(dst, ve)
							}
						}
					}
				}
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
			// No need to FrePack here because we are reusing alloc.
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

		// Check if this is a vector predicate that should be routed to tmpDb
		isVectorPred := req.vi != nil && pk.IsData() && req.vi.isVectorPredicate(pk.Attr)

		shouldSplit := proto.Size(pl) > (1<<20)/2 && len(pl.Pack.Blocks) > 1
		if shouldSplit {
			// Give ownership of pl.Pack away to list. Rollup would deallocate the Pack.
			// We do rollup at math.MaxUint64 so that we don't change the allocated
			// timestamp of the posting list. The posting list originally is written
			// at writeVersionTs, we don't want to change that in rollup.
			l := posting.NewList(y.Copy(currentKey), pl, writeVersionTs)
			kvs, err := l.Rollup(nil, math.MaxUint64)
			x.Check(err)

			// Assign a new allocator, so we don't reset the one we were using during Rollup.
			alloc = z.NewAllocator(16<<20, "Reducer.AppendToList")

			if isVectorPred {
				// Vector predicates go to vectorTmpDb
				for _, kv := range kvs {
					kv.Version = writeVersionTs
					if err := req.vi.writeVectorKV(kv); err != nil {
						glog.Errorf("Error writing vector posting to tmpDb: %v", err)
					}
				}
			} else {
				// Non-vector predicates go to main DB via kvBuf
				for _, kv := range kvs {
					kv.StreamId = r.streamIdFor(pk.Attr)
				}
				badger.KVToBuffer(kvs[0], kvBuf)
				if splits := kvs[1:]; len(splits) > 0 {
					req.splitCh <- &bpb.KVList{Kv: splits}
				}
			}
		} else {
			kv := posting.MarshalPostingList(pl, nil)
			// No need to FreePack here, because we are reusing alloc.

			kv.Key = y.Copy(currentKey)
			kv.Version = writeVersionTs

			if isVectorPred {
				// Vector predicates go to vectorTmpDb
				if err := req.vi.writeVectorKV(kv); err != nil {
					glog.Errorf("Error writing vector posting to tmpDb: %v", err)
				}
			} else {
				// Non-vector predicates go to main DB via kvBuf
				kv.StreamId = r.streamIdFor(pk.Attr)
				badger.KVToBuffer(kv, kvBuf)
			}
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
				kvBuf = z.NewBuffer(260<<20, "Reducer.Buffer.KVBuffer")
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
		if err := kvBuf.Release(); err != nil {
			glog.Warningf("error in releasing buffer: %v", err)
		}
	}
	close(req.listCh)

	// Sort countBuf before returning to better use the goroutines.
	req.countBuf.SortSlice(func(ls, rs []byte) bool {
		left := countEntry(ls)
		right := countEntry(rs)
		return left.less(right)
	})
}
