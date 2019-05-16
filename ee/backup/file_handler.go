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

// readManifest reads a manifest file at path using the handler.
// Returns nil on success, otherwise an error.
func (h *fileHandler) readManifest(path string, m *Manifest) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, m)
}

// Create prepares the a path to save backup files.
// Returns error on failure, nil on success.
func (h *fileHandler) Create(uri *url.URL, req *Request) error {
	var dir, path, fileName string

	// check that the path exists and we can access it.
	if !pathExist(uri.Path) {
		return x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// Find the max version from the latest backup. This is done only when starting a new
	// backup, not when creating a manifest.
	if req.Manifest == nil {
		// Walk the path and find the most recent backup.
		// If we can't find a manifest file, this is a full backup.
		var lastManifest string
		suffix := filepath.Join(string(filepath.Separator), backupManifest)
		_ = x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
			if !isdir && strings.HasSuffix(path, suffix) && path > lastManifest {
				lastManifest = path
			}
			return false
		})
		// Found a manifest now we extract the version to use in Backup().
		if lastManifest != "" {
			var m Manifest
			if err := h.readManifest(lastManifest, &m); err != nil {
				return err
			}

			// Return the version of last backup
			req.Version = m.Version
		}
		fileName = fmt.Sprintf(backupNameFmt, req.Backup.ReadTs, req.Backup.GroupId)
	} else {
		fileName = backupManifest
	}

	// If a full backup is being forced, force the version to zero to stream all
	// the contents from the database.
	if req.Backup.ForceFull {
		req.Version = 0
	}

	dir = filepath.Join(uri.Path, fmt.Sprintf(backupPathFmt, req.Backup.UnixTs))
	err := os.Mkdir(dir, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}

	path = filepath.Join(dir, fileName)
	h.fp, err = os.Create(path)
	if err != nil {
		return err
	}
	glog.V(2).Infof("Using file path: %q", path)

	return nil
}

// Load uses tries to load any backup files found.
// Returns nil and the maximum Ts version on success, error otherwise.
func (h *fileHandler) Load(uri *url.URL, fn loadFn) (uint64, error) {
	if !pathExist(uri.Path) {
		return 0, x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// Get a list of all the manifest files at the location.
	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	manifests := x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, suffix)
	})
	if len(manifests) == 0 {
		return 0, x.Errorf("No manifests found at path: %s", uri.Path)
	}
	sort.Strings(manifests)
	if glog.V(3) {
		fmt.Printf("Found backup manifest(s): %v\n", manifests)
	}

	// version is returned with the max manifest version found.
	var version uint64

	// Process each manifest, first check that they are valid and then confirm the
	// backup files for each group exist. Each group in manifest must have a backup file,
	// otherwise this is a failure and the user must remedy.
	for _, manifest := range manifests {
		var m Manifest
		if err := h.readManifest(manifest, &m); err != nil {
			return 0, x.Wrapf(err, "While reading %q", manifest)
		}
		if m.ReadTs == 0 || m.Version == 0 || len(m.Groups) == 0 {
			if glog.V(2) {
				fmt.Printf("Restore: skip backup: %s: %#v\n", manifest, &m)
			}
			continue
		}

		// Check the files for each group in the manifest exist.
		path := filepath.Dir(manifest)
		for _, groupId := range m.Groups {
			file := filepath.Join(path, fmt.Sprintf(backupNameFmt, m.ReadTs, groupId))
			fp, err := os.Open(file)
			if err != nil {
				return 0, x.Wrapf(err, "Failed to open %q", file)
			}
			defer fp.Close()
			if err = fn(fp, int(groupId)); err != nil {
				return 0, err
			}
		}
		version = m.Version
	}
	return version, nil
}

// ListManifests loads the manifests in the locations and returns them.
func (h *fileHandler) ListManifests(uri *url.URL) ([]string, error) {
	if !pathExist(uri.Path) {
		return nil, x.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// Get a list of all the manifest files at the location.
	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	manifests := x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, suffix)
	})
	if len(manifests) == 0 {
		return nil, x.Errorf("No manifests found at path: %s", uri.Path)
	}
	sort.Strings(manifests)
	if glog.V(3) {
		fmt.Printf("Found backup manifest(s): %v\n", manifests)
	}
	return manifests, nil
}

func (h *fileHandler) ReadManifest(path string, m *Manifest) error {
	return h.readManifest(path, m)
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

// pathExist checks if a path (file or dir) is found at target.
// Returns true if found, false otherwise.
func pathExist(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err) && !os.IsPermission(err)
}
