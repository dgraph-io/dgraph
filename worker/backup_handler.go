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
	"fmt"
	"io"
	"net/url"

	"github.com/dgraph-io/dgraph/protos/pb"

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
	// }
	//
	// "since" is the read timestamp used at the backup request. This value is called "since"
	// because it used by subsequent incremental backups.
	// "groups" are the group IDs that participated.
	backupManifest = `manifest.json`
)

// UriHandler interface is implemented by URI scheme handlers.
// When adding new scheme handles, for example 'azure://', an object will implement
// this interface to supply Dgraph with a way to create or load backup files into DB.
// For all methods below, the URL object is parsed as described in `newHandler' and
// the Processor object has the DB, estimated tablets size, and backup parameters.
type UriHandler interface {
	// Handlers must know how to Write to their URI location.
	// These function calls are used by both Create and Load.
	io.WriteCloser

	// GetLatestManifest reads the manifests at the given URL and returns the
	// latest manifest.
	GetLatestManifest(*url.URL) (*Manifest, error)

	// CreateBackupFile prepares the object or file to save the backup file.
	CreateBackupFile(*url.URL, *pb.BackupRequest) error

	// CreateManifest prepares the manifest for writing.
	CreateManifest(*url.URL, *pb.BackupRequest) error

	// Load will scan location URI for backup files, then load them via loadFn.
	// It optionally takes the name of the last directory to consider. Any backup directories
	// created after will be ignored.
	// Objects implementing this function will be used for retrieving (dowload) backup files
	// and loading the data into a DB. The restore CLI command uses this call.
	Load(*url.URL, string, loadFn) LoadResult

	// ListManifests will scan the provided URI and return the paths to the manifests stored
	// in that location.
	ListManifests(*url.URL) ([]string, error)

	// ReadManifest will read the manifest at the given location and load it into the given
	// Manifest object.
	ReadManifest(string, *Manifest) error
}

// Credentials holds the credentials needed to perform a backup operation.
// If these credentials are missing the default credentials will be used.
type Credentials struct {
	accessKey    string
	secretKey    string
	sessionToken string
	anonymous    bool
}

func (creds *Credentials) hasCredentials() bool {
	if creds == nil {
		return false
	}
	return creds.accessKey != "" || creds.secretKey != "" || creds.sessionToken != ""
}

func (creds *Credentials) isAnonymous() bool {
	if creds == nil {
		return false
	}
	return creds.anonymous
}

// GetCredentialsFromRequest extracts the credentials from a backup request.
func GetCredentialsFromRequest(req *pb.BackupRequest) *Credentials {
	return &Credentials{
		accessKey:    req.GetAccessKey(),
		secretKey:    req.GetSecretKey(),
		sessionToken: req.GetSessionToken(),
		anonymous:    req.GetAnonymous(),
	}
}

// getHandler returns a UriHandler for the URI scheme.
func getHandler(scheme string, creds *Credentials) UriHandler {
	switch scheme {
	case "file", "":
		return &fileHandler{}
	case "minio", "s3":
		return &s3Handler{
			creds: creds,
		}
	}
	return nil
}

// NewUriHandler parses the requested URI and finds the corresponding UriHandler.
// If the passed credentials are not nil, they will be used to override the
// default credentials (only for backups to minio or S3).
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
func NewUriHandler(uri *url.URL, creds *Credentials) (UriHandler, error) {
	h := getHandler(uri.Scheme, creds)
	if h == nil {
		return nil, errors.Errorf("Unable to handle url: %s", uri)
	}

	return h, nil
}

// predicateSet is a map whose keys are predicates. It is meant to be used as a set.
type predicateSet map[string]struct{}

// loadFn is a function that will receive the current file being read.
// A reader, the backup groupId, and a map whose keys are the predicates to restore
// are passed as arguments.
type loadFn func(reader io.Reader, groupId int, preds predicateSet) (uint64, error)

// LoadBackup will scan location l for backup files in the given backup series and load them
// sequentially. Returns the maximum Since value on success, otherwise an error.
func LoadBackup(location, backupId string, fn loadFn) LoadResult {
	uri, err := url.Parse(location)
	if err != nil {
		return LoadResult{0, 0, err}
	}

	// TODO(martinmr): allow overriding credentials during restore.
	h := getHandler(uri.Scheme, nil)
	if h == nil {
		return LoadResult{0, 0, errors.Errorf("Unsupported URI: %v", uri)}
	}

	return h.Load(uri, backupId, fn)
}

// ListBackupManifests scans location l for backup files and returns the list of manifests.
func ListBackupManifests(l string) (map[string]*Manifest, error) {
	uri, err := url.Parse(l)
	if err != nil {
		return nil, err
	}

	// TODO(martinmr): allow overriding credentials while listing manifests.
	h := getHandler(uri.Scheme, nil)
	if h == nil {
		return nil, errors.Errorf("Unsupported URI: %v", uri)
	}

	paths, err := h.ListManifests(uri)
	if err != nil {
		return nil, err
	}

	listedManifests := make(map[string]*Manifest)
	for _, path := range paths {
		var m Manifest
		if err := h.ReadManifest(path, &m); err != nil {
			return nil, errors.Wrapf(err, "While reading %q", path)
		}
		listedManifests[path] = &m
	}

	return listedManifests, nil
}

// filterManifests takes a list of manifests and returns the list of manifests
// that should be considered during a restore.
func filterManifests(manifests []*Manifest, backupId string) ([]*Manifest, error) {
	// Go through the files in reverse order and stop when the latest full backup is found.
	var filteredManifests []*Manifest
	for i := len(manifests) - 1; i >= 0; i-- {
		// If backupId is not empty, skip all the manifests that do not match the given
		// backupId. If it's empty, do not skip any manifests as the default behavior is
		// to restore the latest series of backups.
		if len(backupId) > 0 && manifests[i].BackupId != backupId {
			fmt.Printf("Restore: skip manifest %s as it's not part of the series with uid %s.\n",
				manifests[i].Path, backupId)
			continue
		}

		filteredManifests = append(filteredManifests, manifests[i])
		if manifests[i].Type == "full" {
			break
		}
	}

	// Reverse the filtered lists since the original iteration happened in reverse.
	for i := len(filteredManifests)/2 - 1; i >= 0; i-- {
		opp := len(filteredManifests) - 1 - i
		filteredManifests[i], filteredManifests[opp] = filteredManifests[opp], filteredManifests[i]
	}

	if err := verifyManifests(filteredManifests); err != nil {
		return nil, err
	}

	return filteredManifests, nil
}

func verifyManifests(manifests []*Manifest) error {
	if len(manifests) == 0 {
		return nil
	}

	if manifests[0].BackupNum != 1 {
		return errors.Errorf("expected a BackupNum value of 1 for first manifest but got %d",
			manifests[0].BackupNum)
	}

	backupId := manifests[0].BackupId
	var backupNum uint64
	for _, manifest := range manifests {
		if manifest.BackupId != backupId {
			return errors.Errorf("found a manifest with backup ID %s but expected %s",
				manifest.BackupId, backupId)
		}

		backupNum++
		if manifest.BackupNum != backupNum {
			return errors.Errorf("found a manifest with backup number %d but expected %d",
				manifest.BackupNum, backupNum)
		}
	}

	return nil
}

func backupName(since uint64, groupId uint32) string {
	return fmt.Sprintf(backupNameFmt, since, groupId)
}
