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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	minio "github.com/minio/minio-go"
	"github.com/minio/minio-go/pkg/credentials"
	"github.com/minio/minio-go/pkg/s3utils"
	"github.com/pkg/errors"
)

const (
	// Shown in transfer logs
	appName = "Dgraph"

	// defaultEndpointS3 is used with s3 scheme when no host is provided
	defaultEndpointS3 = "s3.amazonaws.com"

	// s3AccelerateSubstr S3 acceleration is enabled if the S3 host is contains this substring.
	// See http://docs.aws.amazon.com/AmazonS3/latest/dev/transfer-acceleration.html
	s3AccelerateSubstr = "s3-accelerate"
)

// s3Handler is used for 's3:' and 'minio:' URI schemes.
type s3Handler struct {
	bucketName, objectPrefix string
	pwriter                  *io.PipeWriter
	preader                  *io.PipeReader
	cerr                     chan error
	creds                    *Credentials
	uri                      *url.URL
}

// setup creates a new session, checks valid bucket at uri.Path, and configures a minio client.
// setup also fills in values used by the handler in subsequent calls.
// Returns a new S3 minio client, otherwise a nil client with an error.
func (h *s3Handler) setup(uri *url.URL) (*minio.Client, error) {
	if len(uri.Path) < 1 {
		return nil, errors.Errorf("Invalid bucket: %q", uri.Path)
	}

	glog.V(2).Infof("Backup using host: %s, path: %s", uri.Host, uri.Path)

	var creds credentials.Value
	switch {
	case h.creds.isAnonymous():
		// No need to setup credentials.
	case !h.creds.hasCredentials():
		var provider credentials.Provider
		switch uri.Scheme {
		case "s3":
			// Access Key ID:     AWS_ACCESS_KEY_ID or AWS_ACCESS_KEY.
			// Secret Access Key: AWS_SECRET_ACCESS_KEY or AWS_SECRET_KEY.
			// Secret Token:      AWS_SESSION_TOKEN.
			provider = &credentials.EnvAWS{}
		default: // minio
			// Access Key ID:     MINIO_ACCESS_KEY.
			// Secret Access Key: MINIO_SECRET_KEY.
			provider = &credentials.EnvMinio{}
		}

		// If no credentials can be retrieved, an attempt to access the destination
		// with no credentials will be made.
		creds, _ = provider.Retrieve() // error is always nil
	default:
		creds.AccessKeyID = h.creds.accessKey
		creds.SecretAccessKey = h.creds.secretKey
		creds.SessionToken = h.creds.sessionToken
	}

	// Verify URI and set default S3 host if needed.
	switch uri.Scheme {
	case "s3":
		// s3:///bucket/folder
		if !strings.Contains(uri.Host, ".") {
			uri.Host = defaultEndpointS3
		}
		if !s3utils.IsAmazonEndpoint(*uri) {
			return nil, errors.Errorf("Invalid S3 endpoint %q", uri.Host)
		}
	default: // minio
		if uri.Host == "" {
			return nil, errors.Errorf("Minio handler requires a host")
		}
	}

	secure := uri.Query().Get("secure") != "false" // secure by default

	mc, err := minio.New(uri.Host, creds.AccessKeyID, creds.SecretAccessKey, secure)
	if err != nil {
		return nil, err
	}

	// Set client app name "Dgraph/v1.0.x"
	mc.SetAppInfo(appName, x.Version())

	// S3 transfer acceleration support.
	if uri.Scheme == "s3" && strings.Contains(uri.Host, s3AccelerateSubstr) {
		mc.SetS3TransferAccelerate(uri.Host)
	}

	// enable HTTP tracing
	if uri.Query().Get("trace") == "true" {
		mc.TraceOn(os.Stderr)
	}

	// split path into bucketName and blobPrefix
	parts := strings.Split(uri.Path[1:], "/")
	h.bucketName = parts[0] // bucket

	// verify the requested bucket exists.
	found, err := mc.BucketExists(h.bucketName)
	if err != nil {
		return nil, errors.Wrapf(err, "while looking for bucket %s at host %s",
			h.bucketName, uri.Host)
	}
	if !found {
		return nil, errors.Errorf("Bucket was not found: %s", h.bucketName)
	}
	if len(parts) > 1 {
		h.objectPrefix = filepath.Join(parts[1:]...)
	}

	return mc, err
}

func (h *s3Handler) createObject(uri *url.URL, req *pb.BackupRequest, mc *minio.Client,
	objectName string) {

	// The backup object is: folder1...folderN/dgraph.20181106.0113/r110001-g1.backup
	object := filepath.Join(h.objectPrefix, fmt.Sprintf(backupPathFmt, req.UnixTs),
		objectName)
	glog.V(2).Infof("Sending data to %s blob %q ...", uri.Scheme, object)

	h.cerr = make(chan error, 1)
	h.preader, h.pwriter = io.Pipe()
	go func() {
		h.cerr <- h.upload(mc, object)
	}()
}

// GetLatestManifest reads the manifests at the given URL and returns the
// latest manifest.
func (h *s3Handler) GetLatestManifest(uri *url.URL) (*Manifest, error) {
	mc, err := h.setup(uri)
	if err != nil {
		return nil, err
	}

	// Find the max Since value from the latest backup.
	var lastManifest string
	done := make(chan struct{})
	defer close(done)
	suffix := "/" + backupManifest
	for object := range mc.ListObjects(h.bucketName, h.objectPrefix, true, done) {
		if strings.HasSuffix(object.Key, suffix) && object.Key > lastManifest {
			lastManifest = object.Key
		}
	}

	var m Manifest
	if lastManifest == "" {
		return &m, nil
	}

	if err := h.readManifest(mc, lastManifest, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateBackupFile creates a new session and prepares the data stream for the backup.
// URI formats:
//   minio://<host>/bucket/folder1.../folderN?secure=true|false
//   minio://<host:port>/bucket/folder1.../folderN?secure=true|false
//   s3://<s3 region endpoint>/bucket/folder1.../folderN?secure=true|false
//   s3:///bucket/folder1.../folderN?secure=true|false (use default S3 endpoint)
func (h *s3Handler) CreateBackupFile(uri *url.URL, req *pb.BackupRequest) error {
	glog.V(2).Infof("S3Handler got uri: %+v. Host: %s. Path: %s\n", uri, uri.Host, uri.Path)

	mc, err := h.setup(uri)
	if err != nil {
		return err
	}

	objectName := backupName(req.ReadTs, req.GroupId)
	h.createObject(uri, req, mc, objectName)
	return nil
}

// CreateManifest finishes a backup by creating an object to store the manifest.
func (h *s3Handler) CreateManifest(uri *url.URL, req *pb.BackupRequest) error {
	glog.V(2).Infof("S3Handler got uri: %+v. Host: %s. Path: %s\n", uri, uri.Host, uri.Path)

	mc, err := h.setup(uri)
	if err != nil {
		return err
	}

	h.createObject(uri, req, mc, backupManifest)
	return nil
}

// readManifest reads a manifest file at path using the handler.
// Returns nil on success, otherwise an error.
func (h *s3Handler) readManifest(mc *minio.Client, object string, m *Manifest) error {
	reader, err := mc.GetObject(h.bucketName, object, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	return json.NewDecoder(reader).Decode(m)
}

// Load creates a new session, scans for backup objects in a bucket, then tries to
// load any backup objects found.
// Returns nil and the maximum Since value on success, error otherwise.
func (h *s3Handler) Load(uri *url.URL, backupId string, fn loadFn) LoadResult {
	mc, err := h.setup(uri)
	if err != nil {
		return LoadResult{0, 0, err}
	}

	var paths []string

	doneCh := make(chan struct{})
	defer close(doneCh)

	suffix := "/" + backupManifest
	for object := range mc.ListObjects(h.bucketName, h.objectPrefix, true, doneCh) {
		if strings.HasSuffix(object.Key, suffix) {
			paths = append(paths, object.Key)
		}
	}
	if len(paths) == 0 {
		return LoadResult{0, 0, errors.Errorf("No manifests found at: %s", uri.String())}
	}
	sort.Strings(paths)
	if glog.V(3) {
		fmt.Printf("Found backup manifest(s) %s: %v\n", uri.Scheme, paths)
	}

	// since is returned with the max manifest Since value found.
	var since uint64

	// Read and filter the manifests to get the list of manifests to consider
	// for this restore operation.
	var manifests []*Manifest
	for _, path := range paths {
		var m Manifest
		if err := h.readManifest(mc, path, &m); err != nil {
			return LoadResult{0, 0, errors.Wrapf(err, "While reading %q", path)}
		}
		m.Path = path
		manifests = append(manifests, &m)
	}
	manifests, err = filterManifests(manifests, backupId)
	if err != nil {
		return LoadResult{0, 0, err}
	}

	// Process each manifest, first check that they are valid and then confirm the
	// backup manifests for each group exist. Each group in manifest must have a backup file,
	// otherwise this is a failure and the user must remedy.
	var maxUid uint64
	for i, manifest := range manifests {
		if manifest.Since == 0 || len(manifest.Groups) == 0 {
			if glog.V(2) {
				fmt.Printf("Restore: skip backup: %#v\n", manifest)
			}
			continue
		}

		path := filepath.Dir(manifests[i].Path)
		for gid := range manifest.Groups {
			object := filepath.Join(path, backupName(manifest.Since, gid))
			reader, err := mc.GetObject(h.bucketName, object, minio.GetObjectOptions{})
			if err != nil {
				return LoadResult{0, 0, errors.Wrapf(err, "Failed to get %q", object)}
			}
			defer reader.Close()

			st, err := reader.Stat()
			if err != nil {
				return LoadResult{0, 0, errors.Wrapf(err, "Stat failed %q", object)}
			}
			if st.Size <= 0 {
				return LoadResult{0, 0,
					errors.Errorf("Remote object is empty or inaccessible: %s", object)}
			}
			fmt.Printf("Downloading %q, %d bytes\n", object, st.Size)

			// Only restore the predicates that were assigned to this group at the time
			// of the last backup.
			predSet := manifests[len(manifests)-1].getPredsInGroup(gid)

			groupMaxUid, err := fn(reader, int(gid), predSet)
			if err != nil {
				return LoadResult{0, 0, err}
			}
			if groupMaxUid > maxUid {
				maxUid = groupMaxUid
			}
		}
		since = manifest.Since
	}

	return LoadResult{since, maxUid, nil}
}

// ListManifests loads the manifests in the locations and returns them.
func (h *s3Handler) ListManifests(uri *url.URL) ([]string, error) {
	mc, err := h.setup(uri)
	if err != nil {
		return nil, err
	}
	h.uri = uri

	var manifests []string
	doneCh := make(chan struct{})
	defer close(doneCh)

	suffix := "/" + backupManifest
	for object := range mc.ListObjects(h.bucketName, h.objectPrefix, true, doneCh) {
		if strings.HasSuffix(object.Key, suffix) {
			manifests = append(manifests, object.Key)
		}
	}
	if len(manifests) == 0 {
		return nil, errors.Errorf("No manifests found at: %s", uri.String())
	}
	sort.Strings(manifests)
	if glog.V(3) {
		fmt.Printf("Found backup manifest(s) %s: %v\n", uri.Scheme, manifests)
	}
	return manifests, nil
}

func (h *s3Handler) ReadManifest(path string, m *Manifest) error {
	mc, err := h.setup(h.uri)
	if err != nil {
		return err
	}

	return h.readManifest(mc, path, m)
}

// upload will block until it's done or an error occurs.
func (h *s3Handler) upload(mc *minio.Client, object string) error {
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
	if err := h.pwriter.CloseWithError(nil); err != nil && err != io.EOF {
		glog.Errorf("Unexpected error when closing pipe: %v", err)
	}
	glog.V(2).Infof("Backup waiting for upload to complete.")
	return <-h.cerr
}

func (h *s3Handler) Write(b []byte) (int, error) {
	return h.pwriter.Write(b)
}
