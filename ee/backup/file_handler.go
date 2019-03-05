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
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	fp *os.File
}

// Create prepares the a path to save backup files.
// Returns error on failure, nil on success.
func (h *fileHandler) Create(uri *url.URL, req *Request) error {
	var dir, path, fileName string

	// check that the path exists and we can access it.
	if !h.exists(uri.Path) {
		return x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// Find the max version from the latest backup. This is done only when starting a new
	// backup, not when creating a manifest.
	if req.Manifest == nil {
		// Walk the path and find the most recent backup.
		// If we can't find a manifest file, this is a full backup.
		var lastManifest string
		suffix := "/" + backupManifest
		_ = x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
			if !isdir && strings.HasSuffix(path, suffix) && path > lastManifest {
				lastManifest = path
			}
			return false
		})
		// Found a manifest now we extract the version to use in Backup().
		if lastManifest != "" {
			var m Manifest
			b, err := ioutil.ReadFile(lastManifest)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(b, &m); err != nil {
				return err
			}
			// No new changes since last check
			if m.Version == req.Backup.SnapshotTs {
				return ErrBackupNoChanges
			}
			// Return the version of last backup
			req.Version = m.Version
		}
		fileName = fmt.Sprintf(backupNameFmt, req.Backup.ReadTs, req.Backup.GroupId)
	} else {
		fileName = backupManifest
	}

	dir = filepath.Join(uri.Path, fmt.Sprintf(backupPathFmt, req.Backup.UnixTs))
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return err
	}

	path = filepath.Join(dir, fileName)
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
func (h *fileHandler) Load(uri *url.URL, fn loadFn) error {
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
	if glog.V(3) {
		fmt.Printf("Loading backup file(s): %v\n", files)
	}

	for _, file := range files {
		_, groupId, err := getInfo(file)
		if err != nil {
			if glog.V(2) {
				fmt.Printf("Restore: skip invalid backup name format: %q\n", file)
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
