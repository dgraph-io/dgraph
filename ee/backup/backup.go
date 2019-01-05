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
	"net/url"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

// Request has all the information needed to perform a backup.
type Request struct {
	DB     *badger.DB // Badger pstore managed by this node.
	Sizex  uint64     // approximate upload size
	Backup *pb.BackupRequest
}

// Process uses the request values to create a stream writer then hand off the data
// retrieval to stream.Orchestrate. The writer will create all the fd's needed to
// collect the data and later move to the target.
// Returns errors on failure, nil on success.
func (r *Request) Process(ctx context.Context) error {
	h, err := r.newHandler()
	if err != nil {
		glog.Errorf("Unable to get handler for request: %+v. Error: %v", r, err)
		return err
	}

	stream := r.DB.NewStreamAt(r.Backup.ReadTs)
	stream.LogPrefix = "Dgraph.Backup"
	// Take full backups for now.
	if _, err := stream.Backup(h, 0); err != nil {
		glog.Errorf("While taking backup: %v", err)
		return err
	}
	if err := h.Close(); err != nil {
		glog.Errorf("While closing handler: %v", err)
		return err
	}
	glog.Infof("Backup complete: group %d at %d", r.Backup.GroupId, r.Backup.ReadTs)
	return err
}

// newHandler parses the requested target URI, finds a handler and then tries to create a session.
// Target URI formats:
//   [scheme]://[host]/[path]?[args]
//   [scheme]:///[path]?[args]
//   /[path]?[args] (only for local or NFS)
//
// Target URI parts:
//   scheme - service handler, one of: "s3", "gs", "az", "http", "file"
//     host - remote address. ex: "dgraph.s3.amazonaws.com"
//     path - directory, bucket or container at target. ex: "/dgraph/backups/"
//     args - specific arguments that are ok to appear in logs.
//
// Global args (might not be support by all handlers):
//     secure - true|false turn on/off TLS.
//   compress - true|false turn on/off data compression.
//
// Examples:
//   s3://dgraph.s3.amazonaws.com/dgraph/backups?secure=true
//   gs://dgraph/backups/
//   as://dgraph-container/backups/
//   http://backups.dgraph.io/upload
//   file:///tmp/dgraph/backups or /tmp/dgraph/backups?compress=gzip
func (r *Request) newHandler() (handler, error) {
	uri, err := url.Parse(r.Backup.Location)
	if err != nil {
		return nil, err
	}

	// find handler for this URI scheme
	h := getHandler(uri.Scheme)
	if h == nil {
		return nil, x.Errorf("Unable to handle url: %v", uri)
	}

	if err := h.Create(uri, r); err != nil {
		return nil, err
	}
	return h, nil
}
