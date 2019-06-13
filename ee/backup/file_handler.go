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

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/pkg/errors"
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

func (h *fileHandler) createFiles(uri *url.URL, req *pb.BackupRequest, fileName string) error {
	var dir, path string

	dir = filepath.Join(uri.Path, fmt.Sprintf(backupPathFmt, req.UnixTs))
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

// GetLatestManifest reads the manifests at the given URL and returns the
// latest manifest.
func (h *fileHandler) GetLatestManifest(uri *url.URL) (*Manifest, error) {
	if !pathExist(uri.Path) {
		return nil, errors.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	// Find the max Since value from the latest backup.
	var lastManifest string
	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	_ = x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		if !isdir && strings.HasSuffix(path, suffix) && path > lastManifest {
			lastManifest = path
		}
		return false
	})

	if lastManifest == "" {
		return nil, nil
	}

	var m Manifest
	if err := h.readManifest(lastManifest, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// CreateBackupFile prepares the a path to save the backup file.
func (h *fileHandler) CreateBackupFile(uri *url.URL, req *pb.BackupRequest) error {
	if !pathExist(uri.Path) {
		return errors.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	fileName := backupName(req.ReadTs, req.GroupId)
	return h.createFiles(uri, req, fileName)
}

// CreateManifest completes the backup by writing the manifest to a file.
func (h *fileHandler) CreateManifest(uri *url.URL, req *pb.BackupRequest) error {
	if !pathExist(uri.Path) {
		return errors.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	return h.createFiles(uri, req, backupManifest)
}

// Load uses tries to load any backup files found.
// Returns the maximum value of Since on success, error otherwise.
func (h *fileHandler) Load(uri *url.URL, lastDir string, fn loadFn) (uint64, error) {
	if !pathExist(uri.Path) {
		return 0, errors.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	paths := x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, suffix)
	})
	if len(paths) == 0 {
		return 0, errors.Errorf("No manifests found at path: %s", uri.Path)
	}
	sort.Strings(paths)
	if glog.V(3) {
		fmt.Printf("Found backup manifest(s): %v\n", paths)
	}

	// Read and filter the files to get the list of files to consider
	// for this restore operation.
	var manifests []*Manifest
	for _, path := range paths {
		var m Manifest
		if err := h.readManifest(path, &m); err != nil {
			return 0, errors.Wrapf(err, "While reading %q", path)
		}
		m.Path = path
		manifests = append(manifests, &m)
	}
	manifests, err := filterManifests(manifests, lastDir)
	if err != nil {
		return 0, err
	}

	// Process each manifest, first check that they are valid and then confirm the
	// backup files for each group exist. Each group in manifest must have a backup file,
	// otherwise this is a failure and the user must remedy.
	var since uint64
	for i, manifest := range manifests {
		if manifest.Since == 0 || len(manifest.Groups) == 0 {
			if glog.V(2) {
				fmt.Printf("Restore: skip backup: %#v\n", manifest)
			}
			continue
		}

		path := filepath.Dir(manifests[i].Path)
		for _, groupId := range manifest.Groups {
			file := filepath.Join(path, backupName(manifest.Since, groupId))
			fp, err := os.Open(file)
			if err != nil {
				return 0, errors.Wrapf(err, "Failed to open %q", file)
			}
			defer fp.Close()
			if err = fn(fp, int(groupId)); err != nil {
				return 0, err
			}
		}
		since = manifest.Since
	}
	return since, nil
}

// ListManifests loads the manifests in the locations and returns them.
func (h *fileHandler) ListManifests(uri *url.URL) ([]string, error) {
	if !pathExist(uri.Path) {
		return nil, errors.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	manifests := x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, suffix)
	})
	if len(manifests) == 0 {
		return nil, errors.Errorf("No manifests found at path: %s", uri.Path)
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
