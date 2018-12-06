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
	"bufio"
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
)

const fileReadBufSize = 4 << 12

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	writer *os.File
	reader io.Reader
	cerr   chan error
}

// Open authenticates or prepares a handler session.
// Returns error on failure, nil on success.
func (h *fileHandler) Open(uri *url.URL, req *Request) error {
	// check that this path exists and we can access it.
	if !h.exists(uri.Path) {
		return x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// Backup: open a single file for write.
	if req.Backup.Target != "" {
		dir := filepath.Join(uri.Path, fmt.Sprintf("dgraph.%s", req.Backup.UnixTs))
		if err := os.Mkdir(dir, 0700); err != nil {
			return err
		}

		path := filepath.Join(dir,
			fmt.Sprintf("r%d-g%d.backup", req.Backup.ReadTs, req.Backup.GroupId))
		fp, err := os.Create(path)
		if err != nil {
			return err
		}
		glog.V(2).Infof("Using file path: %q", path)
		h.writer = fp

		return nil
	}

	// Restore: scan location and find backup files. load them sequentially.
	files := x.FindFilesFunc(req.Backup.Source, func(path string) bool {
		return strings.HasSuffix(path, ".backup")
	})
	if len(files) == 0 {
		return x.Errorf("No backup files found in %q", req.Backup.Source)
	}
	sort.Strings(files)

	h.cerr = make(chan error, 1)
	go func() {
		h.cerr <- h.multiRead(files...)
	}()

	glog.V(2).Infof("Loading backup file(s): %v", files)
	return nil
}

// multiRead will read multiple files sequentially and send via pipe.
func (h *fileHandler) multiRead(files ...string) error {
	start := time.Now()
	pr, pw := io.Pipe()
	h.reader = pr

	var tot uint64
	for _, file := range files {
		fp, err := os.Open(file)
		if err != nil {
			return err
		}
		n, err := io.Copy(pw, bufio.NewReaderSize(fp, fileReadBufSize))
		if err != nil {
			return err
		}
		tot += uint64(n)
	}
	glog.V(2).Infof("Restore recv %s bytes. Time elapsed: %s",
		humanize.Bytes(tot), time.Since(start).Round(time.Second))
	return pw.CloseWithError(nil)
}

func (h *fileHandler) Close() error {
	if h.writer == nil {
		return <-h.cerr
	}
	if err := h.writer.Sync(); err != nil {
		glog.Errorf("While closing file: %s. Error: %v", h.writer.Name(), err)
		x.Ignore(h.writer.Close())
		return err
	}
	return h.writer.Close()
}

// Read will read each file in order until done or error.
func (h *fileHandler) Read(b []byte) (int, error) {
	return h.reader.Read(b)
}

func (h *fileHandler) Write(b []byte) (int, error) {
	return h.writer.Write(b)
}

// Exists checks if a path (file or dir) is found at target.
// Returns true if found, false otherwise.
func (h *fileHandler) exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err) && !os.IsPermission(err)
}
