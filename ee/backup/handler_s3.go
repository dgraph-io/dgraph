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
	"sync/atomic"

	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	minio "github.com/minio/minio-go"
)

const (
	s3defaultEndpoint = "s3.amazonaws.com"
	s3accelerateHost  = "s3-accelerate"
)

// s3Handler is used for 's3:' URI scheme.
type s3Handler struct {
	bucket string
	path   string
	w      io.Writer
	n      uint64
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
	var host, path string
	// s3:///bucket/folder
	if !strings.Contains(uri.Host, ".") {
		path, host = uri.Host, s3defaultEndpoint
	}
	glog.V(2).Infof("Backup using S3 host: %s, path: %s", host, path)

	if len(path) < 1 {
		return x.Errorf("The S3 bucket %q is invalid", path)
	}

	parts := strings.Split(path[1:], "/")
	h.bucket = parts[0] // bucket

	// split path into bucket and blob
	h.path = filepath.Join(parts[1:]...)
	h.path = filepath.Join(h.path, fmt.Sprintf("dgraph.%s", req.Backup.UnixTs),
		fmt.Sprintf("r%d-g%d.backup", req.Backup.ReadTs, req.Backup.GroupId))

	// if len(parts) > 1 {
	// 	parts = append(parts, s.file)
	// 	// blob: folder1/...folderN/file1
	// 	s.file = filepath.Join(parts[1:]...)
	// }
	glog.V(3).Infof("Sending to S3. Path=%s", h.path)

	// secure by default
	// uri.Query()
	// TODO: Do the arg query here.
	// secure := s.Getarg("secure") != "false"
	secure := true

	mc, err := minio.New(host, accessKeyID, secretAccessKey, secure)
	if err != nil {
		return err
	}
	loc, err := mc.GetBucketLocation(h.bucket)
	if err != nil {
		return err
	}
	if err := mc.MakeBucket(path, loc); err != nil {
		return err
	}

	// S3 transfer acceleration support.
	if strings.Contains(host, "s3-accelerate") {
		mc.SetS3TransferAccelerate(host)
	}
	mc.TraceOn(os.Stderr)

	// TODO: Fix this up.
	// go h.send(c, s)

	return nil
}

// func (h *s3Handler) send(c *minio.Client, s *session) {
// 	glog.Infof("Backup streaming in background, est. %d bytes", s.size)
// 	start := time.Now()
// 	pr, pw := io.Pipe()
// 	buf := bufio.NewWriterSize(pw, 5<<20)
// 	h.w = buf

// 	// block until done or pipe closed.
// 	for {
// 		time.Sleep(10 * time.Millisecond) // wait for some buffered data
// 		n, err := c.PutObject(s.path, s.file, pr, s.size,
// 			minio.PutObjectOptions{})
// 		if err != nil {
// 			glog.Errorf("Failure while uploading backup: %s", err)
// 		}
// 		glog.V(3).Infof("sent %d bytes, actual %d bytes, time elapsed %s", n, h.n, time.Since(start))
// 		break
// 	}
// 	pr.CloseWithError(nil) // EOF
// }

func (h *s3Handler) Close() error {
	return nil
}

func (h *s3Handler) Write(b []byte) (int, error) {
	n, err := h.w.Write(b)
	atomic.AddUint64(&h.n, uint64(n))
	return n, err
}
