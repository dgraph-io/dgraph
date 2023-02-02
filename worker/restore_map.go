//go:build !oss
// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang/glog"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/ee"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

type backupReader struct {
	toClose []io.Closer
	r       io.Reader
	err     error
	once    sync.Once
}

func readerFrom(h UriHandler, file string) *backupReader {
	br := &backupReader{}
	reader, err := h.Stream(file)
	br.setErr(err)
	br.toClose = append(br.toClose, reader)
	br.r = reader
	return br
}
func (br *backupReader) Read(p []byte) (n int, err error) {
	return br.r.Read(p)
}
func (br *backupReader) Close() (rerr error) {
	br.once.Do(func() {
		// Close in reverse order.
		for i := len(br.toClose) - 1; i >= 0; i-- {
			if err := br.toClose[i].Close(); err != nil {
				rerr = err
			}
		}
	})
	return rerr
}
func (br *backupReader) setErr(err error) {
	if br.err == nil {
		br.err = err
	}
}
func (br *backupReader) WithEncryption(encKey x.Sensitive) *backupReader {
	if len(encKey) == 0 {
		return br
	}
	r, err := enc.GetReader(encKey, br.r)
	br.setErr(err)
	br.r = r
	return br
}
func (br *backupReader) WithCompression(comp string) *backupReader {
	switch comp {
	case "snappy":
		br.r = snappy.NewReader(br.r)
	case "gzip", "":
		r, err := gzip.NewReader(br.r)
		br.setErr(err)
		br.r = r
		br.toClose = append(br.toClose, r)
	default:
		br.setErr(fmt.Errorf("unknown compression for backup: %s", comp))
	}
	return br
}

type loadBackupInput struct {
	restoreTs   uint64
	preds       predicateSet
	dropNs      map[uint64]struct{}
	isOld       bool
	keepSchema  bool
	compression string
}

type listReq struct {
	lbuf *z.Buffer
	in   *loadBackupInput
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

type mapper struct {
	once   sync.Once
	nextId uint32
	thr    *y.Throttle

	bytesProcessed uint64
	bytesRead      uint64
	closer         *z.Closer

	buf       *z.Buffer
	bufLock   *sync.Mutex
	restoreTs uint64

	mapDir string
	reqCh  chan listReq
	szHist *z.HistogramData

	maxUid uint64
	maxNs  uint64
}

func (mw *mapper) newMapFile() (*os.File, error) {
	fileNum := atomic.AddUint32(&mw.nextId, 1)
	filename := filepath.Join(mw.mapDir, fmt.Sprintf("%06d.map", fileNum))
	x.Check(os.MkdirAll(filepath.Dir(filename), 0750))

	return os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
}

func (m *mapper) writeToDisk(buf *z.Buffer) error {
	defer buf.Release()
	if buf.IsEmpty() {
		return nil
	}

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
	if fi, err := f.Stat(); err == nil {
		glog.Infof("Created new backup map file: %s of size: %s\n",
			fi.Name(), humanize.IBytes(uint64(fi.Size())))
	}
	return f.Close()
}

func (mw *mapper) sendForWriting() error {
	if mw.buf.IsEmpty() {
		return nil
	}
	mw.buf.SortSlice(func(ls, rs []byte) bool {
		lme := mapEntry(ls)
		rme := mapEntry(rs)
		return y.CompareKeys(lme.Key(), rme.Key()) < 0
	})

	if err := mw.thr.Do(); err != nil {
		return err
	}
	go func(buf *z.Buffer) {
		err := mw.writeToDisk(buf)
		mw.thr.Done(err)
	}(mw.buf)
	mw.buf = z.NewBuffer(mapFileSz, "Restore.Buffer")
	return nil
}

func (mw *mapper) Flush() error {
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

func fromBackupKey(key []byte) ([]byte, uint64, error) {
	backupKey := &pb.BackupKey{}
	if err := backupKey.Unmarshal(key); err != nil {
		return nil, 0, errors.Wrapf(err, "while reading backup key %s", hex.Dump(key))
	}
	return x.FromBackupKey(backupKey), backupKey.Namespace, nil
}

func (m *mapper) processReqCh(ctx context.Context) error {
	buf := z.NewBuffer(20<<20, "processKVList")
	defer buf.Release()

	maxNs := uint64(0)
	maxUid := uint64(0)

	toBuffer := func(kv *bpb.KV, version uint64) error {
		key := y.KeyWithTs(kv.Key, version)
		sz := kv.Size()
		buf := buf.SliceAllocate(2 + len(key) + sz)

		binary.BigEndian.PutUint16(buf[0:2], uint16(len(key)))
		x.AssertTrue(copy(buf[2:], key) == len(key))
		_, err := kv.MarshalToSizedBuffer(buf[2+len(key):])
		return err
	}

	processKV := func(in *loadBackupInput, kv *bpb.KV) error {
		if len(kv.GetUserMeta()) != 1 {
			return errors.Errorf(
				"Unexpected meta: %v for key: %s", kv.UserMeta, hex.Dump(kv.Key))
		}

		restoreKey, ns, err := fromBackupKey(kv.Key)
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

		// Update the local max uid and max namespace values.
		maxUid = x.Max(maxUid, parsedKey.Uid)
		maxNs = x.Max(maxNs, ns)

		if !in.keepSchema && (parsedKey.IsSchema() || parsedKey.IsType()) {
			return nil
		}
		if _, ok := in.preds[parsedKey.Attr]; !parsedKey.IsType() && !ok {
			return nil
		}

		switch kv.GetUserMeta()[0] {
		case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
			if _, ok := in.dropNs[ns]; ok {
				return nil
			}
			backupPl := &pb.BackupPostingList{}
			if err := backupPl.Unmarshal(kv.Value); err != nil {
				return errors.Wrapf(err, "while reading backup posting list")
			}

			pl := posting.FromBackupPostingList(backupPl)
			defer codec.FreePack(pl.Pack)

			shouldSplit := pl.Size() >= (1<<20)/2 && len(pl.Pack.Blocks) > 1
			if !shouldSplit || parsedKey.HasStartUid || len(pl.GetSplits()) > 0 {
				// This covers two cases.
				// 1. The list is not big enough to be split.
				// 2. This key is storing part of a multi-part list. Write each individual
				// part without rolling the key first. This part is here for backwards
				// compatibility. New backups are not affected because there was a change
				// to roll up lists into a single one.
				newKv := posting.MarshalPostingList(pl, nil)
				newKv.Key = restoreKey

				// We are using kv.Version (from the key-value) to generate the key. But, using
				// restoreTs to set the version of the KV. This way, when we sort the keys, we
				// choose the latest key based on kv.Version. But, then set its version to
				// restoreTs.
				newKv.Version = m.restoreTs
				if err := toBuffer(newKv, kv.Version); err != nil {
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
					version := kv.Version
					kv.Version = m.restoreTs
					if err := toBuffer(kv, version); err != nil {
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
				update.TypeName = x.GalaxyAttr(update.TypeName)
				for _, sch := range update.Fields {
					sch.Predicate = x.GalaxyAttr(sch.Predicate)
				}
				kv.Value, err = update.Marshal()
				return err
			}
			if in.isOld && parsedKey.IsType() {
				if err := appendNamespace(); err != nil {
					glog.Errorf("Unable to (un)marshal type: %+v. Err=%v\n", parsedKey, err)
					return nil
				}
			}
			// Reset the StreamId to prevent ordering issues while writing to stream writer.
			kv.StreamId = 0
			// Schema and type keys are not stored in an intermediate format so their
			// value can be written as is.
			version := kv.Version
			kv.Version = m.restoreTs
			kv.Key = restoreKey
			if err := toBuffer(kv, version); err != nil {
				return err
			}

		default:
			return errors.Errorf(
				"Unexpected meta %d for key %s", kv.UserMeta[0], hex.Dump(kv.Key))
		}
		return nil
	}

	mergeBuffer := func() error {
		if buf.IsEmpty() {
			return nil
		}
		atomic.AddUint64(&m.bytesProcessed, uint64(buf.LenNoPadding()))

		m.bufLock.Lock()
		defer m.bufLock.Unlock()

		x.Check2(m.buf.Write(buf.Bytes()))
		buf.Reset()

		if m.buf.LenNoPadding() < mapFileSz {
			return nil
		}
		return m.sendForWriting()
	}

	var list bpb.KVList
	process := func(req listReq) error {
		defer req.lbuf.Release()

		if ctx.Err() != nil {
			return ctx.Err()
		}
		return req.lbuf.SliceIterate(func(s []byte) error {
			list.Reset()
			if err := list.Unmarshal(s); err != nil {
				return err
			}
			for _, kv := range list.GetKv() {
				if err := processKV(req.in, kv); err != nil {
					return err
				}
				if buf.LenNoPadding() > 16<<20 {
					if err := mergeBuffer(); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}

	for req := range m.reqCh {
		if err := process(req); err != nil {
			return err
		}
	}
	if err := mergeBuffer(); err != nil {
		return err
	}

	// Update the global maxUid and maxNs. We need CAS here because mapping is
	// being carried out concurrently.
	for {
		oldMaxUid := atomic.LoadUint64(&m.maxUid)
		newMaxUid := x.Max(oldMaxUid, maxUid)
		if swapped := atomic.CompareAndSwapUint64(&m.maxUid, oldMaxUid, newMaxUid); swapped {
			break
		}
	}
	for {
		oldMaxNs := atomic.LoadUint64(&m.maxNs)
		newMaxNs := x.Max(oldMaxNs, maxNs)
		if swapped := atomic.CompareAndSwapUint64(&m.maxNs, oldMaxNs, newMaxNs); swapped {
			break
		}
	}

	return nil
}

func (m *mapper) Progress() {
	defer m.closer.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	start := time.Now()
	update := func() {
		read := atomic.LoadUint64(&m.bytesRead)
		proc := atomic.LoadUint64(&m.bytesProcessed)
		since := time.Since(start)
		rate := uint64(float64(proc) / since.Seconds())
		glog.Infof("Restore MAP %s read: %s. output: %s. rate: %s/sec. jemalloc: %s.\n",
			x.FixedDuration(since), humanize.IBytes(read), humanize.IBytes(proc),
			humanize.IBytes(rate), humanize.IBytes(uint64(z.NumAllocBytes())))
	}
	for {
		select {
		case <-m.closer.HasBeenClosed():
			update()
			glog.Infof("Restore MAP Done in %s.\n", x.FixedDuration(time.Since(start)))
			return
		case <-ticker.C:
			update()
		}
	}
}

const bufSz = 64 << 20
const bufSoftLimit = bufSz - 2<<20

// mapToDisk reads the backup, converts the keys and values to the required format,
// and loads them to the given badger DB. The set of predicates is used to avoid restoring
// values from predicates no longer assigned to this group.
// If restoreTs is greater than zero, the key-value pairs will be written with that timestamp.
// Otherwise, the original value is used.
// TODO(DGRAPH-1234): Check whether restoreTs can be removed.
func (m *mapper) Map(r io.Reader, in *loadBackupInput) error {
	br := bufio.NewReaderSize(r, 16<<10)
	zbuf := z.NewBuffer(bufSz, "Restore.Map")

	for {
		var sz uint64
		err := binary.Read(br, binary.LittleEndian, &sz)
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		m.szHist.Update(int64(sz))
		buf := zbuf.SliceAllocate(int(sz))
		if _, err = io.ReadFull(br, buf); err != nil {
			return err
		}

		if zbuf.LenNoPadding() > bufSoftLimit {
			atomic.AddUint64(&m.bytesRead, uint64(zbuf.LenNoPadding()))
			glog.Infof("Sending req of size: %s\n", humanize.IBytes(uint64(zbuf.LenNoPadding())))
			m.reqCh <- listReq{zbuf, in}
			zbuf = z.NewBuffer(bufSz, "Restore.Map")
		}
	}
	m.reqCh <- listReq{zbuf, in}
	return nil
}

type mapResult struct {
	maxUid uint64
	maxNs  uint64
}

// we create MAP files each of a limited size and write sorted data into it. We may end up
// creating many such files. Then we take all of these MAP files and read part of the data
// from each file, sort all of this data and then use streamwriter to write the sorted data
// into pstore badger. We store some sort of partition keys in the MAP file in the beginning
// of the file. The partition keys are just intermediate keys among the entries that we store
// in the map file. When we read data during reduce, we read in the chunks of these partition
// keys, meaning from one partition key to the next partition key. I am not sure if there is a
// value in having these partition keys. Maybe, we can live without them.
func RunMapper(req *pb.RestoreRequest, mapDir string) (*mapResult, error) {
	uri, err := url.Parse(req.Location)
	if err != nil {
		return nil, err
	}
	if req.RestoreTs == 0 {
		return nil, errors.New("RestoreRequest must have a valid restoreTs")
	}

	creds := getCredentialsFromRestoreRequest(req)
	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return nil, err
	}

	manifests, err := getManifestsToRestore(h, uri, req)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot retrieve manifests")
	}
	glog.Infof("Got %d backups to restore ", len(manifests))

	cfg, err := getEncConfig(req)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get encryption config")
	}
	keys, err := ee.GetKeys(cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to get encryption keys")
	}

	mapper := &mapper{
		buf:       z.NewBuffer(mapFileSz, "Restore.Buffer"),
		thr:       y.NewThrottle(3),
		bufLock:   &sync.Mutex{},
		closer:    z.NewCloser(1),
		reqCh:     make(chan listReq, 3),
		restoreTs: req.RestoreTs,
		mapDir:    mapDir,
		szHist:    z.NewHistogramData(z.HistogramBounds(10, 32)),
	}

	numGo := 8
	g, ctx := errgroup.WithContext(mapper.closer.Ctx())
	for i := 0; i < numGo; i++ {
		g.Go(func() error {
			return mapper.processReqCh(ctx)
		})
	}
	go mapper.Progress()
	defer func() {
		mapper.Flush()
		mapper.closer.SignalAndWait()
	}()

	dropAll := false
	dropAttr := make(map[string]struct{})
	dropNs := make(map[uint64]struct{})
	var maxBannedNs uint64

	// manifests are ordered as: latest..full
	for i, manifest := range manifests {
		// A dropAll or DropData operation is encountered. No need to restore previous backups.
		if dropAll {
			break
		}
		if manifest.ValidReadTs() == 0 || len(manifest.Groups) == 0 {
			continue
		}
		for gid := range manifest.Groups {
			if gid != req.GroupId {
				// LoadBackup will try to call the backup function for every group.
				// Exit here if the group is not the one indicated by the request.
				continue
			}

			// Only restore the predicates that were assigned to this group at the time
			// of the last backup.
			file := filepath.Join(manifest.Path, backupName(manifest.ValidReadTs(), gid))
			br := readerFrom(h, file).WithEncryption(keys.EncKey).WithCompression(manifest.Compression)
			if br.err != nil {
				return nil, errors.Wrap(br.err, "newBackupReader")
			}
			defer br.Close()

			// Only map the predicates which haven't been dropped yet.
			predSet := manifest.getPredsInGroup(gid)
			for p := range predSet {
				if _, ok := dropAttr[p]; ok {
					delete(predSet, p)
				}
			}
			localDropNs := make(map[uint64]struct{})
			for ns := range dropNs {
				localDropNs[ns] = struct{}{}
			}
			in := &loadBackupInput{
				preds:     predSet,
				dropNs:    localDropNs,
				isOld:     manifest.Version == 0,
				restoreTs: req.RestoreTs,
				// Only map the schema keys corresponding to the latest backup.
				keepSchema:  i == 0,
				compression: manifest.Compression,
			}

			// This would stream the backups from the source, and map them in
			// Dgraph compatible format on disk.
			if err := mapper.Map(br, in); err != nil {
				return nil, errors.Wrap(err, "mapper.Map")
			}
			if err := br.Close(); err != nil {
				return nil, errors.Wrap(err, "br.Close")
			}
		}
		for _, op := range manifest.DropOperations {
			switch op.DropOp {
			case pb.DropOperation_ALL:
				dropAll = true
			case pb.DropOperation_DATA:
				var ns uint64
				if manifest.Version == 0 {
					ns = x.GalaxyNamespace
				} else {
					var err error
					ns, err = strconv.ParseUint(op.DropValue, 0, 64)
					if err != nil {
						return nil, errors.Wrap(err, "Map phase failed to parse namespace")
					}
				}
				dropNs[ns] = struct{}{}
			case pb.DropOperation_ATTR:
				p := op.DropValue
				if manifest.Version == 0 {
					p = x.NamespaceAttr(x.GalaxyNamespace, p)
				}
				dropAttr[p] = struct{}{}
			case pb.DropOperation_NS:
				// pstore will be nil for export_backup tool. In that case we don't need to ban ns.
				if pstore == nil {
					continue
				}
				// If there is a drop namespace, we just ban the namespace in the pstore.
				ns, err := strconv.ParseUint(op.DropValue, 0, 64)
				if err != nil {
					return nil, errors.Wrapf(err, "Map phase failed to parse namespace")
				}
				if err := pstore.BanNamespace(ns); err != nil {
					return nil, errors.Wrapf(err, "Map phase failed to ban namespace: %d", ns)
				}
				maxBannedNs = x.Max(maxBannedNs, ns)
			}
		}
	} // done with all the manifests.

	glog.Infof("Histogram of map input sizes:\n%s\n", mapper.szHist)
	close(mapper.reqCh)
	if err := g.Wait(); err != nil {
		return nil, errors.Wrapf(err, "from processKVList")
	}
	if err := mapper.Flush(); err != nil {
		return nil, errors.Wrap(err, "failed to flush the mapper")
	}
	mapRes := &mapResult{
		maxUid: mapper.maxUid,
		maxNs:  mapper.maxNs,
	}
	// update the maxNsId considering banned namespaces.
	mapRes.maxNs = x.Max(mapRes.maxNs, maxBannedNs)
	return mapRes, nil
}
