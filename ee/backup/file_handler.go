/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
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

	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	fp *os.File
}

// Open authenticates or prepares a handler session.
// Returns error on failure, nil on success.
func (h *fileHandler) Open(uri *url.URL, req *Request) error {
	// check that this path exists and we can access it.
	if !h.exists(uri.Path) {
		return x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

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
	h.fp = fp
	return nil
}

func (h *fileHandler) Close() error {
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
