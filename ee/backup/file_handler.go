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

	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	fp      *os.File
	readers multiReader
}

type multiReader []io.Reader

func (mr multiReader) Close() error {
	var err error
	for i := range mr {
		if mr[i] == nil {
			continue
		}
		if fp, ok := mr[i].(*os.File); ok {
			err = fp.Close()
		}
	}
	return err
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

// Load uses tries to load any backup files found.
// Returns nil on success, error otherwise.
func (h *fileHandler) Load(uri *url.URL) (io.Reader, error) {
	if !h.exists(uri.Path) {
		return nil, x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// find files and sort.
	files := x.FindFilesFunc(uri.Path, func(path string) bool {
		return strings.HasSuffix(path, ".backup")
	})
	if len(files) == 0 {
		return nil, x.Errorf("No backup files found in %q", uri.Path)
	}
	sort.Strings(files)
	glog.V(2).Infof("Loading backup file(s): %v", files)

	h.readers = make(multiReader, 0, len(files))
	for _, file := range files {
		fp, err := os.Open(file)
		if err != nil {
			h.readers.Close()
			return nil, x.Errorf("Error opening %q: %s", file, err)
		}
		h.readers = append(h.readers, fp)
	}
	return io.MultiReader(h.readers...), nil
}

func (h *fileHandler) Close() error {
	h.readers.Close()
	if h.fp == nil {
		return nil
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
