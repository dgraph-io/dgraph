/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"os"
	"path/filepath"

	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
)

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	*session
	fp *os.File
}

// New creates a new instance of this handler.
func (h *fileHandler) New() handler {
	return &fileHandler{}
}

// Open authenticates or prepares a handler session.
// Returns error on failure, nil on success.
func (h *fileHandler) Open(s *session) error {
	// check that this path exists and we can access it.
	if !h.Exists(s.path) {
		return x.Errorf("The path %q does not exist or it is inaccessible.", s.path)
	}
	path := filepath.Join(s.path, s.file)
	fp, err := os.Create(path)
	if err != nil {
		return err
	}
	glog.V(2).Infof("using file path: %q", path)
	h.fp = fp
	h.session = s
	return nil
}

func (h *fileHandler) Close() error {
	if err := h.fp.Sync(); err != nil {
		return err
	}
	return h.fp.Close()
}

func (h *fileHandler) Write(b []byte) (int, error) {
	return h.fp.Write(b)
}

// Exists checks if a path (file or dir) is found at target.
// Returns true if found, false otherwise.
func (h *fileHandler) Exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err) && !os.IsPermission(err)
}

// Register this handler
func init() {
	addHandler("file", &fileHandler{})
}
