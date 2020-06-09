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

package worker

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

	var m Manifest
	if lastManifest == "" {
		return &m, nil
	}

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

func (h *fileHandler) GetManifests(uri *url.URL, backupId string) ([]*Manifest, error) {
	if !pathExist(uri.Path) {
		return nil, errors.Errorf("The path %q does not exist or it is inaccessible.", uri.Path)
	}

	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	paths := x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		return !isdir && strings.HasSuffix(path, suffix)
	})
	if len(paths) == 0 {
		return nil, errors.Errorf("No manifests found at path: %s", uri.Path)
	}
	sort.Strings(paths)

	// Read and filter the files to get the list of files to consider for this restore operation.

	var manifests []*Manifest
	for _, path := range paths {
		var m Manifest
		if err := h.readManifest(path, &m); err != nil {
			return nil, errors.Wrapf(err, "While reading %q", path)
		}
		m.Path = path
		manifests = append(manifests, &m)
	}
	manifests, err := filterManifests(manifests, backupId)
	if err != nil {
		return nil, err
	}

	return manifests, nil
}

// Load uses tries to load any backup files found.
// Returns the maximum value of Since on success, error otherwise.
func (h *fileHandler) Load(uri *url.URL, backupId string, fn loadFn) LoadResult {
	manifests, err := h.GetManifests(uri, backupId)
	if err != nil {
		return LoadResult{0, 0, errors.Wrapf(err, "cannot retrieve manifests")}
	}

	// Process each manifest, first check that they are valid and then confirm the
	// backup files for each group exist. Each group in manifest must have a backup file,
	// otherwise this is a failure and the user must remedy.
	var since uint64
	var maxUid uint64
	for i, manifest := range manifests {
		if manifest.Since == 0 || len(manifest.Groups) == 0 {
			continue
		}

		path := filepath.Dir(manifests[i].Path)
		for gid := range manifest.Groups {
			file := filepath.Join(path, backupName(manifest.Since, gid))
			fp, err := os.Open(file)
			if err != nil {
				return LoadResult{0, 0, errors.Wrapf(err, "Failed to open %q", file)}
			}
			defer fp.Close()

			// Only restore the predicates that were assigned to this group at the time
			// of the last backup.
			predSet := manifests[len(manifests)-1].getPredsInGroup(gid)

			groupMaxUid, err := fn(fp, gid, predSet)
			if err != nil {
				return LoadResult{0, 0, err}
			}
			if groupMaxUid > maxUid {
				maxUid = groupMaxUid
			}
		}
		since = manifest.Since
	}

	return LoadResult{since, maxUid, nil}
}

// Verify performs basic checks to decide whether the specified backup can be restored
// to a live cluster.
func (h *fileHandler) Verify(uri *url.URL, backupId string, currentGroups []uint32) error {
	manifests, err := h.GetManifests(uri, backupId)
	if err != nil {
		return errors.Wrapf(err, "while retrieving manifests")
	}

	if len(manifests) == 0 {
		return errors.Errorf("No backups with the specified backup ID %s", backupId)
	}

	return verifyGroupsInBackup(manifests, currentGroups)
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
