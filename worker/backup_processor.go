// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package worker

import (
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"

	"github.com/dgraph-io/badger/v3"
	bpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/badger/v3/y"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// backupNumGo is the number of go routines used by the backup stream writer.
	backupNumGo = 16
)

// BackupProcessor handles the different stages of the backup process.
type BackupProcessor struct {
	// DB is the Badger pstore managed by this node.
	DB *badger.DB
	// Request stores the backup request containing the parameters for this backup.
	Request *pb.BackupRequest

	// txn is used for the iterators in the threadLocal
	txn     *badger.Txn
	threads []*threadLocal
}

type threadLocal struct {
	Request *pb.BackupRequest
	// pre-allocated pb.PostingList object.
	pl pb.PostingList
	// pre-allocated pb.BackupPostingList object.
	bpl   pb.BackupPostingList
	alloc *z.Allocator
	itr   *badger.Iterator
}

func NewBackupProcessor(db *badger.DB, req *pb.BackupRequest) *BackupProcessor {
	bp := &BackupProcessor{
		DB:      db,
		Request: req,
		threads: make([]*threadLocal, backupNumGo),
	}
	if db != nil {
		bp.txn = db.NewTransactionAt(req.ReadTs, false)
	}
	for i := range bp.threads {
		bp.threads[i] = &threadLocal{
			Request: bp.Request,
		}
		if bp.txn != nil {
			iopt := badger.DefaultIteratorOptions
			iopt.AllVersions = true
			bp.threads[i].itr = bp.txn.NewIterator(iopt)
		}
	}
	return bp
}

// LoadResult holds the output of a Load operation.
type LoadResult struct {
	// Version is the timestamp at which the database is after loading a backup.
	Version uint64
	// MaxLeaseUid is the max UID seen by the load operation. Needed to request zero
	// for the proper number of UIDs.
	MaxLeaseUid uint64
	// The error, if any, of the load operation.
	Err error
}

func (pr *BackupProcessor) Close() {
	for _, th := range pr.threads {
		th.itr.Close()
	}
	pr.txn.Discard()
}

// WriteBackup uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (pr *BackupProcessor) WriteBackup(ctx context.Context) (*pb.BackupResponse, error) {
	var response pb.BackupResponse

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	uri, err := url.Parse(pr.Request.Destination)
	if err != nil {
		return &response, err
	}

	handler, err := NewUriHandler(uri, GetCredentialsFromRequest(pr.Request))
	if err != nil {
		return &response, err
	}

	if err := handler.CreateBackupFile(uri, pr.Request); err != nil {
		return &response, err
	}

	glog.V(3).Infof("Backup manifest version: %d", pr.Request.SinceTs)

	predMap := make(map[string]struct{})
	for _, pred := range pr.Request.Predicates {
		predMap[pred] = struct{}{}
	}

	var maxVersion uint64

	newhandler, err := enc.GetWriter(x.WorkerConfig.EncryptionKey, handler)
	if err != nil {
		return &response, err
	}
	gzWriter := gzip.NewWriter(newhandler)

	stream := pr.DB.NewStreamAt(pr.Request.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	stream.NumGo = backupNumGo
	// Ignore versions less than given sinceTs timestamp, or skip older versions of
	// the given key by returning an empty list.
	// Do not do this for schema and type keys. Those keys always have a
	// version of one. They're handled separately.
	stream.SinceTs = pr.Request.SinceTs
	stream.Prefix = []byte{x.ByteData}

	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		tl := pr.threads[itr.ThreadId]
		tl.alloc = itr.Alloc

		bitr := tl.itr // Use the threadlocal iterator because "itr" has the sinceTs set.
		bitr.Seek(key)

		kvList, dropOp, err := tl.toBackupList(key, bitr)
		if err != nil {
			return nil, err
		}
		// we don't want to append a nil value to the slice, so need to check.
		if dropOp != nil {
			response.DropOperations = append(response.DropOperations, dropOp)
		}
		return kvList, nil
	}

	stream.ChooseKey = func(item *badger.Item) bool {
		parsedKey, err := x.Parse(item.Key())
		if err != nil {
			glog.Errorf("error %v while parsing key %v during backup. Skip.", err, hex.EncodeToString(item.Key()))
			return false
		}

		// Do not choose keys that contain parts of a multi-part list. These keys
		// will be accessed from the main list.
		if parsedKey.HasStartUid {
			return false
		}

		// Skip backing up the schema and type keys. They will be backed up separately.
		if parsedKey.IsSchema() || parsedKey.IsType() {
			return false
		}
		_, ok := predMap[parsedKey.Attr]
		return ok
	}
	stream.Send = func(buf *z.Buffer) error {
		list, err := badger.BufferToKVList(buf)
		if err != nil {
			return err
		}
		for _, kv := range list.Kv {
			if maxVersion < kv.Version {
				maxVersion = kv.Version
			}
		}
		return writeKVList(list, gzWriter)
	}

	if err := stream.Orchestrate(context.Background()); err != nil {
		glog.Errorf("While taking backup: %v", err)
		return &response, err
	}

	// This is used to backup the schema and types.
	writePrefix := func(prefix byte) error {
		tl := threadLocal{
			alloc: z.NewAllocator(1 << 10),
		}
		defer tl.alloc.Release()

		// Schema and types are written at Ts=1.
		txn := pr.DB.NewTransactionAt(1, false)
		defer txn.Discard()
		// We don't need to iterate over all versions.
		iopts := badger.DefaultIteratorOptions
		iopts.Prefix = []byte{prefix}

		itr := txn.NewIterator(iopts)
		defer itr.Close()

		list := &bpb.KVList{}
		for itr.Rewind(); itr.Valid(); itr.Next() {
			item := itr.Item()
			// Don't export deleted items.
			if item.IsDeletedOrExpired() {
				continue
			}
			parsedKey, err := x.Parse(item.Key())
			if err != nil {
				glog.Errorf("error %v while parsing key %v during backup. Skip.", err, hex.EncodeToString(item.Key()))
				continue
			}
			// This check makes sense only for the schema keys. The types are not stored in it.
			if _, ok := predMap[parsedKey.Attr]; !parsedKey.IsType() && !ok {
				continue
			}
			kv := y.NewKV(tl.alloc)
			if err := item.Value(func(val []byte) error {
				kv.Value = append(kv.Value, val...)
				return nil
			}); err != nil {
				return errors.Wrapf(err, "while copying value")
			}

			backupKey, err := tl.toBackupKey(item.Key())
			if err != nil {
				return err
			}
			kv.Key = backupKey
			kv.UserMeta = tl.alloc.Copy([]byte{item.UserMeta()})
			kv.Version = item.Version()
			kv.ExpiresAt = item.ExpiresAt()
			list.Kv = append(list.Kv, kv)
		}
		return writeKVList(list, gzWriter)
	}

	for _, prefix := range []byte{x.ByteSchema, x.ByteType} {
		if err := writePrefix(prefix); err != nil {
			glog.Errorf("While writing prefix %d to backup: %v", prefix, err)
			return &response, err
		}
	}

	if maxVersion > pr.Request.ReadTs {
		glog.Errorf("Max timestamp seen during backup (%d) is greater than readTs (%d)",
			maxVersion, pr.Request.ReadTs)
	}

	glog.V(2).Infof("Backup group %d version: %d", pr.Request.GroupId, pr.Request.ReadTs)
	if err = gzWriter.Close(); err != nil {
		glog.Errorf("While closing gzipped writer: %v", err)
		return &response, err
	}

	if err = handler.Close(); err != nil {
		glog.Errorf("While closing handler: %v", err)
		return &response, err
	}
	glog.Infof("Backup complete: group %d at %d", pr.Request.GroupId, pr.Request.ReadTs)
	return &response, nil
}

// CompleteBackup will finalize a backup by writing the manifest at the backup destination.
func (pr *BackupProcessor) CompleteBackup(ctx context.Context, manifest *Manifest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	uri, err := url.Parse(pr.Request.Destination)
	if err != nil {
		return err
	}

	handler, err := NewUriHandler(uri, GetCredentialsFromRequest(pr.Request))
	if err != nil {
		return err
	}

	if err := handler.CreateManifest(uri, pr.Request); err != nil {
		return err
	}

	if err = json.NewEncoder(handler).Encode(manifest); err != nil {
		return err
	}

	if err = handler.Close(); err != nil {
		return err
	}
	glog.Infof("Backup completed OK.")
	return nil
}

// GoString implements the GoStringer interface for Manifest.
func (m *Manifest) GoString() string {
	return fmt.Sprintf(`Manifest{Since: %d, Groups: %v, Encrypted: %v}`,
		m.Since, m.Groups, m.Encrypted)
}

func (tl *threadLocal) toBackupList(key []byte, itr *badger.Iterator) (
	*bpb.KVList, *pb.DropOperation, error) {
	list := &bpb.KVList{}
	var dropOp *pb.DropOperation

	item := itr.Item()
	if item.Version() < tl.Request.SinceTs {
		return list, nil,
			errors.Errorf("toBackupList: Item.Version(): %d should be less than sinceTs: %d",
				item.Version(), tl.Request.SinceTs)
	}
	if item.IsDeletedOrExpired() {
		return list, nil, nil
	}

	switch item.UserMeta() {
	case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "while reading posting list")
		}

		// Don't allocate kv on tl.alloc, because we don't need it by the end of this func.
		kv, err := l.ToBackupPostingList(&tl.bpl, tl.alloc)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "while rolling up list")
		}

		backupKey, err := tl.toBackupKey(kv.Key)
		if err != nil {
			return nil, nil, err
		}

		// check if this key was storing a DROP operation record. If yes, get the drop operation.
		dropOp, err = checkAndGetDropOp(key, l, tl.Request.ReadTs)
		if err != nil {
			return nil, nil, err
		}

		kv.Key = backupKey
		list.Kv = append(list.Kv, kv)
	default:
		return nil, nil, errors.Errorf(
			"Unexpected meta: %d for key: %s", item.UserMeta(), hex.Dump(key))
	}
	return list, dropOp, nil
}

func (tl *threadLocal) toBackupKey(key []byte) ([]byte, error) {
	parsedKey, err := x.Parse(key)
	if err != nil {
		return nil, errors.Wrapf(err, "could not parse key %s", hex.Dump(key))
	}
	bk := parsedKey.ToBackupKey()

	out := tl.alloc.Allocate(bk.Size())
	n, err := bk.MarshalToSizedBuffer(out)
	return out[:n], err
}

func writeKVList(list *bpb.KVList, w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, uint64(list.Size())); err != nil {
		return err
	}
	buf, err := list.Marshal()
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}

func checkAndGetDropOp(key []byte, l *posting.List, readTs uint64) (*pb.DropOperation, error) {
	isDropOpKey, err := x.IsDropOpKey(key)
	if err != nil || !isDropOpKey {
		return nil, err
	}

	vals, err := l.AllValues(readTs)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot read value of dgraph.drop.op")
	}
	switch len(vals) {
	case 0:
		// do nothing, it means this one was deleted with S * * deletion.
		// So, no need to consider it.
		return nil, nil
	case 1:
		val, ok := vals[0].Value.([]byte)
		if !ok {
			return nil, errors.Errorf("cannot convert value of dgraph.drop.op to byte array, "+
				"got type: %s, value: %v, tid: %v", reflect.TypeOf(vals[0].Value), vals[0].Value,
				vals[0].Tid)
		}
		// A dgraph.drop.op record can have values in only one of the following formats:
		// * DROP_ALL;
		// * DROP_DATA;
		// * DROP_ATTR;attrName
		// So, accordingly construct the *pb.DropOperation.
		dropOp := &pb.DropOperation{}
		dropInfo := strings.Split(string(val), ";")
		if len(dropInfo) != 2 {
			return nil, errors.Errorf("Unexpected value: %s for dgraph.drop.op", val)
		}
		switch dropInfo[0] {
		case "DROP_ALL":
			dropOp.DropOp = pb.DropOperation_ALL
		case "DROP_DATA":
			dropOp.DropOp = pb.DropOperation_DATA
		case "DROP_ATTR":
			dropOp.DropOp = pb.DropOperation_ATTR
			dropOp.DropValue = dropInfo[1]
		}
		return dropOp, nil
	default:
		// getting more than one values for a non-list predicate is an error
		return nil, errors.Errorf("found multiple values for dgraph.drop.op: %v", vals)
	}
}
