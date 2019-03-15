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
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	minio "github.com/minio/minio-go"
	"github.com/minio/minio-go/pkg/credentials"
	"github.com/minio/minio-go/pkg/s3utils"
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
}

// setup creates a new session, checks valid bucket at uri.Path, and configures a minio client.
// setup also fills in values used by the handler in subsequent calls.
// Returns a new S3 minio client, otherwise a nil client with an error.
func (h *s3Handler) setup(uri *url.URL) (*minio.Client, error) {
	if len(uri.Path) < 1 {
		return nil, x.Errorf("Invalid bucket: %q", uri.Path)
	}

	var provider credentials.Provider
	switch uri.Scheme {
	case "s3":
		// s3:///bucket/folder
		if !strings.Contains(uri.Host, ".") {
			uri.Host = defaultEndpointS3
		}
		if !s3utils.IsAmazonEndpoint(*uri) {
			return nil, x.Errorf("Invalid S3 endpoint %q", uri.Host)
		}
		// Access Key ID:     AWS_ACCESS_KEY_ID or AWS_ACCESS_KEY.
		// Secret Access Key: AWS_SECRET_ACCESS_KEY or AWS_SECRET_KEY.
		// Secret Token:      AWS_SESSION_TOKEN.
		provider = &credentials.EnvAWS{}

	default: // minio
		if uri.Host == "" {
			return nil, x.Errorf("Minio handler requires a host")
		}
		// Access Key ID:     MINIO_ACCESS_KEY.
		// Secret Access Key: MINIO_SECRET_KEY.
		provider = &credentials.EnvMinio{}
	}
	glog.V(2).Infof("Backup using host: %s, path: %s", uri.Host, uri.Path)

	if len(uri.Path) < 1 {
		return nil, x.Errorf("The S3 bucket %q is invalid", uri.Path)
	}

	creds, _ := provider.Retrieve() // error is always nil
	if creds.SignerType == credentials.SignatureAnonymous {
		return nil, x.Errorf("Environment variable credentials were not found. " +
			"If you need assistance please contact our support team.")
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
		return nil, x.Errorf("Error while looking for bucket: %s at host: %s. Error: %v",
			h.bucketName, uri.Host, err)
	}
	if !found {
		return nil, x.Errorf("Bucket was not found: %s", h.bucketName)
	}
	if len(parts) > 1 {
		h.objectPrefix = filepath.Join(parts[1:]...)
	}

	return mc, err
}

// Create creates a new session and sends our data stream to an object.
// URI formats:
//   minio://<host>/bucket/folder1.../folderN?secure=true|false
//   minio://<host:port>/bucket/folder1.../folderN?secure=true|false
//   s3://<s3 region endpoint>/bucket/folder1.../folderN?secure=true|false
//   s3:///bucket/folder1.../folderN?secure=true|false (use default S3 endpoint)
func (h *s3Handler) Create(uri *url.URL, req *Request) error {
	var objectName string

	glog.V(2).Infof("S3Handler got uri: %+v. Host: %s. Path: %s\n", uri, uri.Host, uri.Path)

	mc, err := h.setup(uri)
	if err != nil {
		return err
	}

	// Find the max version from the latest backup. This is done only when starting a new
	// backup, not when creating a manifest.
	if req.Manifest == nil {
		// Get list of objects inside the bucket and find the most recent backup.
		// If we can't find a manifest file, this is a full backup.
		var lastManifest string
		done := make(chan struct{})
		defer close(done)
		suffix := "/" + backupManifest
		for object := range mc.ListObjects(h.bucketName, h.objectPrefix, true, done) {
			if strings.HasSuffix(object.Key, suffix) && object.Key > lastManifest {
				lastManifest = object.Key
			}
		}
		if lastManifest != "" {
			var m Manifest
			reader, err := mc.GetObject(h.bucketName, lastManifest, minio.GetObjectOptions{})
			if err != nil {
				return err
			}
			defer reader.Close()
			if err = json.NewDecoder(reader).Decode(&m); err != nil {
				return err
			}
			// No new changes since last check
			if m.Version == req.Backup.SnapshotTs {
				return ErrBackupNoChanges
			}
			// Return the version of last backup
			req.Version = m.Version
		}
		objectName = fmt.Sprintf(backupNameFmt, req.Backup.ReadTs, req.Backup.GroupId)
	} else {
		objectName = backupManifest
	}

	// The backup object is: folder1...folderN/dgraph.20181106.0113/r110001-g1.backup
	object := filepath.Join(h.objectPrefix,
		fmt.Sprintf(backupPathFmt, req.Backup.UnixTs),
		objectName)
	glog.V(2).Infof("Sending data to S3 blob %q ...", object)

	h.cerr = make(chan error, 1)
	h.preader, h.pwriter = io.Pipe()
	go func() {
		h.cerr <- h.upload(mc, object)
	}()

	return nil
}

// Load creates a new session, scans for backup objects in a bucket, then tries to
// load any backup objects found.
// Returns nil on success, error otherwise.
func (h *s3Handler) Load(uri *url.URL, fn loadFn) error {
	var objects []string

	mc, err := h.setup(uri)
	if err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)
	for object := range mc.ListObjects(h.bucketName, h.objectPrefix, true, done) {
		if strings.HasSuffix(object.Key, ".backup") {
			objects = append(objects, object.Key)
		}
	}
	if len(objects) == 0 {
		return x.Errorf("No backup files found in %q", uri.String())
	}
	sort.Strings(objects)
	if glog.V(2) {
		fmt.Printf("Loading from %s: %v\n", uri.Scheme, objects)
	}

	for _, object := range objects {
		_, groupId, err := getInfo(object)
		if err != nil {
			fmt.Printf("Restore: skip invalid backup name format: %q\n", object)
			continue
		}
		reader, err := mc.GetObject(h.bucketName, object, minio.GetObjectOptions{})
		if err != nil {
			return x.Errorf("Restore closing due to error: %s", err)
		}
		defer reader.Close()
		st, err := reader.Stat()
		if err != nil {
			return x.Errorf("Restore got an unexpected error: %s", err)
		}
		if st.Size <= 0 {
			return x.Errorf("Restore remote object is empty or inaccessible: %s", object)
		}
		fmt.Printf("Downloading %q, %d bytes\n", object, st.Size)
		if err = fn(reader, groupId); err != nil {
			return err
		}
	}
	return nil
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
		h.pwriter.Close()
		h.preader.Close()
	}
	return err
}

func (h *s3Handler) Close() error {
	// we are done buffering, send EOF.
	if err := h.pwriter.CloseWithError(nil); err != nil && err != io.EOF {
		glog.Errorf("Unexpected error when closing pipe: %v", err)
	}
	glog.V(2).Infof("Backup waiting for upload to complete.")
	return <-h.cerr
}

func (h *s3Handler) Write(b []byte) (int, error) {
	return h.pwriter.Write(b)
}
