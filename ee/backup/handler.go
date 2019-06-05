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
	"io"
	"net/url"

	"github.com/pkg/errors"
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
	//   "since": 2280,
	//   "groups": [ 1, 2, 3 ],
	//   "read_ts": 110001
	// }
	//
	// "since" is the maximum data version, obtained from Backup() after it runs. This value
	// is used for subsequent incremental backups.
	// "groups" are the group IDs that participated.
	// "read_ts" is the read timestamp used at the backup request.
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
	Create(*url.URL, *Request) error

	// Load will scan location URI for backup files, then load them via loadFn.
	// Objects implementing this function will be used for retrieving (dowload) backup files
	// and loading the data into a DB. The restore CLI command uses this call.
	//
	// The URL object is parsed as described in `newHandler`.
	// The loadFn receives the files as they are processed by a handler, to do the actual
	// load to DB.
	Load(*url.URL, loadFn) (uint64, error)

	// ListManifests will scan the provided URI and return the paths to the manifests stored
	// in that location.
	//
	// The URL object is parsed as described in `newHandler`.
	ListManifests(*url.URL) ([]string, error)

	// ReadManifest will read the manifest at the given location and load it into the given
	// Manifest object.
	ReadManifest(string, *Manifest) error
}

// getHandler returns a handler for the URI scheme.
// Returns new handler on success, nil otherwise.
func getHandler(scheme string) handler {
	switch scheme {
	case "file", "":
		return &fileHandler{}
	case "minio", "s3":
		return &s3Handler{}
	}
	return nil
}

// newHandler parses the requested URI, finds a handler and then tries to create a session.
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
func (r *Request) newHandler() (handler, error) {
	var h handler

	uri, err := url.Parse(r.Backup.Destination)
	if err != nil {
		return nil, err
	}

	h = getHandler(uri.Scheme)
	if h == nil {
		return nil, errors.Errorf("Unable to handle url: %s", uri)
	}

	if err = h.Create(uri, r); err != nil {
		return nil, err
	}
	return h, nil
}

// loadFn is a function that will receive the current file being read.
// A reader and the backup groupId are passed as arguments.
type loadFn func(reader io.Reader, groupId int) error

// Load will scan location l for backup files, then load them sequentially through reader.
// Returns the maximum Since value on success, otherwise an error.
func Load(l string, fn loadFn) (since uint64, err error) {
	uri, err := url.Parse(l)
	if err != nil {
		return 0, err
	}

	h := getHandler(uri.Scheme)
	if h == nil {
		return 0, errors.Errorf("Unsupported URI: %v", uri)
	}

	return h.Load(uri, fn)
}

// ListManifests scans location l for backup files and returns the list of manifests.
func ListManifests(l string) ([]*ManifestStatus, error) {
	uri, err := url.Parse(l)
	if err != nil {
		return nil, err
	}

	h := getHandler(uri.Scheme)
	if h == nil {
		return nil, errors.Errorf("Unsupported URI: %v", uri)
	}

	paths, err := h.ListManifests(uri)
	if err != nil {
		return nil, err
	}

	var listedManifests []*ManifestStatus
	for _, path := range paths {
		var m Manifest
		var ms ManifestStatus

		if err := h.ReadManifest(path, &m); err != nil {
			return nil, errors.Wrapf(err, "While reading %q", path)
		}
		ms.Manifest = &m
		ms.FileName = path
		listedManifests = append(listedManifests, &ms)
	}

	return listedManifests, nil
}
