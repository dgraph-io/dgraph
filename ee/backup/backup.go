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

package backup

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

	badger "github.com/dgraph-io/badger/v2"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

// Processor handles the different stages of the backup process.
type Processor struct {
	// DB is the Badger pstore managed by this node.
	DB *badger.DB
	// Request stores the backup request containing the parameters for this backup.
	Request *pb.BackupRequest
}

// Manifest records backup details, these are values used during restore.
// Since is the timestamp from which the next incremental backup should start (it's set
// to the readTs of the current backup).
// Groups are the IDs of the groups involved.
type Manifest struct {
	sync.Mutex
	//Type is the type of backup, either full or incremental.
	Type string `json:"type"`
	// Since is the timestamp at which this backup was taken. It's called Since
	// because it will become the timestamp from which to backup in the next
	// incremental backup.
	Since uint64 `json:"since"`
	// Groups is the map of valid groups to predicates at the time the backup was created.
	Groups map[uint32][]string `json:"groups"`
	// BackupId is a unique ID assigned to all the backups in the same series
	// (from the first full backup to the last incremental backup).
	BackupId string `json:"backup_id"`
	// BackupNum is a monotonically increasing number assigned to each backup in
	// a series. The full backup as BackupNum equal to one and each incremental
	// backup gets assigned the next available number. Used to verify the integrity
	// of the data during a restore.
	BackupNum uint64 `json:"backup_num"`
	// Path is the path to the manifest file. This field is only used during
	// processing and is not written to disk.
	Path string `json:"-"`
}

func (m *Manifest) getPredsInGroup(gid uint32) predicateSet {
	preds, ok := m.Groups[gid]
	if !ok {
		return nil
	}

	predSet := make(predicateSet)
	for _, pred := range preds {
		predSet[pred] = struct{}{}
	}
	return predSet
}

// WriteBackup uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (pr *Processor) WriteBackup(ctx context.Context) (*pb.Status, error) {
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
	gzWriter := gzip.NewWriter(handler)
	stream := pr.DB.NewStreamAt(pr.Request.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	stream.KeyToList = pr.toBackupList
	stream.ChooseKey = func(item *badger.Item) bool {
		parsedKey, err := x.Parse(item.Key())
		if err != nil {
			return false
		}
		_, ok := predMap[parsedKey.Attr]
		return ok
	}
	stream.Send = func(list *bpb.KVList) error {
		for _, kv := range list.Kv {
			if maxVersion < kv.Version {
				maxVersion = kv.Version
			}
		}
		return writeKVList(list, gzWriter)
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
func (pr *Processor) CompleteBackup(ctx context.Context, manifest *Manifest) error {
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
	return fmt.Sprintf(`Manifest{Since: %d, Groups: %v}`, m.Since, m.Groups)
}

func (pr *Processor) toBackupList(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
	list := &bpb.KVList{}

	item := itr.Item()
	if item.Version() < pr.Request.SinceTs || item.IsDeletedOrExpired() {
		// Ignore versions less than given timestamp, or skip older versions of
		// the given key by returning an empty list.
		return list, nil
	}

	switch item.UserMeta() {
	case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
		l, err := posting.ReadPostingList(key, itr)
		if err != nil {
			return nil, errors.Wrapf(err, "while reading posting list")
		}
		kvs, err := l.Rollup()
		if err != nil {
			return nil, errors.Wrapf(err, "while rolling up list")
		}

		for _, kv := range kvs {
			backupKey, err := toBackupKey(kv.Key)
			if err != nil {
				return nil, err
			}
			kv.Key = backupKey

			backupPl, err := toBackupPostingList(kv.Value)
			if err != nil {
				return nil, err
			}
			kv.Value = backupPl
		}
		list.Kv = append(list.Kv, kvs...)
	case posting.BitSchemaPosting:
		valCopy, err := item.ValueCopy(nil)
		if err != nil {
			return nil, errors.Wrapf(err, "while copying value")
		}

		backupKey, err := toBackupKey(key)
		if err != nil {
			return nil, err
		}

		kv := &bpb.KV{
			Key:       backupKey,
			Value:     valCopy,
			UserMeta:  []byte{item.UserMeta()},
			Version:   item.Version(),
			ExpiresAt: item.ExpiresAt(),
		}
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

func toBackupPostingList(val []byte) ([]byte, error) {
	pl := &pb.PostingList{}
	if err := pl.Unmarshal(val); err != nil {
		return nil, errors.Wrapf(err, "while reading posting list")
	}
	backupVal, err := posting.ToBackupPostingList(pl).Marshal()
	if err != nil {
		return nil, errors.Wrapf(err, "while converting posting list for backup")
	}
	return backupVal, nil
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
