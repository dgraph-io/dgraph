/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 */

package backup

import (
	"io"
	"os"
	"path/filepath"

	"github.com/dgraph-io/dgraph/x"
)

const dgraphBackupFullPrefix = "full-"
const dgraphBackupPartialPrefix = "part-"
const dgraphBackupSuffix = ".dgraph-backup"

type fileHandler struct {
	path string
}

func (h *fileHandler) Session(_, path string) error {
	h.path = path
	return os.Chdir(h.path)
}

func (h *fileHandler) List() ([]string, error) {
	return filepath.Glob(filepath.Join(h.path, "*"+dgraphBackupSuffix))
}

func (h *fileHandler) Copy(in, out string) error {
	if filepath.Base(out) == out {
		out = filepath.Join(h.path, out)
	}

	if h.Exists(out) {
		return x.Errorf("file already exists: %q", out)
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

func (h *fileHandler) Exists(path string) bool {
	_, err := os.Stat(path)
	return os.IsExist(err)
}
