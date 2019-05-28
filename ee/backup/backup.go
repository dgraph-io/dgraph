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
	"github.com/golang/glog"
)

// Request has all the information needed to perform a backup.
type Request struct {
	DB       *badger.DB // Badger pstore managed by this node.
	Backup   *pb.BackupRequest
	Manifest *Manifest

	// Since indicates the beginning timestamp from which the backup should start.
	// For a partial backup, the value is the largest value from the previous manifest
	// files. For a full backup, Since is set to zero so that all data is included.
	Since uint64
}

// Process uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (r *Request) Process(ctx context.Context) (*pb.BackupResponse, error) {
	var res pb.BackupResponse

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	handler, err := r.newHandler()
	if err != nil {
		return nil, err
	}
	glog.V(3).Infof("Backup manifest version: %d", r.Since)

	stream := r.DB.NewStreamAt(r.Backup.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	res.Since, err = stream.Backup(handler, r.Since)
	if err != nil {
		glog.Errorf("While taking backup: %v", err)
		return nil, err
	}
	glog.V(2).Infof("Backup group %d version: %d", r.Backup.GroupId, res.Since)
	if err = handler.Close(); err != nil {
		glog.Errorf("While closing handler: %v", err)
		return nil, err
	}
	glog.Infof("Backup complete: group %d at %d", r.Backup.GroupId, r.Backup.ReadTs)
	return &res, nil
}

// Manifest records backup details, these are values used during restore.
// Since is the timestamp from which the next incremental backup should start.
// Groups are the IDs of the groups involved.
// ReadTs is the original backup request timestamp.
type Manifest struct {
	sync.Mutex
	Since  uint64   `json:"since"`
	ReadTs uint64   `json:"read_ts"`
	Groups []uint32 `json:"groups"`
}

// ManifestStatus combines a manifest along with other information about it
// that should not be inside the Manifest struct since it should not be
// recorded in manifest files.
type ManifestStatus struct {
	*Manifest
	FileName string
}

// GoString implements the GoStringer interface for Manifest.
func (m *Manifest) GoString() string {
	return fmt.Sprintf(`Manifest{Since: %d, ReadTs: %d, Groups: %v}`,
		m.Since, m.ReadTs, m.Groups)
}

// Complete will finalize a backup by writing the manifest at the backup destination.
func (r *Request) Complete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	handler, err := r.newHandler()
	if err != nil {
		return err
	}
	if r.Manifest.ReadTs == 0 {
		r.Manifest.ReadTs = r.Backup.ReadTs
	}
	if err = json.NewEncoder(handler).Encode(r.Manifest); err != nil {
		return err
	}
	if err = handler.Close(); err != nil {
		return err
	}
	glog.Infof("Backup completed OK.")
	return nil
}
