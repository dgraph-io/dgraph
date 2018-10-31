/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
)

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	path string
}

// Session authenticates or prepares a handler session.
// Returns error on failure, nil on success.
func (h *fileHandler) Session(_, path string) error {
	h.path = path
	return os.Chdir(h.path)
}

// List returns a list of Dgraph backup files at target.
// Returns a list (might be empty) on success, error otherwise.
func (h *fileHandler) List() ([]string, error) {
	return filepath.Glob(filepath.Join(h.path, "*"+dgraphBackupSuffix))
}

// Copy is called when we are ready to transmit a file to the target.
// Returns error on failure, nil on success.
func (h *fileHandler) Copy(in, out string) error {
	if filepath.Base(out) == out {
		out = filepath.Join(h.path, out)
	}

	if h.Exists(out) {
		glog.Errorf("File already exists on target: %q", out)
		return os.ErrExist
	}

	// if we are in the same volume, just rename. it should work on NFS too.
	if strings.HasPrefix(in, h.path) {
		glog.V(3).Infof("renaming %q to %q", in, out)
		return os.Rename(in, out)
	}

	src, err := os.Open(in)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(out)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		return err
	}

	return dst.Sync()
}

// Exists checks if a path (file or dir) is found at target.
// Returns true if found, false otherwise.
func (h *fileHandler) Exists(path string) bool {
	_, err := os.Stat(path)
	return os.IsExist(err)
}

// Register this handler
func init() {
	addSchemeHandler("file", &fileHandler{})
}
