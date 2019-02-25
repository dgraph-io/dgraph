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

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

// Process uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func Process(ctx context.Context, db *badger.DB, req *pb.BackupRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	obj := &object{
		uri:  req.Location,
		path: fmt.Sprintf(backupPathFmt, req.UnixTs),
		name: fmt.Sprintf(backupNameFmt, req.ReadTs, req.GroupId),
	}
	handler, err := create(obj)
	if err != nil {
		glog.Errorf("Unable to get handler for request: %+v. Error: %v", req, err)
		return err
	}

	stream := db.NewStreamAt(req.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	// Here we return the max version in the original request obejct. We will use this
	// to create our manifest to complete the backup.
	req.Since, err = stream.Backup(handler, obj.version)
	if err != nil {
		glog.Errorf("While taking backup: %v", err)
		return err
	}
	glog.V(3).Infof("Backup maximum version: %d", req.Since)
	if err = handler.Close(); err != nil {
		glog.Errorf("While closing handler: %v", err)
		return err
	}
	glog.Infof("Backup complete: group %d at %d", req.GroupId, req.ReadTs)
	return nil
}

// Manifest records backup details, these are values used during restore.
// Version is the maximum version seen.
// Groups are the IDs of the groups involved.
// Request is the original backup request.
type Manifest struct {
	Version uint64           `json:"version"`
	Groups  []uint32         `json:"groups"`
	Request pb.BackupRequest `json:"request"`
}

// Complete will finalize a backup by writing the manifest at the backup destination.
func (m *Manifest) Complete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.Version == 0 {
		return x.Errorf("Backup manifest version is zero")
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
