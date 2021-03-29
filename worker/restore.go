// +build !oss

/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/badger/v3"
	"github.com/dgraph-io/badger/v3/options"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/golang/snappy"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

type loadBackupInput struct {
	r              io.Reader
	restoreTs      uint64
	preds          predicateSet
	dropOperations []*pb.DropOperation
	isOld          bool
}

type mapper struct {
	once   sync.Once
	buf    *z.Buffer
	nextId uint32
	thr    *y.Throttle
}

const (
	mapFileSz      int = 2 << 30
	partitionBufSz int = 4 << 20
	restoreTmpDir      = "restore-tmp"
	restoreMapDir      = "restore-map"
)

func newBuffer() *z.Buffer {
	path := filepath.Join(x.WorkerConfig.TmpDir, restoreTmpDir)
	x.Check(os.MkdirAll(path, 0750))
	buf, err := z.NewBufferWithDir(mapFileSz, 2*mapFileSz, z.UseMmap,
		path, "Restore.Buffer")
	x.Check(err)
	return buf
}

// mapEntry stores uint16 (2 bytes), which store the length of the key, followed by the key itself.
// The rest of the mapEntry stores the marshalled KV.
// We store the key alongside the protobuf, to make it easier to parse for comparison.
type mapEntry []byte

func (me mapEntry) Key() []byte {
	sz := binary.BigEndian.Uint16(me[0:2])
	return me[2 : 2+sz]
}
func (me mapEntry) Data() []byte {
	sz := binary.BigEndian.Uint16(me[0:2])
	return me[2+sz:]
}

func (mw *mapper) Set(kv *bpb.KV) error {

	key := y.KeyWithTs(kv.Key, kv.Version)
	sz := kv.Size()
	buf := mw.buf.SliceAllocate(2 + len(key) + sz)

	binary.BigEndian.PutUint16(buf[0:2], uint16(len(key)))
	x.AssertTrue(copy(buf[2:], key) == len(key))
	if _, err := kv.MarshalToSizedBuffer(buf[2+len(key):]); err != nil {
		return err
	}
	if mw.buf.LenNoPadding() <= mapFileSz {
		return nil
	}
	return mw.sendForWriting()
}

func (mw *mapper) newMapFile() (*os.File, error) {
	fileNum := atomic.AddUint32(&mw.nextId, 1)
	filename := filepath.Join(
		x.WorkerConfig.TmpDir,
		restoreMapDir,
		fmt.Sprintf("%06d.map", fileNum),
	)
	glog.Infof("Creating new backup map file at: %q", filename)
	x.Check(os.MkdirAll(filepath.Dir(filename), 0750))
	return os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
}

func (m *mapper) writeToDisk(buf *z.Buffer) error {
	defer buf.Release()
	if buf.IsEmpty() {
		return nil
	}
	buf.SortSlice(func(ls, rs []byte) bool {
		lme := mapEntry(ls)
		rme := mapEntry(rs)
		return y.CompareKeys(lme.Key(), rme.Key()) < 0
	})

	f, err := m.newMapFile()
	if err != nil {
		return errors.Wrap(err, "openOutputFile")
	}
	defer f.Close()

	// Create partition keys for the map file.
	header := &pb.MapHeader{PartitionKeys: [][]byte{}}
	var bufSize int
	buf.SliceIterate(func(slice []byte) error {
		bufSize += 4 + len(slice)
		if bufSize < partitionBufSz {
			return nil
		}
		sz := len(header.PartitionKeys)
		me := mapEntry(slice)
		if sz > 0 && bytes.Equal(me.Key(), header.PartitionKeys[sz-1]) {
			// We already have this key.
			return nil
		}
		header.PartitionKeys = append(header.PartitionKeys, me.Key())
		bufSize = 0
		return nil
	})

	// Write the header to the map file.
	headerBuf, err := header.Marshal()
	x.Check(err)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(headerBuf)))

	w := snappy.NewBufferedWriter(f)
	x.Check2(w.Write(lenBuf[:]))
	x.Check2(w.Write(headerBuf))
	x.Check(err)

	sizeBuf := make([]byte, binary.MaxVarintLen64)
	err = buf.SliceIterate(func(slice []byte) error {
		n := binary.PutUvarint(sizeBuf, uint64(len(slice)))
		_, err := w.Write(sizeBuf[:n])
		x.Check(err)

		_, err = w.Write(slice)
		return err
	})
	if err != nil {
		return errors.Wrap(err, "sliceIterate")
	}
	if err := w.Close(); err != nil {
		return errors.Wrap(err, "writer.Close")
	}
	if err := f.Sync(); err != nil {
		return errors.Wrap(err, "file.Sync")
	}
	return f.Close()
}

func (mw *mapper) sendForWriting() error {
	if err := mw.thr.Do(); err != nil {
		return err
	}
	go func(buf *z.Buffer) {
		err := mw.writeToDisk(buf)
		mw.thr.Done(err)
	}(mw.buf)
	mw.buf = newBuffer()
	return nil
}

func (mw *mapper) Close() error {
	cl := func() error {
		if err := mw.sendForWriting(); err != nil {
			return err
		}
		if err := mw.thr.Finish(); err != nil {
			return err
		}
		return mw.buf.Release()
	}

	var rerr error
	mw.once.Do(func() {
		rerr = cl()
	})
	return rerr
}

// mapToDisk reads the backup, converts the keys and values to the required format,
// and loads them to the given badger DB. The set of predicates is used to avoid restoring
// values from predicates no longer assigned to this group.
// If restoreTs is greater than zero, the key-value pairs will be written with that timestamp.
// Otherwise, the original value is used.
// TODO(DGRAPH-1234): Check whether restoreTs can be removed.
func (m *mapper) Map(in *loadBackupInput, keepSchema bool) error {
	br := bufio.NewReaderSize(in.r, 16<<10)
	unmarshalBuf := make([]byte, 1<<10)
	for {
		var sz uint64
		err := binary.Read(br, binary.LittleEndian, &sz)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if cap(unmarshalBuf) < int(sz) {
			unmarshalBuf = make([]byte, sz)
		}

		if _, err = io.ReadFull(br, unmarshalBuf[:sz]); err != nil {
			return err
		}

		list := &bpb.KVList{}
		if err := list.Unmarshal(unmarshalBuf[:sz]); err != nil {
			return err
		}

		for _, kv := range list.Kv {
			if len(kv.GetUserMeta()) != 1 {
				return errors.Errorf(
					"Unexpected meta: %v for key: %s", kv.UserMeta, hex.Dump(kv.Key))
			}

			restoreKey, _, err := fromBackupKey(kv.Key)
			if err != nil {
				return errors.Wrap(err, "fromBackupKey")
			}

			// Filter keys using the preds set. Do not do this filtering for type keys
			// as they are meant to be in every group and their Attr value does not
			// match a predicate name.
			parsedKey, err := x.Parse(restoreKey)
			if err != nil {
				return errors.Wrapf(err, "could not parse key %s", hex.Dump(restoreKey))
			}
			if !keepSchema && (parsedKey.IsSchema() || parsedKey.IsType()) {
				continue
			}
			if _, ok := in.preds[parsedKey.Attr]; !parsedKey.IsType() && !ok {
				continue
			}

			switch kv.GetUserMeta()[0] {
			case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
				backupPl := &pb.BackupPostingList{}
				if err := backupPl.Unmarshal(kv.Value); err != nil {
					return errors.Wrapf(err, "while reading backup posting list")
				}
				pl := posting.FromBackupPostingList(backupPl)

				shouldSplit, err := posting.ShouldSplit(pl)
				if err != nil {
					return errors.Wrap(err, "Failed to get shouldSplit")
				}

				if !shouldSplit || parsedKey.HasStartUid || len(pl.GetSplits()) > 0 {
					// This covers two cases.
					// 1. The list is not big enough to be split.
					// 2. This key is storing part of a multi-part list. Write each individual
					// part without rolling the key first. This part is here for backwards
					// compatibility. New backups are not affected because there was a change
					// to roll up lists into a single one.
					newKv := posting.MarshalPostingList(pl, nil)
					newKv.Key = restoreKey
					// Use the version of the KV before we marshalled the
					// posting list. The MarshalPostingList function returns KV
					// with a zero version.
					newKv.Version = kv.Version
					if err := m.Set(newKv); err != nil {
						return err
					}
				} else {
					// This is a complete list. It should be rolled up to avoid writing
					// a list that is too big to be read back from disk.
					// Rollup will take ownership of the Pack and will free the memory.
					l := posting.NewList(restoreKey, pl, kv.Version)
					kvs, err := l.Rollup(nil)
					if err != nil {
						// TODO: wrap errors in this file for easier debugging.
						return err
					}
					for _, kv := range kvs {
						if err := m.Set(kv); err != nil {
							return err
						}
					}
				}

			case posting.BitSchemaPosting:
				appendNamespace := func() error {
					// If the backup was taken on old version, we need to append the namespace to
					// the fields of TypeUpdate.
					var update pb.TypeUpdate
					if err := update.Unmarshal(kv.Value); err != nil {
						return err
					}
					for _, sch := range update.Fields {
						sch.Predicate = x.GalaxyAttr(sch.Predicate)
					}
					kv.Value, err = update.Marshal()
					return err
				}
				if in.isOld && parsedKey.IsType() {
					if err := appendNamespace(); err != nil {
						glog.Errorf("Unable to (un)marshal type: %+v. Err=%v\n", parsedKey, err)
						continue
					}
				}
				// Schema and type keys are not stored in an intermediate format so their
				// value can be written as is.
				kv.Key = restoreKey
				if err := m.Set(kv); err != nil {
					return err
				}

			default:
				return errors.Errorf(
					"Unexpected meta %d for key %s", kv.UserMeta[0], hex.Dump(kv.Key))
			}
		}
	}
	return nil
}

func fromBackupKey(key []byte) ([]byte, uint64, error) {
	backupKey := &pb.BackupKey{}
	if err := backupKey.Unmarshal(key); err != nil {
		return nil, 0, errors.Wrapf(err, "while reading backup key %s", hex.Dump(key))
	}
	return x.FromBackupKey(backupKey), backupKey.Namespace, nil
}

type backupReader struct {
	toClose []io.Closer
	r       io.Reader
}

func (br *backupReader) Read(p []byte) (n int, err error) {
	return br.r.Read(p)
}
func (br *backupReader) Close() (rerr error) {
	for i := len(br.toClose) - 1; i >= 0; i-- {
		if err := br.toClose[i].Close(); err != nil {
			rerr = err
		}
	}
	return rerr
}
func newBackupReader(h UriHandler, file string, encKey x.Sensitive) (*backupReader, error) {
	br := &backupReader{}
	reader, err := h.Stream(file)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to open %q", file)
	}
	br.toClose = append(br.toClose, reader)

	encReader, err := enc.GetReader(encKey, reader)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot get encrypted reader")
	}
	gzReader, err := gzip.NewReader(encReader)
	if err != nil {
		return nil, errors.Wrapf(err, "couldn't create gzip reader")
	}
	br.toClose = append(br.toClose, gzReader)

	br.r = bufio.NewReaderSize(gzReader, 16<<10)
	return br, nil
}

func MapBackup(req *pb.RestoreRequest) error {
	uri, err := url.Parse(req.Location)
	if err != nil {
		return err
	}

	creds := getCredentialsFromRestoreRequest(req)
	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return err
	}

	manifests, err := getManifestsToRestore(h, uri, req)
	if err != nil {
		return errors.Wrapf(err, "cannot retrieve manifests")
	}

	fmt.Printf("Got %d backups to restore ", len(manifests))

	cfg, err := getEncConfig(req)
	if err != nil {
		return errors.Wrapf(err, "unable to get encryption config")
	}
	_, encKey := ee.GetKeys(cfg)

	mapper := &mapper{
		buf: newBuffer(),
		thr: y.NewThrottle(3),
	}
	defer mapper.Close()

	dropAll := false
	dropAttr := make(map[string]struct{})

	// manifests are ordered as: latest..full
	for i, manifest := range manifests {
		// A dropAll or DropData operation is encountered. No need to restore previous backups.
		if dropAll {
			break
		}
		if manifest.Since == 0 || len(manifest.Groups) == 0 {
			continue
		}
		path := manifest.Path
		for gid := range manifest.Groups {
			if gid != req.GroupId {
				// LoadBackup will try to call the backup function for every group.
				// Exit here if the group is not the one indicated by the request.
				continue
			}
			file := filepath.Join(path, backupName(manifest.Since, gid))

			// Only restore the predicates that were assigned to this group at the time
			// of the last backup.
			predSet := manifests[0].getPredsInGroup(gid)
			br, err := newBackupReader(h, file, encKey)
			if err != nil {
				return errors.Wrap(err, "newBackupReader")
			}

			// Only map the predicates which haven't been dropped yet.
			for p, _ := range predSet {
				if _, ok := dropAttr[p]; ok {
					delete(predSet, p)
				}
			}
			in := &loadBackupInput{
				r:              br,
				preds:          predSet,
				dropOperations: manifest.DropOperations,
				isOld:          manifest.Version == 0,
				restoreTs:      req.RestoreTs,
			}

			// Only map the schema keys corresponding to the latest backup.
			keepSchema := i == 0

			// This would stream the backups from the source, and map them in
			// Dgraph compatible format on disk.
			if err := mapper.Map(in, keepSchema); err != nil {
				return errors.Wrap(err, "mapper.Map")
			}
			if err := br.Close(); err != nil {
				return errors.Wrap(err, "br.Close")
			}
		}
		for _, op := range manifest.DropOperations {
			switch op.DropOp {
			case pb.DropOperation_ALL:
				dropAll = true
			case pb.DropOperation_DATA:
				dropAll = true
			case pb.DropOperation_ATTR:
				dropAttr[op.DropValue] = struct{}{}
			case pb.DropOperation_NS:
				// If there is a drop namespace, we just ban the namespace in the pstore.
				// TODO: We probably need to propose ban request.
				ns, err := strconv.ParseUint(op.DropValue, 0, 64)
				if err != nil {
					return errors.Wrapf(err, "Map phase failed to parse namespace")
				}
				if err := pstore.BanNamespace(ns); err != nil {
					return errors.Wrapf(err, "Map phase failed to ban namespace: %d", ns)
				}
			}
		}
	}
	return nil
}

// VerifyBackup will access the backup location and verify that the specified backup can
// be restored to the cluster.
func VerifyBackup(req *pb.RestoreRequest, creds *x.MinioCredentials, currentGroups []uint32) error {
	uri, err := url.Parse(req.GetLocation())
	if err != nil {
		return err
	}

	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return errors.Wrap(err, "VerifyBackup")
	}

	return verifyRequest(h, uri, req, currentGroups)
}

// verifyRequest verifies that the manifest satisfies the requirements to process the given
// restore request.
func verifyRequest(h UriHandler, uri *url.URL, req *pb.RestoreRequest,
	currentGroups []uint32) error {

	manifests, err := getManifestsToRestore(h, uri, req)
	if err != nil {
		return errors.Wrapf(err, "while retrieving manifests")
	}
	if len(manifests) == 0 {
		return errors.Errorf("No backups with the specified backup ID %s", req.GetBackupId())
	}

	// TODO(Ahsan): Do we need to verify the manifests again here?
	if err := verifyManifests(manifests); err != nil {
		return err
	}

	lastManifest := manifests[0]
	if len(currentGroups) != len(lastManifest.Groups) {
		return errors.Errorf("groups in cluster and latest backup manifest differ")
	}

	for _, group := range currentGroups {
		if _, ok := lastManifest.Groups[group]; !ok {
			return errors.Errorf("groups in cluster and latest backup manifest differ")
		}
	}
	return nil
}

type mapIterator struct {
	fd     *os.File
	reader *bufio.Reader
	meBuf  []byte
}

func (mi *mapIterator) Next(cbuf *z.Buffer, partitionKey []byte) error {
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
		} else if err != nil {
			return err
		}
		key := mapEntry(mi.meBuf).Key()

		if len(partitionKey) == 0 || y.CompareKeys(key, partitionKey) < 0 {
			b := cbuf.SliceAllocate(len(mi.meBuf))
			copy(b, mi.meBuf)
			mi.meBuf = mi.meBuf[:0]
			// map entry is already part of cBuf.
			continue
		}
		// Current key is not part of this batch so track that we have already read the key.
		return nil
	}
	return nil
}

func (mi *mapIterator) Close() error {
	return mi.fd.Close()
}

func newMapIterator(filename string) (*pb.MapHeader, *mapIterator) {
	fd, err := os.Open(filename)
	x.Check(err)
	r := snappy.NewReader(fd)

	// Read the header size.
	reader := bufio.NewReaderSize(r, 16<<10)
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

func getBuf() *z.Buffer {
	path := filepath.Join(x.WorkerConfig.TmpDir, "buffer")
	x.Check(os.MkdirAll(path, 0750))
	cbuf, err := z.NewBufferWithDir(64<<20, 64<<30, z.UseCalloc, path, "Restore.GetBuf")
	x.Check(err)
	cbuf.AutoMmapAfter(1 << 30)
	return cbuf
}

type reducer struct {
	mapItrs       []*mapIterator
	partitionKeys [][]byte
	bufferCh      chan *z.Buffer
	db            *badger.DB
	writeCh       chan *z.Buffer
	restoreTs     uint64
}

func reduceToDB(db *badger.DB, restoreTs uint64) error {
	r := NewBackupReducer(db, restoreTs)
	return r.Reduce()
}

func NewBackupReducer(db *badger.DB, restoreTs uint64) *reducer {
	return &reducer{
		db:        db,
		restoreTs: restoreTs,
		bufferCh:  make(chan *z.Buffer, 10),
		writeCh:   make(chan *z.Buffer, 10),
	}
}

func (r *reducer) WriteCh() chan *z.Buffer {
	return r.writeCh
}

func (r *reducer) Reduce() error {
	var files []string
	f := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(info.Name(), ".map") {
			files = append(files, path)
		}
		return nil
	}

	mapDir := filepath.Join(x.WorkerConfig.TmpDir, restoreMapDir)
	defer os.RemoveAll(mapDir)
	if err := filepath.Walk(mapDir, f); err != nil {
		return err
	}
	glog.Infof("Got files: %+v\n", files)

	// Pick up map iterators and partition keys.
	partitions := make(map[string]struct{})
	for _, fname := range files {
		header, itr := newMapIterator(fname)
		for _, k := range header.PartitionKeys {
			if len(k) == 0 {
				continue
			}
			partitions[string(k)] = struct{}{}
		}
		r.mapItrs = append(r.mapItrs, itr)
	}

	keys := make([][]byte, 0, len(partitions))
	for k := range partitions {
		keys = append(keys, []byte(k))
	}
	sort.Slice(keys, func(i, j int) bool {
		return y.CompareKeys(keys[i], keys[j]) < 0
	})
	// Append nil for the last entries.
	keys = append(keys, nil)
	r.partitionKeys = keys

	errCh := make(chan error, 2)
	go func() {
		errCh <- r.blockingRead()
	}()
	go func() {
		errCh <- r.writeToDB()
	}()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func (r *reducer) blockingRead() error {
	cbuf := getBuf()
	for _, pkey := range r.partitionKeys {
		for _, itr := range r.mapItrs {
			if err := itr.Next(cbuf, pkey); err != nil {
				cbuf.Release()
				return err
			}
		}
		if cbuf.LenNoPadding() < 256<<20 {
			// Pick up more data.
			continue
		}
		r.bufferCh <- cbuf
		cbuf = getBuf()
	}

	if !cbuf.IsEmpty() {
		r.bufferCh <- cbuf
	} else {
		cbuf.Release()
	}
	close(r.bufferCh)
	return nil
}

func (r *reducer) writeToDB() error {
	toStreamWriter := func() error {
		if r.db == nil {
			return nil
		}
		writer := r.db.NewStreamWriter()
		x.Check(writer.Prepare())

		for buf := range r.writeCh {
			if err := writer.Write(buf); err != nil {
				return err
			}
			buf.Release()
		}
		return writer.Flush()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- toStreamWriter()
	}()

	kvBuf := getBuf()
	var lastKey []byte
	for cbuf := range r.bufferCh {
		cbuf.SortSlice(func(ls, rs []byte) bool {
			lme := mapEntry(ls)
			rme := mapEntry(rs)
			return y.CompareKeys(lme.Key(), rme.Key()) < 0
		})

		err := cbuf.SliceIterate(func(s []byte) error {
			me := mapEntry(s)
			key := me.Key()

			pk, err := x.Parse(key)
			if err != nil {
				return errors.Wrap(err, "writeToDB failed to parse key")
			}

			// Don't need to pick multiple versions of the same key.
			if y.SameKey(key, lastKey) {
				return nil
			}

			kv := &bpb.KV{}
			b := me.Data()
			// Override the version if requested. Should not be done for type and schema predicates,
			// which always have their version set to 1.
			if r.restoreTs > 0 && !pk.IsSchema() && !pk.IsType() {
				if err := kv.Unmarshal(me.Data()); err != nil {
					return errors.Wrap(err, "writeToDB failed to unmarshal KV")
				}
				kv.Version = r.restoreTs
				b = make([]byte, kv.Size())
				if _, err := kv.MarshalToSizedBuffer(b); err != nil {
					return errors.Wrap(err, "writeToDB failed to marshal KV")
				}
			}

			lastKey = append(lastKey[:0], key...)
			kvBuf.WriteSlice(b)
			return nil
		})
		if err != nil {
			return err
		}

		r.writeCh <- kvBuf
		// Reuse cbuf for the next kvBuf.
		cbuf.Reset()
		kvBuf = cbuf
	}
	close(r.writeCh)
	kvBuf.Release()
	return <-errCh
}

// RunRestore creates required DBs and streams the backups to them. It is used only for testing.
func RunRestore(dir, location, backupId string, keyFile string,
	ctype options.CompressionType, clevel int) LoadResult {
	// Create the pdir if it doesn't exist.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return LoadResult{Err: err}
	}

	uri, err := url.Parse(location)
	if err != nil {
		return LoadResult{Err: err}
	}

	h, err := NewUriHandler(uri, nil)
	if err != nil {
		return LoadResult{Err: errors.Errorf("Unsupported URI: %v", uri)}
	}
	manifest, err := GetLatestManifest(h, uri)
	if err != nil {
		return LoadResult{Err: errors.Wrapf(err, "cannot retrieve manifests")}
	}
	var key x.Sensitive
	if len(keyFile) > 0 {
		key, err = ioutil.ReadFile(keyFile)
		if err != nil {
			return LoadResult{Err: errors.Wrapf(err, "RunRestore failed to read enc-key")}
		}
	}

	for gid := range manifest.Groups {
		req := &pb.RestoreRequest{
			Location:          location,
			GroupId:           gid,
			BackupId:          backupId,
			EncryptionKeyFile: keyFile,
		}
		if err := MapBackup(req); err != nil {
			return LoadResult{Err: errors.Wrap(err, "RunRestore failed to map")}
		}
		pdir := filepath.Join(dir, fmt.Sprintf("p%d", gid))
		db, err := badger.OpenManaged(badger.DefaultOptions(pdir).
			WithCompression(ctype).
			WithZSTDCompressionLevel(clevel).
			WithSyncWrites(false).
			WithBlockCacheSize(100 * (1 << 20)).
			WithIndexCacheSize(100 * (1 << 20)).
			WithNumVersionsToKeep(math.MaxInt32).
			WithEncryptionKey(key).
			WithNamespaceOffset(x.NamespaceOffset))
		if err != nil {
			return LoadResult{Err: errors.Wrap(err, "RunRestore failed to open DB")}

		}
		defer db.Close()
		if err := reduceToDB(db, 0); err != nil {
			return LoadResult{Err: errors.Wrap(err, "RunRestore failed to reduce")}
		}
		if err := x.WriteGroupIdFile(pdir, uint32(gid)); err != nil {
			return LoadResult{Err: errors.Wrap(err, "RunRestore failed to write group id file")}
		}
	}
	// TODO: Fix this return value.
	return LoadResult{Version: manifest.Since}
}
