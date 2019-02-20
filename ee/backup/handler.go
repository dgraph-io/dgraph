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
	"fmt"
	"io"
	"net/url"
	"path"

	"github.com/dgraph-io/dgraph/x"
)

const (
	// backupPathFmt defines the path to store or index backup objects.
	// The expected parameter is a date in string format.
	backupPathFmt = `dgraph.%s`

	// backupNameFmt defines the name of backups files or objects (remote).
	// The first parameter is the read timestamp at the time of backup. This is used for
	// incremental backups and partial restore.
	// The second parameter is the group ID when backup happened. This is used for partitioning
	// the posting directories 'p' during restore.
	backupNameFmt = `r%d-g%d.backup`

	// backupManifest is the name of backup manifests. This a JSON file that contains the
	// details of the backup. A backup dir without a manifest is ignored.
	//
	// Example manifest:
	// {
	//   "version": 2280,
	//   "groups": [ 1, 2, 3 ],
	//   "request": {
	//     "read_ts": 110001,
	//     "unix_ts": "20190220.222326",
	//     "location": "/tmp/dgraph"
	//   }
	// }
	//
	// "version" is the maximum data version, obtained from Backup() after it runs. This value
	// is used for subsequent incremental backups.
	// "groups" are the group IDs that participated.
	// "request" is the original backup request.
	backupManifest = `manifest.json`
)

// handler interface is implemented by URI scheme handlers.
// When adding new scheme handles, for example 'azure://', an object will implement
// this interface to supply Dgraph with a way to create or load backup files into DB.
type handler interface {
	// Handlers must know how to Write to their URI location.
	// These function calls are used by both Create and Load.
	io.WriteCloser

	// Create prepares the location for write operations. This function is defined for
	// creating new backup files at a location described by the URL. The caller of this
	// comes from an HTTP request.
	//
	// The URL object is parsed as described in `newHandler`.
	// The Request object has the DB, estimated tablets size, and backup parameters.
	Create(*url.URL, *object) error

	// Load will scan location URI for backup files, then load them via loadFn.
	// Objects implementing this function will use for retrieving (dowload) backup files
	// and loading the data into a DB. The restore CLI command uses this call.
	//
	// The URL object is parsed as described in `newHandler`.
	// The loadFn receives the files as they are processed by a handler, to do the actual
	// load to DB.
	Load(*url.URL, loadFn) error
}

// object describes a file object, not necessarilly a backup file.
// uri is the local or remote destination.
// path is an optional path to create at destination.
// name is the name of the file or object at uri under path.
type object struct {
	uri, path, name string
	version         uint64
}

// getHandler returns a handler for the URI scheme.
// Returns new handler on success, nil otherwise.
func getHandler(scheme string) handler {
	switch scheme {
	case "file", "":
		return &fileHandler{}
	case "s3":
		return &s3Handler{}
	}
	return nil
}

// create parses the requested target URI, finds a handler and then tries to create a session.
// Target URI formats:
//   [scheme]://[host]/[path]?[args]
//   [scheme]:///[path]?[args]
//   /[path]?[args] (only for local or NFS)
//
// Target URI parts:
//   scheme - service handler, one of: "file", "s3", "minio"
//     host - remote address. ex: "dgraph.s3.amazonaws.com"
//     path - directory, bucket or container at target. ex: "/dgraph/backups/"
//     args - specific arguments that are ok to appear in logs.
//
// Global args (if supported by the handler):
//     secure - true|false turn on/off TLS.
//      trace - true|false turn on/off HTTP tracing.
//   compress - true|false turn on/off data compression.
//    encrypt - true|false turn on/off data encryption.
//
// Examples:
//   s3://dgraph.s3.amazonaws.com/dgraph/backups?secure=true
//   minio://localhost:9000/dgraph?secure=true
//   file:///tmp/dgraph/backups
//   /tmp/dgraph/backups?compress=gzip
func create(o *object) (handler, error) {
	uri, err := url.Parse(o.uri)
	if err != nil {
		return nil, err
	}

	// find handler for this URI scheme
	h := getHandler(uri.Scheme)
	if h == nil {
		return nil, x.Errorf("Unable to handle url: %v", uri)
	}

	if err := h.Create(uri, o); err != nil {
		return nil, err
	}
	return h, nil
}

// getInfo scans the file name and returns its the read timestamp and group ID.
// If the file is not scannable, returns an error.
// Returns read timestamp and group ID, or an error otherwise.
func getInfo(file string) (uint64, int, error) {
	var readTs uint64
	var groupId int
	_, err := fmt.Sscanf(path.Base(file), backupNameFmt, &readTs, &groupId)
	return readTs, groupId, err
}

// loadFn is a function that will receive the current file being read.
// A reader and the backup readTs are passed as arguments.
type loadFn func(io.Reader, int) error

// Load will scan location l for backup files, then load them sequentially through reader.
// Returns nil on success, error otherwise.
func Load(l string, fn loadFn) error {
	uri, err := url.Parse(l)
	if err != nil {
		return err
	}

	h := getHandler(uri.Scheme)
	if h == nil {
		return x.Errorf("Unsupported URI: %v", uri)
	}

	return h.Load(uri, fn)
}
