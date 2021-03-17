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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/x"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/golang/glog"
	minio "github.com/minio/minio-go/v6"
	"github.com/minio/minio-go/v6/pkg/credentials"
	"github.com/pkg/errors"
)

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
			maxUid = x.Max(maxUid, groupMaxUid)
			maxNsId = x.Max(maxNsId, groupMaxNsId)
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
