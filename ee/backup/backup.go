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
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

// ErrBackupNoChanges is returned when the manifest version is equal to the snapshot version.
// This means that no data updates happened since the last backup.
var ErrBackupNoChanges = x.Errorf("No changes since last backup, OK.")

// Request has all the information needed to perform a backup.
type Request struct {
	DB     *badger.DB // Badger pstore managed by this node.
	Backup *pb.BackupRequest
}

// Process uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (r *Request) Process(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	obj := &object{
		uri:    r.Backup.Location,
		path:   fmt.Sprintf(backupPathFmt, r.Backup.UnixTs),
		name:   fmt.Sprintf(backupNameFmt, r.Backup.ReadTs, r.Backup.GroupId),
		snapTs: r.Backup.SnapshotTs, // used to verify version changes
	}
	handler, err := create(obj)
	if err != nil {
		if err != ErrBackupNoChanges {
			glog.Errorf("Unable to get handler for request: %+v. Error: %v", r.Backup, err)
		}
		return err
	}
	glog.V(3).Infof("Backup manifest version: %d", obj.version)

	stream := r.DB.NewStreamAt(r.Backup.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	// Here we return the max version in the original request obejct. We will use this
	// to create our manifest to complete the backup.
	r.Backup.Since, err = stream.Backup(handler, obj.version)
	if err != nil {
		glog.Errorf("While taking backup: %v", err)
		return err
	}
	glog.V(3).Infof("Backup group %d version: %d", r.Backup.GroupId, r.Backup.Since)
	if err = handler.Close(); err != nil {
		glog.Errorf("While closing handler: %v", err)
		return err
	}
	glog.Infof("Backup complete: group %d at %d", r.Backup.GroupId, r.Backup.ReadTs)
	return nil
}

// Manifest records backup details, these are values used during restore.
// Version is the maximum version seen.
// Groups are the IDs of the groups involved.
// Request is the original backup request.
type Manifest struct {
	sync.Mutex
	Version uint64           `json:"version"`
	Groups  []uint32         `json:"groups"`
	Request pb.BackupRequest `json:"request"`
}

// Complete will finalize a backup by writing the manifest at the backup destination.
func (m *Manifest) Complete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// None of the groups produced backup files.
	if m.Version == 0 {
		glog.Infof("%s", ErrBackupNoChanges)
		return nil
	}
	handler, err := create(&object{
		uri:     m.Request.Location,
		path:    fmt.Sprintf(backupPathFmt, m.Request.UnixTs),
		name:    backupManifest,
		version: m.Version,
	})
	if err != nil {
		return err
	}
	if err = json.NewEncoder(handler).Encode(&m); err != nil {
		return err
	}
	if err = handler.Close(); err != nil {
		return err
	}
	glog.Infof("Backup completed OK.")
	return nil
}
