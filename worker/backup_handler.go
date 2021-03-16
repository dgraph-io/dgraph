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
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/minio/minio-go/v6"
	"github.com/minio/minio-go/v6/pkg/credentials"

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

	tmpManifest = `manifest_tmp.json`
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

	// TODO: This is what we need
	// GetFiles(*url.URL, "MANIFEST") // sort of like file.Walk in Go.
	//
	// Or, ideally, just GetFile(...)
	// CreateFile(...)
	// Stream(*url.URL)

	PathJoin(left, right string) string
	Exists(path string) bool
	Read(path string) ([]byte, error)
	// Stream would stream the path via an instance of io.ReadCloser. Close must be called at the
	// end to release resources appropriately.
	Stream(path string) (io.ReadCloser, error)

	// GetManifest returns the master manifest, containing information about all the
	// backups. If the backup directory is using old formats (version < 21.03) of manifests,
	// then it will return a consolidated master manifest.
	// GetManifest(*url.URL) (*MasterManifest, error)

	// // GetManifests returns the list of manifest for the given backup series ID
	// // and backup number at the specified location. If backupNum is set to zero,
	// // all the manifests for the backup series will be returned. If it's greater
	// // than zero, manifests from one to backupNum will be returned.
	// GetManifests(*url.URL, string, uint64) ([]*Manifest, error)

	// // GetLatestManifest reads the manifests at the given URL and returns the
	// // latest manifest.
	// GetLatestManifest(*url.URL) (*Manifest, error)

	// // CreateBackupFile prepares the object or file to save the backup file.
	// CreateBackupFile(*url.URL, *pb.BackupRequest) error

	// // CreateManifest creates the given manifest.
	// CreateManifest(*url.URL, *MasterManifest) error

	// // Load will scan location URI for backup files, then load them via loadFn.
	// // It optionally takes the name of the last directory to consider. Any backup directories
	// // created after will be ignored.
	// // Objects implementing this function will be used for retrieving (dowload) backup files
	// // and loading the data into a DB. The restore CLI command uses this call.
	// Load(*url.URL, string, uint64, loadFn) LoadResult

	// // Verify checks that the specified backup can be restored to a cluster with the
	// // given groups. The last manifest of that backup should have the same number of
	// // groups as given list of groups.
	// Verify(*url.URL, *pb.RestoreRequest, []uint32) error
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
func NewUriHandler(uri *url.URL, creds *x.MinioCredentials) (UriHandler, error) {
	switch uri.Scheme {
	case "file", "":
		return &fileHandler{}, nil
	case "minio", "s3":
		return NewS3Handler(uri, creds)
	}
	return nil, errors.Errorf("Unable to handle url: %s", uri)

}

// loadFn is a function that will receive the current file being read.
// A reader, the backup groupId, and a map whose keys are the predicates to restore
// are passed as arguments.
type loadFn func(groupId uint32, in *loadBackupInput) (uint64, uint64, error)

// LoadBackup will scan location l for backup files in the given backup series and load them
// sequentially. Returns the maximum Since value on success, otherwise an error.
func LoadBackup(location, backupId string, backupNum uint64, creds *x.MinioCredentials,
	fn loadFn) LoadResult {
	uri, err := url.Parse(location)
	if err != nil {
		return LoadResult{Err: err}
	}

	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return LoadResult{Err: errors.Errorf("Unsupported URI: %v", uri)}
	}

	return h.Load(uri, backupId, backupNum, fn)
}

// VerifyBackup will access the backup location and verify that the specified backup can
// be restored to the cluster.
func VerifyBackup(req *pb.RestoreRequest, creds *x.MinioCredentials, currentGroups []uint32) error {
	uri, err := url.Parse(req.GetLocation())
	if err != nil {
		return err
	}

	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return errors.Wrap(err, "VerifyBackup")
	}

	return h.Verify(uri, req, currentGroups)
}

// ListBackupManifests scans location l for backup files and returns the list of manifests.
func ListBackupManifests(l string, creds *x.MinioCredentials) ([]*Manifest, error) {
	uri, err := url.Parse(l)
	if err != nil {
		return nil, err
	}

	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return nil, errors.Wrap(err, "ListBackupManifests")
	}

	m, err := h.GetManifest(uri)
	if err != nil {
		return nil, err
	}
	return m.Manifests, nil
}

func GetManifest(h *UriHandler, uri *url.URL) error {
	if !h.Exists(uri.Path) {
		return fmt.Errorf("uri: %+v not found", uri)
	}
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

// S3 Handler.

// FillRestoreCredentials fills the empty values with the default credentials so that
// a restore request is sent to all the groups with the same credentials.
func FillRestoreCredentials(location string, req *pb.RestoreRequest) error {
	uri, err := url.Parse(location)
	if err != nil {
		return err
	}

	defaultCreds := credentials.Value{
		AccessKeyID:     req.AccessKey,
		SecretAccessKey: req.SecretKey,
		SessionToken:    req.SessionToken,
	}
	provider := x.MinioCredentialsProvider(uri.Scheme, defaultCreds)

	creds, _ := provider.Retrieve() // Error is always nil.

	req.AccessKey = creds.AccessKeyID
	req.SecretKey = creds.SecretAccessKey
	req.SessionToken = creds.SessionToken

	return nil
}

// s3Handler is used for 's3:' and 'minio:' URI schemes.
type s3Handler struct {
	bucketName, objectPrefix string
	pwriter                  *io.PipeWriter
	preader                  *io.PipeReader
	cerr                     chan error
	creds                    *x.MinioCredentials
	uri                      *url.URL
	mc                       *x.MinioClient
}

// setup creates a new session, checks valid bucket at uri.Path, and configures a minio client.
// setup also fills in values used by the handler in subsequent calls.
// Returns a new S3 minio client, otherwise a nil client with an error.
func NewS3Handler(uri *url.URL, creds *x.MinioCredentials) (*s3Handler, error) {
	h := &s3Handler{
		creds: creds,
		uri:   uri,
	}
	mc, err := x.NewMinioClient(uri, creds)
	if err != nil {
		return nil, err
	}
	h.mc = mc
	h.bucketName, h.objectPrefix = mc.ParseBucketAndPrefix(uri.Path)
	return h, nil
}

func (h *s3Handler) createObject(mc *x.MinioClient, objectPath string) {

	// The backup object is: folder1...folderN/dgraph.20181106.0113/r110001-g1.backup
	object := filepath.Join(h.objectPrefix, objectPath)
	glog.V(2).Infof("Sending data to %s blob %q ...", h.uri.Scheme, object)

	h.cerr = make(chan error, 1)
	h.preader, h.pwriter = io.Pipe()
	go func() {
		h.cerr <- h.upload(mc, object)
	}()
}

// GetLatestManifest reads the manifests at the given URL and returns the
// latest manifest.
func (h *s3Handler) GetLatestManifest(uri *url.URL) (*Manifest, error) {
	manifest, err := h.getConsolidatedManifest()
	if err != nil {
		errors.Wrap(err, "GetLatestManifest failed to get consolidated manifests: ")
	}
	if len(manifest.Manifests) == 0 {
		return &Manifest{}, nil
	}
	return manifest.Manifests[len(manifest.Manifests)-1], nil
}

// CreateBackupFile creates a new session and prepares the data stream for the backup.
// URI formats:
//   minio://<host>/bucket/folder1.../folderN?secure=true|false
//   minio://<host:port>/bucket/folder1.../folderN?secure=true|false
//   s3://<s3 region endpoint>/bucket/folder1.../folderN?secure=true|false
//   s3:///bucket/folder1.../folderN?secure=true|false (use default S3 endpoint)
func (h *s3Handler) CreateBackupFile(uri *url.URL, req *pb.BackupRequest) error {
	glog.V(2).Infof("S3Handler got uri: %+v. Host: %s. Path: %s\n", uri, uri.Host, uri.Path)

	fileName := backupName(req.ReadTs, req.GroupId)
	objectPath := filepath.Join(fmt.Sprintf(backupPathFmt, req.UnixTs), fileName)
	h.createObject(h.mc, objectPath)
	return nil
}

// CreateManifest finishes a backup by creating an object to store the manifest.
func (h *s3Handler) CreateManifest(uri *url.URL, manifest *MasterManifest) error {
	glog.V(2).Infof("S3Handler got uri: %+v. Host: %s. Path: %s\n", uri, uri.Host, uri.Path)

	// If there is already a consolidated manifest, write the manifest to a temp file, which
	// will be used to replace original manifest.
	objectPath := filepath.Join(h.objectPrefix, backupManifest)
	if h.objectExists(objectPath) {
		h.createObject(h.mc, tmpManifest)
		if err := json.NewEncoder(h).Encode(manifest); err != nil {
			return err
		}
		if err := h.flush(); err != nil {
			return errors.Wrap(err, "CreateManifest failed to flush the handler")
		}

		// At this point, a temporary manifest is successfully created, we need to replace the
		// original manifest with this temporary manifest.
		object := filepath.Join(h.objectPrefix, backupManifest)
		tmpObject := filepath.Join(h.objectPrefix, tmpManifest)
		src := minio.NewSourceInfo(h.bucketName, tmpObject, nil)
		dst, err := minio.NewDestinationInfo(h.bucketName, object, nil, nil)
		if err != nil {
			return errors.Wrap(err, "CreateManifest failed to create dstInfo")
		}

		// We try copying 100 times, if it still fails, the user should manually copy the
		// tmpManifest to the original manifest.
		err = x.RetryUntilSuccess(100, time.Second, func() error {
			if err := h.mc.CopyObject(dst, src); err != nil {
				return errors.Wrapf(err, "COPYING TEMPORARY MANIFEST TO MAIN MANIFEST FAILED!!!\n"+
					"It is possible that the manifest would have been corrupted. You must copy "+
					"the file: %s to: %s (present in the backup s3 bucket),  in order to "+
					"fix the backup manifest.", tmpManifest, backupManifest)
			}
			return nil
		})
		if err != nil {
			return err
		}

		err = h.mc.RemoveObject(h.bucketName, tmpObject)
		return errors.Wrap(err, "CreateManifest failed to remove temporary manifest")
	}
	h.createObject(h.mc, backupManifest)
	err := json.NewEncoder(h).Encode(manifest)
	return errors.Wrap(err, "CreateManifest failed to create a new master manifest")
}

// GetManifest returns the master manifest, if the directory doesn't contain
// a master manifest, then it will try to return a master manifest by consolidating
// the manifests.
func (h *s3Handler) GetManifest(uri *url.URL) (*MasterManifest, error) {
	manifest, err := h.getConsolidatedManifest()
	if err != nil {
		return manifest, errors.Wrap(err, "GetManifest failed to get consolidated manifests: ")
	}
	return manifest, nil

}

func (h *s3Handler) GetManifests(uri *url.URL, backupId string,
	backupNum uint64) ([]*Manifest, error) {
	manifest, err := h.getConsolidatedManifest()
	if err != nil {
		return manifest.Manifests, errors.Wrap(err, "GetManifest failed to get consolidated manifests: ")
	}

	var filtered []*Manifest
	for _, m := range manifest.Manifests {
		path := filepath.Join(uri.Path, m.Path)
		if h.objectExists(path) {
			filtered = append(filtered, m)
		}
	}
	return getManifests(manifest.Manifests, backupId, backupNum)
}

// Load creates a new session, scans for backup objects in a bucket, then tries to
// load any backup objects found.
// Returns nil and the maximum Since value on success, error otherwise.
func (h *s3Handler) Load(uri *url.URL, backupId string, backupNum uint64, fn loadFn) LoadResult {
	manifests, err := h.GetManifests(uri, backupId, backupNum)
	if err != nil {
		return LoadResult{Err: errors.Wrapf(err, "while retrieving manifests")}
	}
	// since is returned with the max manifest Since value found.
	var since uint64

	// Process each manifest, first check that they are valid and then confirm the
	// backup manifests for each group exist. Each group in manifest must have a backup file,
	// otherwise this is a failure and the user must remedy.
	var maxUid, maxNsId uint64
	for i, manifest := range manifests {
		if manifest.Since == 0 || len(manifest.Groups) == 0 {
			continue
		}

		path := manifests[i].Path
		for gid := range manifest.Groups {
			object := filepath.Join(path, backupName(manifest.Since, gid))
			reader, err := h.mc.GetObject(h.bucketName, object, minio.GetObjectOptions{})
			if err != nil {
				return LoadResult{Err: errors.Wrapf(err, "Failed to get %q", object)}
			}
			defer reader.Close()

			st, err := reader.Stat()
			if err != nil {
				return LoadResult{Err: errors.Wrapf(err, "Stat failed %q", object)}
			}
			if st.Size <= 0 {
				return LoadResult{Err: errors.Errorf("Remote object is empty or inaccessible: %s",
					object)}
			}

			// Only restore the predicates that were assigned to this group at the time
			// of the last backup.
			predSet := manifests[len(manifests)-1].getPredsInGroup(gid)

			groupMaxUid, groupMaxNsId, err := fn(gid,
				&loadBackupInput{r: reader, preds: predSet, dropOperations: manifest.DropOperations,
					isOld: manifest.Version == 0})
			if err != nil {
				return LoadResult{Err: err}
			}
			if groupMaxUid > maxUid {
				maxUid = groupMaxUid
			}
			if groupMaxNsId > maxNsId {
				maxNsId = groupMaxNsId
			}
		}
		since = manifest.Since
	}

	return LoadResult{Version: since, MaxLeaseUid: maxUid, MaxLeaseNsId: maxNsId}
}

// Verify performs basic checks to decide whether the specified backup can be restored
// to a live cluster.
func (h *s3Handler) Verify(uri *url.URL, req *pb.RestoreRequest, currentGroups []uint32) error {
	manifests, err := h.GetManifests(uri, req.GetBackupId(), req.GetBackupNum())
	if err != nil {
		return errors.Wrapf(err, "while retrieving manifests")
	}
	return verifyRequest(req, manifests, currentGroups)
}

// upload will block until it's done or an error occurs.
func (h *s3Handler) upload(mc *x.MinioClient, object string) error {
	start := time.Now()

	// We don't need to have a progress object, because we're using a Pipe. A write to Pipe would
	// block until it can be fully read. So, the rate of the writes here would be equal to the rate
	// of upload. We're already tracking progress of the writes in stream.Lists, so no need to track
	// the progress of read. By definition, it must be the same.
	n, err := mc.PutObject(h.bucketName, object, h.preader, -1, minio.PutObjectOptions{})
	glog.V(2).Infof("Backup sent %d bytes. Time elapsed: %s",
		n, time.Since(start).Round(time.Second))

	if err != nil {
		// This should cause Write to fail as well.
		glog.Errorf("Backup: Closing RW pipe due to error: %v", err)
		if err := h.pwriter.Close(); err != nil {
			return err
		}
		if err := h.preader.Close(); err != nil {
			return err
		}
	}
	return err
}

func (h *s3Handler) Close() error {
	// Done buffering, send EOF.
	if h.pwriter == nil {
		return nil
	}
	return h.flush()
}

func (h *s3Handler) flush() error {
	if err := h.pwriter.CloseWithError(nil); err != nil && err != io.EOF {
		glog.Errorf("Unexpected error when closing pipe: %v", err)
	}
	glog.V(2).Infof("Backup waiting for upload to complete.")
	// We are setting this to nil, so that closing the handler after flushing is a no-op.
	h.pwriter = nil
	return <-h.cerr

}

func (h *s3Handler) Write(b []byte) (int, error) {
	return h.pwriter.Write(b)
}

func (h *s3Handler) objectExists(objectPath string) bool {
	_, err := h.mc.StatObject(h.bucketName, objectPath, minio.StatObjectOptions{})
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return false
		} else {
			glog.Errorf("Failed to verify object existance: %s", errResponse.Code)
			return false
		}
	}
	return true
}

func (h *s3Handler) readManifest(path string, m *Manifest) error {
	reader, err := h.mc.GetObject(h.bucketName, path, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	return json.NewDecoder(reader).Decode(m)
}

func (h *s3Handler) readMasterManifest(m *MasterManifest) error {
	path := filepath.Join(h.objectPrefix, backupManifest)
	reader, err := h.mc.GetObject(h.bucketName, path, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	return json.NewDecoder(reader).Decode(m)
}

// getConsolidatedManifest walks over all the backup directories and generates a master manifest.
func (h *s3Handler) getConsolidatedManifest() (*MasterManifest, error) {
	var manifest MasterManifest

	// If there is a master manifest already, we just return it.
	objectPath := filepath.Join(h.objectPrefix, backupManifest)
	if h.objectExists(objectPath) {
		if err := h.readMasterManifest(&manifest); err != nil {
			return nil, err
		}
		return &manifest, nil
	}

	// Otherwise, we consolidate the manifests to make a master manifest.
	var paths []string
	done := make(chan struct{})
	defer close(done)
	suffix := "/" + backupManifest
	for object := range h.mc.ListObjects(h.bucketName, h.objectPrefix, true, done) {
		if strings.HasSuffix(object.Key, suffix) {
			paths = append(paths, object.Key)
		}
	}

	sort.Strings(paths)
	var mlist []*Manifest

	for _, path := range paths {
		var m Manifest
		if err := h.readManifest(path, &m); err != nil {
			return nil, errors.Wrap(err, "While Getting latest manifest")
		}
		path = filepath.Dir(path)
		_, path = filepath.Split(path)
		m.Path = path
		mlist = append(mlist, &m)
	}
	manifest.Manifests = mlist
	return &manifest, nil
}

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	fp *os.File
}

func (h *fileHandler) Exists(uri *url.URL) bool {
	return pathExist(uri.Path)
}
func (h *fileHandler) Read(uri *url.URL) ([]byte, error) {
	return ioutil.ReadFile(uri.Path)
}
func (h *fileHandler) Stream(uri *url.URL) (io.ReadCloser, error) {
	return os.Open(uri.Path)
}

// readManifest reads a manifest file at path using the handler.
// Returns nil on success, otherwise an error.
func (h *fileHandler) readManifest(path string, m *Manifest) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, m)
}

// readMasterManifest reads the master manifest file at path using the handler.
// Returns nil on success, otherwise an error.
func (h *fileHandler) readMasterManifest(path string, m *MasterManifest) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, m)
}

func (h *fileHandler) createFiles(uri *url.URL, req *pb.BackupRequest, fileName string) error {
	var dir, path string

	dir = filepath.Join(uri.Path, fmt.Sprintf(backupPathFmt, req.UnixTs))
	err := os.Mkdir(dir, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}

	path = filepath.Join(dir, fileName)
	h.fp, err = os.Create(path)
	if err != nil {
		return err
	}
	glog.V(2).Infof("Using file path: %q", path)
	return nil
}

// GetLatestManifest reads the manifests at the given URL and returns the
// latest manifest.
func (h *fileHandler) GetLatestManifest(uri *url.URL) (*Manifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Wrap(err, "Get latest manifest failed:")
	}

	manifest, err := h.getConsolidatedManifest(uri)
	if err != nil {
		return nil, errors.Wrap(err, "Get latest manifest failed while consolidation: ")
	}
	if len(manifest.Manifests) == 0 {
		return &Manifest{}, nil
	}
	return manifest.Manifests[len(manifest.Manifests)-1], nil
}

func createIfNotExists(path string) error {
	if pathExist(path) {
		return nil
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.Errorf("The path %q does not exist or it is inaccessible."+
			" While trying to create it, got error: %v", path, err)
	}
	return nil
}

// CreateBackupFile prepares the a path to save the backup file.
func (h *fileHandler) CreateBackupFile(uri *url.URL, req *pb.BackupRequest) error {
	if err := createIfNotExists(uri.Path); err != nil {
		return errors.Errorf("while CreateBackupFile: %v", err)
	}

	fileName := backupName(req.ReadTs, req.GroupId)
	return h.createFiles(uri, req, fileName)
}

// CreateManifest completes the backup by writing the manifest to a file.
func (h *fileHandler) CreateManifest(uri *url.URL, manifest *MasterManifest) error {
	var err error
	if err = createIfNotExists(uri.Path); err != nil {
		return errors.Errorf("while WriteManifest: %v", err)
	}

	tmpPath := filepath.Join(uri.Path, tmpManifest)
	if h.fp, err = os.Create(tmpPath); err != nil {
		return err
	}
	if err = json.NewEncoder(h).Encode(manifest); err != nil {
		return err
	}

	// Move the tmpManifest to backupManifest
	path := filepath.Join(uri.Path, backupManifest)
	return os.Rename(tmpPath, path)
}

// GetManifest returns the master manifest, if the directory doesn't contain
// a master manifest, then it will try to return a master manifest by consolidating
// the manifests.
func (h *fileHandler) GetManifest(uri *url.URL) (*MasterManifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Errorf("while GetLatestManifest: %v", err)
	}
	manifest, err := h.getConsolidatedManifest(uri)
	if err != nil {
		return manifest, errors.Wrap(err, "GetManifest failed to get consolidated manifest: ")
	}
	return manifest, nil
}

func (h *fileHandler) GetManifests(uri *url.URL, backupId string,
	backupNum uint64) ([]*Manifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Errorf("while GetManifests: %v", err)
	}

	manifest, err := h.getConsolidatedManifest(uri)
	if err != nil {
		return manifest.Manifests, errors.Wrap(err, "GetManifests failed to get consolidated manifest: ")
	}

	var filtered []*Manifest
	for _, m := range manifest.Manifests {
		path := filepath.Join(uri.Path, m.Path)
		if pathExist(path) {
			filtered = append(filtered, m)
		}
	}

	return getManifests(filtered, backupId, backupNum)
}

// Load uses tries to load any backup files found.
// Returns the maximum value of Since on success, error otherwise.
func (h *fileHandler) Load(uri *url.URL, backupId string, backupNum uint64, fn loadFn) LoadResult {
	manifests, err := h.GetManifests(uri, backupId, backupNum)
	if err != nil {
		return LoadResult{Err: errors.Wrapf(err, "cannot retrieve manifests")}
	}

	// Process each manifest, first check that they are valid and then confirm the
	// backup files for each group exist. Each group in manifest must have a backup file,
	// otherwise this is a failure and the user must remedy.
	var since uint64
	var maxUid, maxNsId uint64
	for i, manifest := range manifests {
		if manifest.Since == 0 || len(manifest.Groups) == 0 {
			continue
		}

		path := filepath.Join(uri.Path, manifests[i].Path)
		for gid := range manifest.Groups {
			file := filepath.Join(path, backupName(manifest.Since, gid))
			fp, err := os.Open(file)
			if err != nil {
				return LoadResult{Err: errors.Wrapf(err, "Failed to open %q", file)}
			}
			defer fp.Close()

			// Only restore the predicates that were assigned to this group at the time
			// of the last backup.
			predSet := manifests[len(manifests)-1].getPredsInGroup(gid)

			groupMaxUid, groupMaxNsId, err := fn(gid,
				&loadBackupInput{r: fp, preds: predSet, dropOperations: manifest.DropOperations,
					isOld: manifest.Version == 0})
			if err != nil {
				return LoadResult{Err: err}
			}
			maxUid = x.Max(maxUid, groupMaxUid)
			maxNsId = x.Max(maxNsId, groupMaxNsId)
		}
		since = manifest.Since
	}

	return LoadResult{Version: since, MaxLeaseUid: maxUid, MaxLeaseNsId: maxNsId}
}

// Verify performs basic checks to decide whether the specified backup can be restored
// to a live cluster.
func (h *fileHandler) Verify(uri *url.URL, req *pb.RestoreRequest, currentGroups []uint32) error {
	manifests, err := h.GetManifests(uri, req.GetBackupId(), req.GetBackupNum())
	if err != nil {
		return errors.Wrapf(err, "while retrieving manifests")
	}
	return verifyRequest(req, manifests, currentGroups)
}

func (h *fileHandler) Close() error {
	if h.fp == nil {
		return nil
	}
	if err := h.fp.Sync(); err != nil {
		glog.Errorf("While closing file: %s. Error: %v", h.fp.Name(), err)
		x.Ignore(h.fp.Close())
		return err
	}
	return h.fp.Close()
}

func (h *fileHandler) Write(b []byte) (int, error) {
	return h.fp.Write(b)
}

// pathExist checks if a path (file or dir) is found at target.
// Returns true if found, false otherwise.
func pathExist(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err) && !os.IsPermission(err)
}

// getConsolidatedManifest walks over all the backup directories and generates a master manifest.
func getConsolidatedManifest(h *UriHandler, uri *url.URL) (*MasterManifest, error) {
	if !h.Exists(uri.Path) {
		return nil, fmt.Errorf("uri: %+v not found", uri)
	}
	// if err := createIfNotExists(uri.Path); err != nil {
	// 	return nil, errors.Wrap(err, "While GetLatestManifest")
	// }

	var manifest MasterManifest

	// If there is a master manifest already, we just return it.
	path := filepath.Join(uri.Path, backupManifest)
	h.PathJoin(uri.Path, backupManifest)
	uri.Path = filepath.Join(uri.Path, backupManifest)
	if pathExist(path) {
		if err := h.readMasterManifest(path, &manifest); err != nil {
			return nil, errors.Wrap(err, "Get latest manifest failed to read master manifest: ")
		}
		return &manifest, nil
	}

	// Otherwise, we create a master manifest by going through all the backup directories.
	var paths []string
	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	_ = x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		if !isdir && strings.HasSuffix(path, suffix) {
			paths = append(paths, path)
		}
		return false
	})

	sort.Strings(paths)
	var mlist []*Manifest

	for _, path := range paths {
		var m Manifest
		if err := h.readManifest(path, &m); err != nil {
			return nil, errors.Wrap(err, "While Getting latest manifest")
		}
		path = filepath.Dir(path)
		_, path = filepath.Split(path)
		m.Path = path
		mlist = append(mlist, &m)
	}
	manifest.Manifests = mlist
	return &manifest, nil
}
