/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
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
	bucket  string
	object  string
	pwriter *io.PipeWriter
	preader *io.PipeReader
	cerr    chan error
}

// Open creates an AWS session and sends our data stream to an S3 blob.
// URI formats:
//   s3://<s3 region endpoint>/bucket/folder1.../folderN?secure=true|false
//   s3:///bucket/folder1.../folderN?secure=true|false (use default S3 endpoint)
func (h *s3Handler) Open(uri *url.URL, req *Request) error {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKeyID == "" || secretAccessKey == "" {
		return x.Errorf("Env vars AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY not set.")
	}

	glog.V(2).Infof("S3Handler got uri: %+v\n", uri)
	// s3:///bucket/folder
	if !strings.Contains(uri.Host, ".") {
		uri.Path, uri.Host = uri.Host, s3DefaultEndpoint
	}
	glog.V(2).Infof("Backup using S3 host: %s, path: %s", uri.Host, uri.Path)

	if len(uri.Path) < 1 {
		return x.Errorf("The S3 bucket %q is invalid", uri.Path)
	}

	// split path into bucket and blob
	parts := strings.Split(uri.Path[1:], "/")
	h.bucket = parts[0] // bucket
	// The location is: /bucket/folder1...folderN/dgraph.20181106.0113/r110001-g1.backup
	parts = append(parts, fmt.Sprintf("dgraph.%s", req.Backup.UnixTs))
	parts = append(parts, fmt.Sprintf("r%d-g%d.backup", req.Backup.ReadTs, req.Backup.GroupId))
	h.object = filepath.Join(parts[1:]...)
	glog.V(3).Infof("Sending data to S3 blob %q ...", h.object)

	// secure by default
	secure := uri.Query().Get("secure") != "false"

	mc, err := minio.New(uri.Host, accessKeyID, secretAccessKey, secure)
	if err != nil {
		return err
	}
	// S3 transfer acceleration support.
	if strings.Contains(uri.Host, s3AccelerateHost) {
		mc.SetS3TransferAccelerate(uri.Host)
	}
	// mc.TraceOn(os.Stderr)

	found, err := mc.BucketExists(h.bucket)
	if err != nil {
		return err
	}
	if !found {
		return x.Errorf("S3 bucket %q not found. Use host with specific region.", h.bucket)
	}

	h.cerr = make(chan error, 1)
	go func() {
		h.cerr <- h.upload(mc)
	}()

	glog.Infof("Uploading data, estimated size %s", humanize.Bytes(req.Sizex))
	return nil
}

// progress allows us to monitor the progress of an upload.
type progress struct{ bytes, count int }

func (p *progress) Read(b []byte) (int, error) {
	p.count++
	p.bytes += len(b)
	if p.count%10 == 0 {
		glog.V(2).Infof("Backup Progress: Uploaded %s.\n", humanize.Bytes(uint64(p.bytes)))
	}
	return len(b), nil
}

// upload will block until it's done or an error occurs.
func (h *s3Handler) upload(mc *minio.Client) error {
	start := time.Now()
	h.preader, h.pwriter = io.Pipe()
	n, err := mc.PutObject(h.bucket, h.object, h.preader, -1,
		minio.PutObjectOptions{Progress: &progress{}})
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
		glog.Errorf("Unexpected error while uploading: %v", err)
	}
	if err := h.preader.Close(); err != nil {
		glog.Errorf("Error while closing pipe reader: %v", err)
	}
	glog.V(2).Infof("Backup waiting for upload to complete.")
	return <-h.cerr
}

func (h *s3Handler) Write(b []byte) (int, error) {
	return h.pwriter.Write(b)
}
