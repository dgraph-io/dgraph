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
	"sync"

	"github.com/dgraph-io/badger/v2"
	bpb "github.com/dgraph-io/badger/v2/pb"
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

	// plList is an array of pre-allocated pb.PostingList objects.
	plList []*pb.PostingList
	// bplList is an array of pre-allocated pb.BackupPostingList objects.
	bplList []*pb.BackupPostingList
	// kvPool
	kvPool *sync.Pool
}

func NewBackupProcessor(db *badger.DB, req *pb.BackupRequest) *BackupProcessor {
	bp := &BackupProcessor{
		DB:      db,
		Request: req,
		plList:  make([]*pb.PostingList, backupNumGo),
		bplList: make([]*pb.BackupPostingList, backupNumGo),
		kvPool: &sync.Pool{
			New: func() interface{} {
				return &bpb.KV{}
			},
		},
	}

	for i := range bp.plList {
		bp.plList[i] = &pb.PostingList{}
	}
	for i := range bp.bplList {
		bp.bplList[i] = &pb.BackupPostingList{}
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

// WriteBackup uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (pr *BackupProcessor) WriteBackup(ctx context.Context) (*pb.Status, error) {
	var emptyRes pb.Status

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	uri, err := url.Parse(pr.Request.Destination)
	if err != nil {
		return &emptyRes, err
	}

	handler, err := NewUriHandler(uri, GetCredentialsFromRequest(pr.Request))
	if err != nil {
		return &emptyRes, err
	}

	if err := handler.CreateBackupFile(uri, pr.Request); err != nil {
		return &emptyRes, err
	}

	glog.V(3).Infof("Backup manifest version: %d", pr.Request.SinceTs)

	predMap := make(map[string]struct{})
	for _, pred := range pr.Request.Predicates {
		predMap[pred] = struct{}{}
	}

	var maxVersion uint64

	newhandler, err := enc.GetWriter(x.WorkerConfig.EncryptionKey, handler)
	if err != nil {
		return &emptyRes, err
	}
	gzWriter := gzip.NewWriter(newhandler)

	stream := pr.DB.NewStreamAt(pr.Request.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	stream.NumGo = backupNumGo
	stream.KeyToList = pr.toBackupList
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

		// Backup type keys in every group.
		if parsedKey.IsType() {
			return true
		}

		// Only backup schema and data keys for the requested predicates.
		_, ok := predMap[parsedKey.Attr]
		return ok
	}
	stream.Send = func(list *bpb.KVList) error {
		for _, kv := range list.Kv {
			if maxVersion < kv.Version {
				maxVersion = kv.Version
			}
		}
		err := writeKVList(list, gzWriter)

		for _, kv := range list.Kv {
			pr.kvPool.Put(kv)
		}
		return err
	}

	if err := stream.Orchestrate(context.Background()); err != nil {
		glog.Errorf("While taking backup: %v", err)
		return &emptyRes, err
	}

	if maxVersion > pr.Request.ReadTs {
		glog.Errorf("Max timestamp seen during backup (%d) is greater than readTs (%d)",
			maxVersion, pr.Request.ReadTs)
	}

	glog.V(2).Infof("Backup group %d version: %d", pr.Request.GroupId, pr.Request.ReadTs)
	if err = gzWriter.Close(); err != nil {
		glog.Errorf("While closing gzipped writer: %v", err)
		return &emptyRes, err
	}

	if err = handler.Close(); err != nil {
		glog.Errorf("While closing handler: %v", err)
		return &emptyRes, err
	}
	glog.Infof("Backup complete: group %d at %d", pr.Request.GroupId, pr.Request.ReadTs)
	return &emptyRes, nil
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

func (pr *BackupProcessor) toBackupList(key []byte, itr *badger.Iterator) (
	*bpb.KVList, error) {
	list := &bpb.KVList{}

	item := itr.Item()
	if item.UserMeta() != posting.BitSchemaPosting && (item.Version() < pr.Request.SinceTs ||
		item.IsDeletedOrExpired()) {
		// Ignore versions less than given timestamp, or skip older versions of
		// the given key by returning an empty list.
		// Do not do this for schema and type keys. Those keys always have a
		// version of one so they would be incorrectly rejected by above check.
		return list, nil
	}

	kv := pr.kvPool.Get().(*bpb.KV)
	kv.Reset()

	switch item.UserMeta() {
	case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, errors.Wrapf(err, "while reading posting list")
		}

		err = l.SingleListRollup(kv)
		if err != nil {
			return nil, errors.Wrapf(err, "while rolling up list")
		}

		backupKey, err := toBackupKey(kv.Key)
		if err != nil {
			return nil, err
		}
		kv.Key = backupKey

		backupPl, err := pr.toBackupPostingList(kv.Value, itr.ThreadId)
		if err != nil {
			return nil, err
		}
		kv.Value = backupPl
		list.Kv = append(list.Kv, kv)
	case posting.BitSchemaPosting:
		valCopy, err := item.ValueCopy(nil)
		if err != nil {
			return nil, errors.Wrapf(err, "while copying value")
		}

		backupKey, err := toBackupKey(key)
		if err != nil {
			return nil, err
		}

		kv.Key = backupKey
		kv.Value = valCopy
		kv.UserMeta = []byte{item.UserMeta()}
		kv.Version = item.Version()
		kv.ExpiresAt = item.ExpiresAt()
		list.Kv = append(list.Kv, kv)
	default:
		return nil, errors.Errorf(
			"Unexpected meta: %d for key: %s", item.UserMeta(), hex.Dump(key))
	}
	return list, nil
}

func toBackupKey(key []byte) ([]byte, error) {
	parsedKey, err := x.Parse(key)
	if err != nil {
		return nil, errors.Wrapf(err, "could not parse key %s", hex.Dump(key))
	}
	backupKey, err := parsedKey.ToBackupKey().Marshal()
	if err != nil {
		return nil, errors.Wrapf(err, "while converting key for backup")
	}
	return backupKey, nil
}

func (pr *BackupProcessor) toBackupPostingList(val []byte, threadNum int) ([]byte, error) {
	pl := pr.plList[threadNum]
	bpl := pr.bplList[threadNum]
	pl.Reset()
	bpl.Reset()

	if err := pl.Unmarshal(val); err != nil {
		return nil, errors.Wrapf(err, "while reading posting list")
	}

	return posting.ToBackupPostingList(pl, bpl)
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
