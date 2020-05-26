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
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v2"
	bo "github.com/dgraph-io/badger/v2/options"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type reducer struct {
	*state
	streamId  uint32
	mu        sync.RWMutex
	streamIds map[string]uint32
}

const batchSize = 10000
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
		go func(shardId int, db *badger.DB) {
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

			ci := &countIndexer{reducer: r, writer: writer}
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
	if r.opt.EncryptionKey != nil {
		// Need to set zero addr in WorkerConfig before checking the license.
		x.WorkerConfig.ZeroAddr = []string{r.opt.ZeroAddr}

		if !worker.EnterpriseEnabled() {
			// Crash since the enterprise license is not enabled..
			log.Fatal("Enterprise License needed for the Encryption feature.")
		} else {
			log.Printf("Encryption feature enabled.")
		}
	}

	opt := badger.DefaultOptions(r.opt.shardOutputDirs[i]).WithSyncWrites(false).
		WithTableLoadingMode(bo.MemoryMap).WithValueThreshold(1 << 10 /* 1 KB */).
		WithLogger(nil).WithMaxCacheSize(1 << 20).
		WithEncryptionKey(r.opt.EncryptionKey).WithCompression(bo.None)

	// Overwrite badger options based on the options provided by the user.
	r.setBadgerOptions(&opt)

	db, err := badger.OpenManaged(opt)
	x.Check(err)

	// Zero out the key from memory.
	opt.EncryptionKey = nil

	r.dbs = append(r.dbs, db)
	return db
}

func (r *reducer) setBadgerOptions(opt *badger.Options) {
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
	batch        [][]byte
}

func (mi *mapIterator) release(ie *iteratorEntry) {
	ie.batch = ie.batch[:0]
	select {
	case mi.freelist <- ie:
	default:
	}
}

func (mi *mapIterator) startBatching(partitionsKeys [][]byte) {
	var ie *iteratorEntry
	prevKeyExist := false
	var buf, eBuf, key []byte
	var err error

	// readKey reads the next map entry key.
	readMapEntry := func() error {
		if prevKeyExist {
			return nil
		}
		r := mi.reader
		buf, err = r.Peek(binary.MaxVarintLen64)
		if err != nil {
			return err
		}
		sz, n := binary.Uvarint(buf)
		if n <= 0 {
			log.Fatalf("Could not read uvarint: %d", n)
		}
		x.Check2(r.Discard(n))

		eBuf = make([]byte, sz)
		x.Check2(io.ReadFull(r, eBuf))

		key, err = GetKeyForMapEntry(eBuf)
		return err
	}

	for _, pKey := range partitionsKeys {
		select {
		case ie = <-mi.freelist:
		default:
			ie = &iteratorEntry{
				batch: make([][]byte, 0, batchAlloc),
			}
		}
		ie.partitionKey = pKey
		for {
			err := readMapEntry()
			if err == io.EOF {
				break
			}
			x.Check(err)
			if bytes.Compare(key, ie.partitionKey) < 0 {
				prevKeyExist = false
				ie.batch = append(ie.batch, eBuf)
				continue
			}

			// Current key is not part of this batch so track that we have already read the key.
			prevKeyExist = true
			break
		}
		mi.batchCh <- ie
	}

	// Drain the last items.
	batch := make([][]byte, 0, batchAlloc)
	for {
		err := readMapEntry()
		if err == io.EOF {
			break
		}
		x.Check(err)
		prevKeyExist = false
		batch = append(batch, eBuf)
	}
	mi.batchCh <- &iteratorEntry{
		batch:        batch,
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
		freelist: make(chan *iteratorEntry, 3),
	}
	return header, itr
}

type countIndexEntry struct {
	key   []byte
	count int
}

type encodeRequest struct {
	entries   [][]byte
	countKeys []*countIndexEntry
	wg        *sync.WaitGroup
	list      *bpb.KVList
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
	for req := range entryCh {

		req.list = &bpb.KVList{}
		countKeys := r.toList(req.entries, req.list)
		for _, kv := range req.list.Kv {
			pk, err := x.Parse(kv.Key)
			x.Check(err)
			x.AssertTrue(len(pk.Attr) > 0)
			kv.StreamId = r.streamIdFor(pk.Attr)
			if pk.HasStartUid {
				// If the key is for a split key, it cannot go into the same stream
				// due to ordering issues. Instead, the stream ID for this keys is
				// derived by flipping the MSB of the original stream ID.
				kv.StreamId |= 0x80000000
			}
		}
		req.countKeys = countKeys
		req.wg.Done()
		atomic.AddInt32(&r.prog.numEncoding, -1)
	}
}

func (r *reducer) startWriting(ci *countIndexer, writerCh chan *encodeRequest, closer *y.Closer) {
	defer closer.Done()
	for req := range writerCh {
		req.wg.Wait()

		for _, countKey := range req.countKeys {
			ci.addUid(countKey.key, countKey.count)
		}
		// Wait for it to be encoded.
		start := time.Now()

		x.Check(ci.writer.Write(req.list))
		if dur := time.Since(start).Round(time.Millisecond); dur > time.Second {
			fmt.Printf("writeCh: Time taken to write req: %v\n",
				time.Since(start).Round(time.Millisecond))
		}
	}
}

func (r *reducer) reduce(partitionKeys [][]byte, mapItrs []*mapIterator, ci *countIndexer) {
	cpu := runtime.NumCPU()
	fmt.Printf("Num CPUs: %d\n", cpu)
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

	for i := 0; i < len(partitionKeys); i++ {
		batch := make([][]byte, 0, batchAlloc)
		for _, itr := range mapItrs {
			res := itr.Next()
			y.AssertTrue(bytes.Equal(res.partitionKey, partitionKeys[i]))
			batch = append(batch, res.batch...)
			itr.release(res)
		}
		wg := new(sync.WaitGroup)
		wg.Add(1)
		req := &encodeRequest{entries: batch, wg: wg}
		atomic.AddInt32(&r.prog.numEncoding, 1)
		encoderCh <- req
		writerCh <- req
	}

	// Drain the last batch
	batch := make([][]byte, 0, batchAlloc)
	for _, itr := range mapItrs {
		res := itr.Next()
		y.AssertTrue(res.partitionKey == nil)
		batch = append(batch, res.batch...)
	}
	wg := new(sync.WaitGroup)
	wg.Add(1)
	req := &encodeRequest{entries: batch, wg: wg}
	atomic.AddInt32(&r.prog.numEncoding, 1)
	encoderCh <- req
	writerCh <- req

	// Close the encodes.
	close(encoderCh)
	encoderCloser.SignalAndWait()

	// Close the writer.
	close(writerCh)
	writerCloser.SignalAndWait()
}

func (r *reducer) toList(bufEntries [][]byte, list *bpb.KVList) []*countIndexEntry {
	sort.Slice(bufEntries, func(i, j int) bool {
		lh, err := GetKeyForMapEntry(bufEntries[i])
		x.Check(err)
		rh, err := GetKeyForMapEntry(bufEntries[j])
		x.Check(err)
		return bytes.Compare(lh, rh) < 0
	})

	var currentKey []byte
	var uids []uint64
	pl := new(pb.PostingList)

	userMeta := []byte{posting.BitCompletePosting}
	writeVersionTs := r.state.writeTs

	countEntries := []*countIndexEntry{}
	currentBatch := make([]*pb.MapEntry, 0, 100)
	freelist := make([]*pb.MapEntry, 0)

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
			return less(currentBatch[i], currentBatch[j])
		})
		for _, mapEntry := range currentBatch {
			uid := mapEntry.Uid
			if mapEntry.Posting != nil {
				uid = mapEntry.Posting.Uid
			}
			if len(uids) > 0 && uids[len(uids)-1] == uid {
				continue
			}
			// TODO: Potentially could be doing codec.Encoding right here.
			uids = append(uids, uid)
			if mapEntry.Posting != nil {
				pl.Postings = append(pl.Postings, mapEntry.Posting)
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
		shouldSplit := pl.Size() > (1<<20)/2 && len(pl.Pack.Blocks) > 1
		if shouldSplit {
			l := posting.NewList(y.Copy(currentKey), pl, writeVersionTs)
			kvs, err := l.Rollup()
			x.Check(err)
			list.Kv = append(list.Kv, kvs...)
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
		// Now we have written the list. It's time to reuse the current batch.
		freelist = append(freelist, currentBatch...)
		// Reset the current batch.
		currentBatch = currentBatch[:0]
	}

	for _, entry := range bufEntries {
		atomic.AddInt64(&r.prog.reduceEdgeCount, 1)
		entryKey, err := GetKeyForMapEntry(entry)
		x.Check(err)
		if !bytes.Equal(entryKey, currentKey) && currentKey != nil {
			appendToList()
		}
		currentKey = append(currentKey[:0], entryKey...)
		var mapEntry *pb.MapEntry
		if len(freelist) == 0 {
			// Create a new map entry.
			mapEntry = &pb.MapEntry{}
		} else {
			// Obtain from freelist.
			mapEntry = freelist[0]
			mapEntry.Reset()
			freelist = freelist[1:]
		}
		x.Check(mapEntry.Unmarshal(entry))
		currentBatch = append(currentBatch, mapEntry)
	}

	appendToList()
	return countEntries
}
