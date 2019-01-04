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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/x"

	humanize "github.com/dustin/go-humanize"

	"github.com/golang/glog"
	minio "github.com/minio/minio-go"
)

const (
	s3DefaultEndpoint = "s3.amazonaws.com"
	s3AccelerateHost  = "s3-accelerate"
)

// s3Handler is used for 's3:' URI scheme.
type s3Handler struct {
	bucketName, objectPrefix string
	pwriter                  *io.PipeWriter
	preader                  *io.PipeReader
	cerr                     chan error
}

// setup creates an AWS session, checks valid bucket at uri.Path, and configures a minio client.
// setup also fills in values used by the handler in subsequent calls.
// Returns a new S3 minio client, otherwise a nil client with an error.
func (h *s3Handler) setup(uri *url.URL) (*minio.Client, error) {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, x.Errorf("Env vars AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY not set.")
	}

	// s3:///bucket/folder
	if !strings.Contains(uri.Host, ".") {
		uri.Host = s3DefaultEndpoint
	}
	if !strings.HasSuffix(uri.Host, s3DefaultEndpoint[2:]) {
		return nil, x.Errorf("Not an S3 endpoint: %s", uri.Host)
	}
	glog.V(2).Infof("Backup using S3 host: %s, path: %s", uri.Host, uri.Path)

	if len(uri.Path) < 1 {
		return nil, x.Errorf("The S3 bucket %q is invalid", uri.Path)
	}

	// secure by default
	secure := uri.Query().Get("secure") != "false"
	// enable HTTP tracing
	traceon := uri.Query().Get("trace") == "true"

	mc, err := minio.New(uri.Host, accessKeyID, secretAccessKey, secure)
	if err != nil {
		return nil, err
	}
	// S3 transfer acceleration support.
	if strings.Contains(uri.Host, s3AccelerateHost) {
		mc.SetS3TransferAccelerate(uri.Host)
	}
	if traceon {
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
		return nil, x.Errorf("S3 bucket %s not found.", h.bucketName)
	}
	if len(parts) > 1 {
		h.objectPrefix = filepath.Join(parts[1:]...)
	}

	return mc, err
}

// Create creates an AWS session and sends our data stream to an S3 blob.
// URI formats:
//   s3://<s3 region endpoint>/bucket/folder1.../folderN?secure=true|false
//   s3:///bucket/folder1.../folderN?secure=true|false (use default S3 endpoint)
func (h *s3Handler) Create(uri *url.URL, req *Request) error {
	glog.V(2).Infof("S3Handler got uri: %+v. Host: %s. Path: %s\n", uri, uri.Host, uri.Path)

	mc, err := h.setup(uri)
	if err != nil {
		return err
	}

	// The object is: folder1...folderN/dgraph.20181106.0113/r110001-g1.backup
	object := filepath.Join(h.objectPrefix,
		fmt.Sprintf("dgraph.%s", req.Backup.UnixTs),
		fmt.Sprintf(backupFmt, req.Backup.ReadTs, req.Backup.GroupId))
	glog.V(2).Infof("Sending data to S3 blob %q ...", object)

	h.cerr = make(chan error, 1)
	go func() {
		h.cerr <- h.upload(mc, object)
	}()

	glog.Infof("Uploading data, estimated size %s", humanize.Bytes(req.Sizex))
	return nil
}

// Load creates an AWS session, scans for backup objects in a bucket, then tries to
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
	glog.V(2).Infof("Loading from S3: %v", objects)

	for _, object := range objects {
		reader, err := mc.GetObject(h.bucketName, object, minio.GetObjectOptions{})
		if err != nil {
			return x.Errorf("Restore closing due to error: %s", err)
		}
		st, err := reader.Stat()
		if err != nil {
			reader.Close()
			return x.Errorf("Restore got an unexpected error: %s", err)
		}
		if st.Size <= 0 {
			reader.Close()
			return x.Errorf("Restore remote object is empty or inaccessible: %s", object)
		}
		fmt.Printf("--- Downloading %q, %d bytes\n", object, st.Size)
		if err = fn(reader, object); err != nil {
			reader.Close()
			return err
		}
	}
	return nil
}

// upload will block until it's done or an error occurs.
func (h *s3Handler) upload(mc *minio.Client, object string) error {
	start := time.Now()
	h.preader, h.pwriter = io.Pipe()

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
