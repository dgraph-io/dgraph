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
	"bytes"
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

// getConsolidatedManifest walks over all the backup directories and generates a master manifest.
func getConsolidatedManifest(h UriHandler, uri *url.URL) (*MasterManifest, error) {
	// If there is a master manifest already, we just return it.
	if h.FileExists(backupManifest) {
		manifest, err := readMasterManifest(h, backupManifest)
		if err != nil {
			return &MasterManifest{}, errors.Wrap(err, "Failed to read master manifest")
		}
		return manifest, nil
	}

	// Otherwise, we create a master manifest by going through all the backup directories.
	paths := h.ListPaths("")

	var manifestPaths []string
	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	for _, p := range paths {
		if strings.HasSuffix(p, suffix) {
			manifestPaths = append(manifestPaths, p)
		}
	}

	sort.Strings(manifestPaths)
	var mlist []*Manifest

	for _, path := range manifestPaths {
		path = filepath.Dir(path)
		_, path = filepath.Split(path)
		m, err := readManifest(h, filepath.Join(path, backupManifest))
		if err != nil {
			return nil, errors.Wrap(err, "While Getting latest manifest")
		}
		m.Path = path
		mlist = append(mlist, m)
	}
	return &MasterManifest{Manifests: mlist}, nil
}

func readManifest(h UriHandler, path string) (*Manifest, error) {
	var m Manifest
	b, err := h.Read(path)
	if err != nil {
		return &m, errors.Wrap(err, "readManifest failed to read the file: ")
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return &m, errors.Wrap(err, "readManifest failed to unmarshal: ")
	}
	return &m, nil
}

func readMasterManifest(h UriHandler, path string) (*MasterManifest, error) {
	var m MasterManifest
	b, err := h.Read(path)
	if err != nil {
		return &m, errors.Wrap(err, "readMasterManifest failed to read the file: ")
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return &m, errors.Wrap(err, "readMasterManifest failed to unmarshal: ")
	}
	return &m, nil
}

func getLatestManifest(h UriHandler, uri *url.URL) (*Manifest, error) {
	if !h.DirExists("./") {
		return &Manifest{}, errors.Errorf("getLatestManifest: The uri path: %q doesn't exists",
			uri.Path)
	}
	manifest, err := getConsolidatedManifest(h, uri)
	if err != nil {
		return nil, errors.Wrap(err, "Get latest manifest failed while consolidation: ")
	}
	if len(manifest.Manifests) == 0 {
		return &Manifest{}, nil
	}
	return manifest.Manifests[len(manifest.Manifests)-1], nil
}

func getManifest(h UriHandler, uri *url.URL) (*MasterManifest, error) {
	if !h.DirExists("") {
		return &MasterManifest{}, errors.Errorf("getManifest: The uri path: %q doesn't exists",
			uri.Path)
	}
	manifest, err := getConsolidatedManifest(h, uri)
	if err != nil {
		return manifest, errors.Wrap(err, "Failed to get consolidated manifest: ")
	}
	return manifest, nil
}

func createManifest(h UriHandler, uri *url.URL, manifest *MasterManifest) error {
	var err error
	if !h.DirExists("./") {
		if err := h.CreateDir("./"); err != nil {
			return errors.Wrap(err, "createManifest failed to create path")
		}
	}

	if err := h.CreateFile(tmpManifest); err != nil {
		return errors.Wrap(err, "createManifest failed to create tmp path")
	}
	if err = json.NewEncoder(h).Encode(manifest); err != nil {
		return err
	}
	if err := h.WaitForWrite(); err != nil {
		return err
	}
	// Move the tmpManifest to backupManifest, this operation is not atomic for s3.
	// We try our best to move the file but if it fails then the user must move it manually.
	err = h.Rename(tmpManifest, backupManifest)
	return errors.Wrapf(err, "MOVING TEMPORARY MANIFEST TO MAIN MANIFEST FAILED!\n"+
		"It is possible that the manifest would have been corrupted. You must move "+
		"the file: %s to: %s in order to "+
		"fix the backup manifest.", tmpManifest, backupManifest)

}

func createBackupFile(h UriHandler, uri *url.URL, req *pb.BackupRequest) error {
	if !h.DirExists("./") {
		if err := h.CreateDir("./"); err != nil {
			return errors.Wrap(err, "while creating backup file")
		}
	}
	fileName := backupName(req.ReadTs, req.GroupId)
	dir := fmt.Sprintf(backupPathFmt, req.UnixTs)
	if err := h.CreateDir(dir); err != nil {
		return errors.Wrap(err, "while creating backup file")
	}
	backupFile := filepath.Join(dir, fileName)
	if err := h.CreateFile(backupFile); err != nil {
		return errors.Wrap(err, "while creating backup file")
	}
	return nil
}

func Load(h UriHandler, uri *url.URL, backupId string, backupNum uint64, fn loadFn) LoadResult {
	manifests, err := getManifestsToRestore(h, uri, backupId, backupNum)
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

		path := manifests[i].Path
		for gid := range manifest.Groups {
			file := filepath.Join(path, backupName(manifest.Since, gid))
			reader, err := h.Stream(file)
			if err != nil {
				return LoadResult{Err: errors.Wrapf(err, "Failed to open %q", file)}
			}
			defer reader.Close()

			// Only restore the predicates that were assigned to this group at the time
			// of the last backup.
			predSet := manifests[len(manifests)-1].getPredsInGroup(gid)

			groupMaxUid, groupMaxNsId, err := fn(gid,
				&loadBackupInput{r: reader, preds: predSet, dropOperations: manifest.DropOperations,
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

// verifyRequest verifies that the manifest satisfies the requirements to process the given
// restore request.
func verifyRequest(h UriHandler, uri *url.URL, req *pb.RestoreRequest,
	currentGroups []uint32) error {

	manifests, err := getManifestsToRestore(h, uri, req.GetBackupId(), req.GetBackupNum())
	if err != nil {
		return errors.Wrapf(err, "while retrieving manifests")
	}
	if len(manifests) == 0 {
		return errors.Errorf("No backups with the specified backup ID %s", req.GetBackupId())
	}

	// TODO(Ahsan): Do we need to verify the manifests again here?
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

func getManifestsToRestore(h UriHandler, uri *url.URL, backupId string,
	backupNum uint64) ([]*Manifest, error) {
	if !h.DirExists("") {
		return nil, errors.Errorf("getManifestsToRestore: The uri path: %q doesn't exists",
			uri.Path)
	}

	manifest, err := getConsolidatedManifest(h, uri)
	if err != nil {
		return manifest.Manifests, errors.Wrap(err, "Failed to get consolidated manifest: ")
	}
	return getFilteredManifests(h, manifest.Manifests, backupId, backupNum)
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

	return Load(h, uri, backupId, backupNum, fn)
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

	return verifyRequest(h, uri, req, currentGroups)
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

	m, err := getManifest(h, uri)
	if err != nil {
		return nil, err
	}
	return m.Manifests, nil
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

func getFilteredManifests(h UriHandler, manifests []*Manifest, backupId string,
	backupNum uint64) ([]*Manifest, error) {

	// validManifests are the ones for which the corresponding backup files exists.
	var validManifests []*Manifest
	for _, m := range manifests {
		missingFiles := false
		for g, _ := range m.Groups {
			path := filepath.Join(m.Path, backupName(m.Since, g))
			if !h.FileExists(path) {
				missingFiles = true
				break
			}
		}
		if !missingFiles {
			validManifests = append(validManifests, m)
		}
	}
	manifests, err := filterManifests(validManifests, backupId)
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

// UriHandler interface is implemented by URI scheme handlers.
// When adding new scheme handles, for example 'azure://', an object will implement
// this interface to supply Dgraph with a way to create or load backup files into DB.
// For all methods below, the URL object is parsed as described in `newHandler' and
// the Processor object has the DB, estimated tablets size, and backup parameters.
type UriHandler interface {
	// Handlers must know how to Write to their URI location.
	// These function calls are used by both Create and Load.
	io.WriteCloser

	// CreateDir creates a directory relative to the root path of the handler.
	CreateDir(path string) error
	// CreateFile creates a file relative to the root path of the handler. It also makes the
	// handler's descriptor to point to this file.
	CreateFile(path string) error
	// DirExists returns true if the directory relative to the root path of the handler exists.
	DirExists(path string) bool
	// FileExists returns true if the file relative to the root path of the handler exists.
	FileExists(path string) bool
	// JoinPath appends the given path to the root path of the handler.
	JoinPath(path string) string
	// ListPaths returns a list of all the valid paths from the given root path. The given root path
	// should be relative to the handler's root path.
	ListPaths(path string) []string
	// Read reads the file at given relative path and returns the read bytes.
	Read(path string) ([]byte, error)
	// Rename renames the src file to the destination file.
	Rename(src, dst string) error
	// Stream would stream the path via an instance of io.ReadCloser. Close must be called at the
	// end to release resources appropriately.
	Stream(path string) (io.ReadCloser, error)
	// WaitForWrite waits for the write to complete. It is meaningful for s3 uploads.
	WaitForWrite() error
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
		return NewFileHandler(uri), nil
	case "minio", "s3":
		return NewS3Handler(uri, creds)
	}
	return nil, errors.Errorf("Unable to handle url: %s", uri)
}

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	fp      *os.File
	rootDir string
	prefix  string
}

func NewFileHandler(uri *url.URL) *fileHandler {
	h := &fileHandler{}
	h.rootDir, h.prefix = filepath.Split(uri.Path)
	return h
}

func (h *fileHandler) JoinPath(path string) string {
	return filepath.Join(h.rootDir, h.prefix, path)
}

func (h *fileHandler) DirExists(path string) bool {
	path = h.JoinPath(path)
	return pathExist(path)
}
func (h *fileHandler) FileExists(path string) bool {
	path = h.JoinPath(path)
	return pathExist(path)
}
func (h *fileHandler) Read(path string) ([]byte, error) {
	path = h.JoinPath(path)
	return ioutil.ReadFile(path)
}
func (h *fileHandler) Stream(path string) (io.ReadCloser, error) {
	path = h.JoinPath(path)
	return os.Open(path)
}

func (h *fileHandler) ListPaths(path string) []string {
	path = h.JoinPath(path)
	return x.WalkPathFunc(path, func(path string, isDis bool) bool {
		return true
	})
}

func (h *fileHandler) CreateDir(path string) error {
	path = h.JoinPath(path)
	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.Errorf("Create path failed to create path %s, got error: %v", path, err)
	}
	return nil
}

func (h *fileHandler) CreateFile(path string) error {
	path = h.JoinPath(path)
	var err error
	h.fp, err = os.Create(path)
	return errors.Wrapf(err, "File handler failed to create file %s", path)
}

func (h *fileHandler) Rename(src, dst string) error {
	src = h.JoinPath(src)
	dst = h.JoinPath(dst)
	return os.Rename(src, dst)
}

func (h *fileHandler) WaitForWrite() error         { return nil }
func (h *fileHandler) Write(b []byte) (int, error) { return h.fp.Write(b) }

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

// pathExist checks if a path (file or dir) is found at target.
// Returns true if found, false otherwise.
func pathExist(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err) && !os.IsPermission(err)
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

// NewS3Handler creates a new session, checks valid bucket at uri.Path, and configures a
// minio client. It also fills in values used by the handler in subsequent calls.
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

func (h *s3Handler) CreateDir(path string) error { return nil }
func (h *s3Handler) DirExists(path string) bool  { return true }

func (h *s3Handler) FileExists(path string) bool {
	objectPath := h.getObjectPath(path)
	_, err := h.mc.StatObject(h.bucketName, objectPath, minio.StatObjectOptions{})
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" {
			return false
		} else {
			glog.Errorf("Failed to verify object existence: %v", err)
			return false
		}
	}
	return true
}

func (h *s3Handler) JoinPath(path string) string {
	return filepath.Join(h.bucketName, h.objectPrefix, path)
}

func (h *s3Handler) Read(path string) ([]byte, error) {
	objectPath := h.getObjectPath(path)
	var buf bytes.Buffer

	reader, err := h.mc.GetObject(h.bucketName, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return buf.Bytes(), errors.Wrap(err, "Failed to read s3 object")
	}
	defer reader.Close()

	if _, err := buf.ReadFrom(reader); err != nil {
		return buf.Bytes(), errors.Wrap(err, "Failed to read the s3 object")
	}
	return buf.Bytes(), nil
}

func (h *s3Handler) Stream(path string) (io.ReadCloser, error) {
	objectPath := h.getObjectPath(path)
	reader, err := h.mc.GetObject(h.bucketName, objectPath, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return reader, nil
}

func (h *s3Handler) ListPaths(path string) []string {
	var paths []string
	done := make(chan struct{})
	defer close(done)
	path = h.getObjectPath(path)
	for object := range h.mc.ListObjects(h.bucketName, path, true, done) {
		paths = append(paths, object.Key)
	}
	return paths
}

func (h *s3Handler) CreateFile(path string) error {
	objectPath := h.getObjectPath(path)
	glog.V(2).Infof("Sending data to %s blob %q ...", h.uri.Scheme, objectPath)

	h.cerr = make(chan error, 1)
	h.preader, h.pwriter = io.Pipe()
	go func() {
		h.cerr <- h.upload(h.mc, objectPath)
	}()
	return nil
}

func (h *s3Handler) Rename(srcPath, dstPath string) error {
	src := minio.NewSourceInfo(h.bucketName, srcPath, nil)
	dst, err := minio.NewDestinationInfo(h.bucketName, dstPath, nil, nil)
	if err != nil {
		return errors.Wrap(err, "Rename failed to create dstInfo")
	}
	// We try copying 100 times, if it still fails, then the user should manually rename.
	err = x.RetryUntilSuccess(100, time.Second, func() error {
		if err := h.mc.CopyObject(dst, src); err != nil {
			return errors.Wrapf(err, "While renaming object in s3, copy failed ")
		}
		return nil
	})
	if err != nil {
		return err
	}

	err = h.mc.RemoveObject(h.bucketName, srcPath)
	return errors.Wrap(err, "Rename failed to remove temporary file")
}

func (h *s3Handler) WaitForWrite() error {
	if err := h.pwriter.CloseWithError(nil); err != nil && err != io.EOF {
		glog.Errorf("Unexpected error when closing pipe: %v", err)
	}
	glog.V(2).Infof("Waiting for upload to complete.")
	// We are setting this to nil, so that closing the handler after FinishWrite is a no-op.
	h.pwriter = nil
	return <-h.cerr
}

func (h *s3Handler) Write(b []byte) (int, error) { return h.pwriter.Write(b) }

func (h *s3Handler) Close() error {
	if h.pwriter == nil {
		return nil
	}
	if err := h.pwriter.CloseWithError(nil); err != nil && err != io.EOF {
		glog.Errorf("Unexpected error when closing pipe: %v", err)
	}
	glog.V(2).Infof("Backup waiting for upload to complete.")
	return <-h.cerr
}

func (h *s3Handler) getObjectPath(path string) string {
	return filepath.Join(h.objectPrefix, path)
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
