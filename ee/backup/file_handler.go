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
		fmt.Sprintf(backupFmt, req.Backup.ReadTs, req.Backup.GroupId))
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
func (h *fileHandler) Load(uri *url.URL, since uint64, fn loadFn) error {
	if !h.exists(uri.Path) {
		return x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// find files and sort.
	files := x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, ".backup")
	})
	if len(files) == 0 {
		return x.Errorf("No backup files found in %q", uri.Path)
	}
	sort.Strings(files)
	glog.V(2).Infof("Loading backup file(s): %v", files)

	for _, file := range files {
		readTs, groupId, err := getInfo(file)
		if err != nil {
			if glog.V(2) {
				fmt.Printf("--- Skip: invalid backup name format: %q\n", file)
			}
			continue
		}
		// if we are doing a partial restore, check the backup readTs here.
		if since != 0 && readTs < since {
			if glog.V(2) {
				fmt.Printf("--- Skip: readTs too old (%d < %d): %q\n", since, readTs, file)
			}
			continue
		}
		fp, err := os.Open(file)
		if err != nil {
			return x.Errorf("Error opening %q: %s", file, err)
		}
		defer fp.Close()
		if err = fn(fp, groupId); err != nil {
			return err
		}
	}
	return nil
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
