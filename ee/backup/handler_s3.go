/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"bufio"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

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
	w io.Writer
	n uint64
}

// New creates a new instance of this handler.
func (h *s3Handler) New() handler {
	return &s3Handler{}
}

// Open creates an AWS session and sends our data stream to an S3 blob.
// URI formats:
//   s3://<s3 region endpoint>/bucket/folder1.../folderN?secure=true|false
//   s3:///bucket/folder1.../folderN?secure=true|false (use default S3 endpoint)
func (h *s3Handler) Open(s *session) error {
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKeyID == "" || secretAccessKey == "" {
		return x.Errorf("Env vars AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY not set.")
	}
	// s3:///bucket/folder
	if !strings.Contains(s.host, ".") {
		s.path = s.host
		s.host = s3defaultEndpoint
	}
	glog.V(3).Infof("using S3 endpoint: %s", s.host)

	if len(s.path) < 1 {
		return x.Errorf("the S3 bucket %q is invalid", s.path)
	}

	// split path into bucket and blob
	parts := strings.Split(s.path[1:], "/")
	s.path = parts[0] // bucket
	if len(parts) > 1 {
		parts = append(parts, s.file)
		// blob: folder1/...folderN/file1
		s.file = filepath.Join(parts[1:]...)
	}
	glog.V(3).Infof("sending to S3: %s", path.Join(s.host, s.path, s.file))

	// secure by default
	secure := s.Getarg("secure") != "false"

	c, err := minio.New(s.host, accessKeyID, secretAccessKey, secure)
	if err != nil {
		return err
	}
	// S3 transfer acceleration support.
	if strings.Contains(s.host, "s3-accelerate") {
		c.SetS3TransferAccelerate(s.host)
	}
	c.TraceOn(os.Stderr)

	go h.send(c, s)

	return nil
}

func (h *s3Handler) send(c *minio.Client, s *session) {
	glog.Infof("Backup streaming in background, est. %d bytes", s.size)
	start := time.Now()
	pr, pw := io.Pipe()
	buf := bufio.NewWriterSize(pw, 5<<20)
	h.w = buf

	// block until done or pipe closed.
	for {
		time.Sleep(10 * time.Millisecond) // wait for some buffered data
		n, err := c.PutObject(s.path, s.file, pr, s.size,
			minio.PutObjectOptions{})
		if err != nil {
			glog.Errorf("Failure while uploading backup: %s", err)
		}
		glog.V(3).Infof("sent %d bytes, actual %d bytes, time elapsed %s", n, h.n, time.Since(start))
		break
	}
	pr.CloseWithError(nil) // EOF
}

func (h *s3Handler) Close() error {
	return nil
}

func (h *s3Handler) Write(b []byte) (int, error) {
	n, err := h.w.Write(b)
	atomic.AddUint64(&h.n, uint64(n))
	return n, err
}

func init() {
	addHandler("s3", &s3Handler{})
}
