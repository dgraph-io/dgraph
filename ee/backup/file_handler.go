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
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

const fileReadBufSize = 4 << 12

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	fp   *os.File
	cerr chan error
}

// Create prepares the a path to save backup files.
// Returns error on failure, nil on success.
func (h *fileHandler) Create(uri *url.URL, req *Request) error {
	// check that the path exists and we can access it.
	if !h.exists(uri.Path) {
		return x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	dir := filepath.Join(uri.Path, fmt.Sprintf("dgraph.%s", req.Backup.UnixTs))
	if err := os.Mkdir(dir, 0700); err != nil {
		if !os.IsExist(err) {
			return err
		}
	}

	path := filepath.Join(dir,
		fmt.Sprintf("r%d-g%d.backup", req.Backup.ReadTs, req.Backup.GroupId))
	fp, err := os.Create(path)
	if err != nil {
		return err
	}
	glog.V(2).Infof("Using file path: %q", path)
	h.fp = fp

	return nil
}

// Load uses loadFn to try to load any backup files found.
// Returns nil on success, error otherwise.
func (h *fileHandler) Load(uri *url.URL, loadFn loadFunc) error {
	if !h.exists(uri.Path) {
		return x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// find files and sort.
	files := x.FindFilesFunc(uri.Path, func(path string) bool {
		return strings.HasSuffix(path, ".backup")
	})
	if len(files) == 0 {
		return x.Errorf("No backup files found in %q", uri.Path)
	}
	sort.Strings(files)
	glog.V(2).Infof("Loading backup file(s): %v", files)

	for _, file := range files {
		fp, err := os.Open(file)
		if err != nil {
			return err
		}
		if err = loadFn(fp, file); err != nil {
			return err
		}
		if err = fp.Close(); err != nil {
			glog.V(3).Infof("Error closing file %q: %s", file, err)
		}
	}

	return nil
}

func (h *fileHandler) Close() error {
	if h.fp == nil {
		return <-h.cerr
	}
	if err := h.fp.Sync(); err != nil {
		glog.Errorf("While closing file: %s. Error: %v", h.fp.Name(), err)
		x.Ignore(h.fp.Close())
		return err
	}
	return h.fp.Close()
}

func (h *fileHandler) Write(b []byte) (int, error) {
	return h.fp.Write(b)
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
