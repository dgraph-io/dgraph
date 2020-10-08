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
	"sort"

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

	// GetManifests returns the list of manfiests for the given backup series ID
	// and backup number at the specified location. If backupNum is set to zero,
	// all the manifests for the backup series will be returned. If it's greater
	// than zero, manifests from one to backupNum will be returned.
	GetManifests(*url.URL, string, uint64) ([]*Manifest, error)

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
	Load(*url.URL, string, uint64, loadFn) LoadResult

	// Verify checks that the specified backup can be restored to a cluster with the
	// given groups. The last manifest of that backup should have the same number of
	// groups as given list of groups.
	Verify(*url.URL, *pb.RestoreRequest, []uint32) error

	// ListManifests will scan the provided URI and return the paths to the manifests stored
	// in that location.
	ListManifests(*url.URL) ([]string, error)

	// ReadManifest will read the manifest at the given location and load it into the given
	// Manifest object.
	ReadManifest(string, *Manifest) error
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

// loadFn is a function that will receive the current file being read.
// A reader, the backup groupId, and a map whose keys are the predicates to restore
// are passed as arguments.
type loadFn func(reader io.Reader, groupId uint32, preds predicateSet) (uint64, error)

// LoadBackup will scan location l for backup files in the given backup series and load them
// sequentially. Returns the maximum Since value on success, otherwise an error.
func LoadBackup(location, backupId string, backupNum uint64, creds *Credentials,
	fn loadFn) LoadResult {
	uri, err := url.Parse(location)
	if err != nil {
		return LoadResult{0, 0, err}
	}

	h := getHandler(uri.Scheme, creds)
	if h == nil {
		return LoadResult{0, 0, errors.Errorf("Unsupported URI: %v", uri)}
	}

	return h.Load(uri, backupId, backupNum, fn)
}

// VerifyBackup will access the backup location and verify that the specified backup can
// be restored to the cluster.
func VerifyBackup(req *pb.RestoreRequest, creds *Credentials, currentGroups []uint32) error {
	uri, err := url.Parse(req.GetLocation())
	if err != nil {
		return err
	}

	h := getHandler(uri.Scheme, creds)
	if h == nil {
		return errors.Errorf("Unsupported URI: %v", uri)
	}

	return h.Verify(uri, req, currentGroups)
}

// ListBackupManifests scans location l for backup files and returns the list of manifests.
func ListBackupManifests(l string, creds *Credentials) (map[string]*Manifest, error) {
	uri, err := url.Parse(l)
	if err != nil {
		return nil, err
	}

	h := getHandler(uri.Scheme, creds)
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
		m.Path = path
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

// verifyRequest verifies the manifests satisfy the requirements to process the given
// restore request.
func verifyRequest(req *pb.RestoreRequest, manifests []*Manifest, currentGroups []uint32) error {
	if len(manifests) == 0 {
		return errors.Errorf("No backups with the specified backup ID %s", req.GetBackupId())
	}

	if err := verifyManifests(manifests); err != nil {
		return err
	}

	lastManifest := manifests[len(manifests)-1]
	if len(currentGroups) != len(lastManifest.Groups) {
		return errors.Errorf("groups in cluster and latest backup manifest differ")
	}

	for _, group := range currentGroups {
		if _, ok := lastManifest.Groups[group]; !ok {
			return errors.Errorf("groups in cluster and latest backup manifest differ")
		}
	}
	return nil
}

func getManifests(manifests []*Manifest, backupId string,
	backupNum uint64) ([]*Manifest, error) {

	manifests, err := filterManifests(manifests, backupId)
	if err != nil {
		return nil, err
	}

	// Sort manifests in the ascending order of their BackupNum so that the first
	// manifest corresponds to the first full backup and so on.
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].BackupNum < manifests[j].BackupNum
	})

	if backupNum > 0 {
		if len(manifests) < int(backupNum) {
			return nil, errors.Errorf("not enough backups to restore manifest with backupNum %d",
				backupNum)
		}
		manifests = manifests[:backupNum]
	}
	return manifests, nil
}
