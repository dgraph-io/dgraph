/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package badger

import (
	"bytes"
	"math"

	"github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/table"
	"github.com/dgraph-io/badger/y"
	humanize "github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

// StreamWriter is used to write data coming from multiple streams. The streams must not have any
// overlapping key ranges. Within each stream, the keys must be sorted. Badger Stream framework is
// capable of generating such an output. So, this StreamWriter can be used at the other end to build
// BadgerDB at a much faster pace by writing SSTables (and value logs) directly to LSM tree levels
// without causing any compactions at all. This is way faster than using batched writer or using
// transactions, but only applicable in situations where the keys are pre-sorted and the DB is being
// bootstrapped. Existing data would get deleted when using this writer. So, this is only useful
// when restoring from backup or replicating DB across servers.
//
// StreamWriter should not be called on in-use DB instances. It is designed only to bootstrap new
// DBs.
type StreamWriter struct {
	db         *DB
	done       func()
	throttle   *y.Throttle
	head       valuePointer
	maxVersion uint64
	writers    map[uint32]*sortedWriter
}

// NewStreamWriter creates a StreamWriter. Right after creating StreamWriter, Prepare must be
// called. The memory usage of a StreamWriter is directly proportional to the number of streams
// possible. So, efforts must be made to keep the number of streams low. Stream framework would
// typically use 16 goroutines and hence create 16 streams.
func (db *DB) NewStreamWriter() *StreamWriter {
	return &StreamWriter{
		db: db,
		// throttle shouldn't make much difference. Memory consumption is based on the number of
		// concurrent streams being processed.
		throttle: y.NewThrottle(16),
		writers:  make(map[uint32]*sortedWriter),
	}
}

// Prepare should be called before writing any entry to StreamWriter. It deletes all data present in
// existing DB, stops compactions and any writes being done by other means. Be very careful when
// calling Prepare, because it could result in permanent data loss. Not calling Prepare would result
// in a corrupt Badger instance.
func (sw *StreamWriter) Prepare() error {
	var err error
	sw.done, err = sw.db.dropAll()
	return err
}

// Write writes KVList to DB. Each KV within the list contains the stream id which StreamWriter
// would use to demux the writes.
func (sw *StreamWriter) Write(kvs *pb.KVList) error {
	var entries []*Entry
	for _, kv := range kvs.Kv {
		var meta, userMeta byte
		if len(kv.Meta) > 0 {
			meta = kv.Meta[0]
		}
		if len(kv.UserMeta) > 0 {
			userMeta = kv.UserMeta[0]
		}
		if sw.maxVersion < kv.Version {
			sw.maxVersion = kv.Version
		}
		e := &Entry{
			Key:       y.KeyWithTs(kv.Key, kv.Version),
			Value:     kv.Value,
			UserMeta:  userMeta,
			ExpiresAt: kv.ExpiresAt,
			meta:      meta,
		}
		// If the value can be colocated with the key in LSM tree, we can skip
		// writing the value to value log.
		e.skipVlog = sw.db.shouldWriteValueToLSM(*e)
		entries = append(entries, e)
	}
	req := &request{
		Entries: entries,
	}
	y.AssertTrue(len(kvs.Kv) == len(req.Entries))
	if err := sw.db.vlog.write([]*request{req}); err != nil {
		return err
	}

	for i, kv := range kvs.Kv {
		e := req.Entries[i]
		vptr := req.Ptrs[i]
		if !vptr.IsZero() {
			y.AssertTrue(sw.head.Less(vptr))
			sw.head = vptr
		}

		writer, ok := sw.writers[kv.StreamId]
		if !ok {
			writer = sw.newWriter(kv.StreamId)
			sw.writers[kv.StreamId] = writer
		}

		var vs y.ValueStruct
		if e.skipVlog {
			vs = y.ValueStruct{
				Value:     e.Value,
				Meta:      e.meta,
				UserMeta:  e.UserMeta,
				ExpiresAt: e.ExpiresAt,
			}
		} else {
			vbuf := make([]byte, vptrSize)
			vs = y.ValueStruct{
				Value:     vptr.Encode(vbuf),
				Meta:      e.meta | bitValuePointer,
				UserMeta:  e.UserMeta,
				ExpiresAt: e.ExpiresAt,
			}
		}
		if err := writer.Add(e.Key, vs); err != nil {
			return err
		}
	}
	return nil
}

// Done is called once we are done writing all the entries. It syncs DB directories. It also
// updates Oracle with maxVersion found in all entries (if DB is not managed).
func (sw *StreamWriter) Done() error {
	defer sw.done()
	for _, writer := range sw.writers {
		if err := writer.Done(); err != nil {
			return err
		}
	}

	// Encode and write the value log head into a new table.
	data := make([]byte, vptrSize)
	sw.head.Encode(data)
	headWriter := sw.newWriter(math.MaxUint32)
	if err := headWriter.Add(
		y.KeyWithTs(head, sw.maxVersion),
		y.ValueStruct{Value: data}); err != nil {
		return err
	}
	if err := headWriter.Done(); err != nil {
		return err
	}

	if !sw.db.opt.managedTxns {
		sw.db.orc = newOracle(sw.db.opt)
		sw.db.orc.nextTxnTs = sw.maxVersion
		sw.db.orc.txnMark.Done(sw.maxVersion)
		sw.db.orc.readMark.Done(sw.maxVersion)
		sw.db.orc.incrementNextTs()
	}

	// Wait for all files to be written.
	if err := sw.throttle.Finish(); err != nil {
		return err
	}

	// Now sync the directories, so all the files are registered.
	if sw.db.opt.ValueDir != sw.db.opt.Dir {
		if err := syncDir(sw.db.opt.ValueDir); err != nil {
			return err
		}
	}
	return syncDir(sw.db.opt.Dir)
}

type sortedWriter struct {
	db       *DB
	throttle *y.Throttle

	builder  *table.Builder
	lastKey  []byte
	streamID uint32
}

func (sw *StreamWriter) newWriter(streamID uint32) *sortedWriter {
	return &sortedWriter{
		db:       sw.db,
		streamID: streamID,
		throttle: sw.throttle,
		builder:  table.NewTableBuilder(),
	}
}

// ErrUnsortedKey is returned when any out of order key arrives at sortedWriter during call to Add.
var ErrUnsortedKey = errors.New("Keys not in sorted order")

// Add adds key and vs to sortedWriter.
func (w *sortedWriter) Add(key []byte, vs y.ValueStruct) error {
	if bytes.Compare(key, w.lastKey) <= 0 {
		return ErrUnsortedKey
	}
	sameKey := y.SameKey(key, w.lastKey)
	w.lastKey = y.SafeCopy(w.lastKey, key)

	if err := w.builder.Add(key, vs); err != nil {
		return err
	}
	// Same keys should go into the same SSTable.
	if !sameKey && w.builder.ReachedCapacity(w.db.opt.MaxTableSize) {
		return w.send()
	}
	return nil
}

func (w *sortedWriter) send() error {
	if err := w.throttle.Do(); err != nil {
		return err
	}
	go func(builder *table.Builder) {
		data := builder.Finish()
		err := w.createTable(data)
		w.throttle.Done(err)
	}(w.builder)
	w.builder = table.NewTableBuilder()
	return nil
}

// Done is called once we are done writing all keys and valueStructs
// to sortedWriter. It completes writing current SST to disk.
func (w *sortedWriter) Done() error {
	if w.builder.Empty() {
		return nil
	}
	return w.send()
}

func (w *sortedWriter) createTable(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	fileID := w.db.lc.reserveFileID()
	fd, err := y.CreateSyncedFile(table.NewFilename(fileID, w.db.opt.Dir), true)
	if err != nil {
		return err
	}
	if _, err := fd.Write(data); err != nil {
		return err
	}
	tbl, err := table.OpenTable(fd, w.db.opt.TableLoadingMode, nil)
	if err != nil {
		return err
	}
	lc := w.db.lc

	var lhandler *levelHandler
	for _, l := range lc.levels[1:] {
		ratio := float64(l.getTotalSize()) / float64(l.maxTotalSize)
		if ratio < 1.0 {
			lhandler = l
			break
		}
	}
	if lhandler == nil {
		lhandler = lc.levels[len(lc.levels)-1]
	}
	// Now that table can be opened successfully, let's add this to the MANIFEST.
	change := &pb.ManifestChange{
		Id:       tbl.ID(),
		Op:       pb.ManifestChange_CREATE,
		Level:    uint32(lhandler.level),
		Checksum: tbl.Checksum,
	}
	if err := w.db.manifest.addChanges([]*pb.ManifestChange{change}); err != nil {
		return err
	}
	if err := lhandler.replaceTables([]*table.Table{}, []*table.Table{tbl}); err != nil {
		return err
	}
	w.db.opt.Infof("Table created: %d at level: %d for stream: %d. Size: %s\n",
		fileID, lhandler.level, w.streamID, humanize.Bytes(uint64(tbl.Size())))
	return nil
}
