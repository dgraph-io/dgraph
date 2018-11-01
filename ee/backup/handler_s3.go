/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	minio "github.com/minio/minio-go"
)

const (
	bufSize           = 1 << 20
	defaultS3Endpoint = "s3.amazonaws.com"
)

// s3Handler is used for 's3:' URI scheme.
type s3Handler struct {
	client *minio.Client
	buf    *bufio.Writer
	pr     io.ReadCloser
	pw     io.WriteCloser
}

func (h *s3Handler) Open(s *session) error {
	var err error

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKeyID == "" || secretAccessKey == "" {
		return x.Errorf("Env vars AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY not set.")
	}
	// s3:///bucket/folder
	if !strings.Contains(s.host, ".") {
		s.path = s.host
		s.host = ""
	}
	// no host part, use default server
	if s.host == "" {
		s.host = defaultS3Endpoint
	}
	glog.V(3).Infof("using S3 host: %s", s.host)

	if len(s.path) < 1 {
		return x.Errorf("The bucket name %q is invalid", s.path)
	}
	// split path into bucket and blob
	parts := strings.Split(s.path[1:], "/")
	s.path = parts[0] // bucket
	if len(parts) > 1 {
		parts = append(parts, s.file)
		// blob: folder1/...folderN/file1
		s.file = filepath.Join(parts[1:]...)
	}
	glog.V(3).Infof("sending to S3: bucket[%q] blob[%q]", s.path, s.file)

	// secure by default
	secure := s.Getarg("secure") != "false"

	h.client, err = minio.New(s.host, accessKeyID, secretAccessKey, secure)
	if err != nil {
		return err
	}

	h.pr, h.pw = io.Pipe()
	h.buf = bufio.NewWriterSize(h.pw, bufSize)

	// block until done.
	go func() {
		for {
			glog.V(3).Infof("--- blocking for pipe reader ...")
			_, err := h.client.PutObject(s.path, s.file, h.pr, -1,
				minio.PutObjectOptions{ContentType: "application/octet-stream"})
			if err != nil {
				// TODO: need to send this back to caller
				glog.Errorf("PutObject %s, %s: %v", s.path, s.file, err)
			}
			glog.V(3).Infof("--- done with pipe")
			break
		}
		if err := h.pr.Close(); err != nil {
			glog.V(3).Infof("failure closing pipe reader: %s", err)
		}
	}()

	return nil
}

func (h *s3Handler) Write(b []byte) (int, error) {
	return h.buf.Write(b)
}

func (h *s3Handler) Close() error {
	defer func() {
		if err := h.pw.Close(); err != nil {
			glog.V(3).Infof("failure closing pipe writer: %s", err)
		}
	}()
	return h.buf.Flush()
}

func init() {
	addHandler("s3", &s3Handler{})
}
