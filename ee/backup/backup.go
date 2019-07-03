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
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"

	"github.com/dgraph-io/badger"
	bpb "github.com/dgraph-io/badger/pb"
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
	// Groups is the list of valid groups at the time the backup was created.
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

	handler, err := NewUriHandler(uri)
	if err != nil {
		return &emptyRes, err
	}

	if err := handler.CreateBackupFile(uri, pr.Request); err != nil {
		return &emptyRes, err
	}

	glog.V(3).Infof("Backup manifest version: %d", pr.Request.SinceTs)

	stream := pr.DB.NewStreamAt(pr.Request.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	stream.KeyToList = toBackupList
	gzWriter := gzip.NewWriter(handler)
	newSince, err := stream.Backup(gzWriter, pr.Request.SinceTs)

	if err != nil {
		glog.Errorf("While taking backup: %v", err)
		return &emptyRes, err
	}

	if newSince > pr.Request.ReadTs {
		glog.Errorf("Max timestamp seen during backup (%d) is greater than readTs (%d)",
			newSince, pr.Request.ReadTs)
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

	handler, err := NewUriHandler(uri)
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

// toBackupList replaces the default KeyToList implementation so that the keys and
// values are written in the backup format.
func toBackupList(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
	list := &bpb.KVList{}
	for ; itr.Valid(); itr.Next() {
		item := itr.Item()
		if item.IsDeletedOrExpired() {
			break
		}
		if !bytes.Equal(key, item.Key()) {
			// Break out on the first encounter with another key.
			break
		}

		keyCopy := item.KeyCopy(nil)
		valCopy, err := item.ValueCopy(nil)
		if err != nil {
			panic("wtf")
			return nil, err
		}
		var backupKey []byte
		var backupVal []byte

		switch item.UserMeta() {
		case posting.BitEmptyPosting, posting.BitCompletePosting, posting.BitDeltaPosting:
			var err error
			parsedKey := x.Parse(keyCopy)
			if parsedKey == nil {
				return nil, errors.Errorf("could not parse key %s", hex.Dump(keyCopy))
			}
			backupKey, err = parsedKey.ToBackupKey().Marshal()
			if err != nil {
				return nil, errors.Wrapf(err, "while converting key for backup")
			}

			pl := &pb.PostingList{}
			if err := pl.Unmarshal(valCopy); err != nil {
				return nil, errors.Wrapf(err, "while reading posting list")
			}
			backupVal, err = posting.ToBackupPostingList(pl).Marshal()
			if err != nil {
				return nil, errors.Wrapf(err, "while converting posting list for backup")
			}

		case posting.BitSchemaPosting:
			backupKey = keyCopy
			backupVal = valCopy

		default:
			return nil, errors.Errorf(
				"Unexpected meta: %d for key: %s", item.UserMeta(), hex.Dump(key))
		}

		fmt.Printf("backup key at backup point: %s", hex.Dump(backupKey))
		kv := &bpb.KV{
			Key:       backupKey,
			Value:     backupVal,
			UserMeta:  []byte{item.UserMeta()},
			Version:   item.Version(),
			ExpiresAt: item.ExpiresAt(),
		}
		list.Kv = append(list.Kv, kv)

		if item.DiscardEarlierVersions() {
			break
		}
	}
	return list, nil
}
