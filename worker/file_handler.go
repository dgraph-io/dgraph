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

// readMasterManifest reads the master manifest file at path using the handler.
// Returns nil on success, otherwise an error.
func (h *fileHandler) readMasterManifest(path string, m *MasterManifest) error {
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
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Wrap(err, "Get latest manifest failed:")
	}

	manifest, err := h.getConsolidatedManifest(uri)
	if err != nil {
		return nil, errors.Wrap(err, "Get latest manifest failed while consolidation: ")
	}
	if len(manifest.Manifests) == 0 {
		return &Manifest{}, nil
	}
	return manifest.Manifests[len(manifest.Manifests)-1], nil
}

func createIfNotExists(path string) error {
	if pathExist(path) {
		return nil
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return errors.Errorf("The path %q does not exist or it is inaccessible."+
			" While trying to create it, got error: %v", path, err)
	}
	return nil
}

// CreateBackupFile prepares the a path to save the backup file.
func (h *fileHandler) CreateBackupFile(uri *url.URL, req *pb.BackupRequest) error {
	if err := createIfNotExists(uri.Path); err != nil {
		return errors.Errorf("while CreateBackupFile: %v", err)
	}

	fileName := backupName(req.ReadTs, req.GroupId)
	return h.createFiles(uri, req, fileName)
}

// CreateManifest completes the backup by writing the manifest to a file.
func (h *fileHandler) CreateManifest(uri *url.URL, manifest *MasterManifest) error {
	var err error
	if err = createIfNotExists(uri.Path); err != nil {
		return errors.Errorf("while WriteManifest: %v", err)
	}

	tmpPath := filepath.Join(uri.Path, tmpManifest)
	if h.fp, err = os.Create(tmpPath); err != nil {
		return err
	}
	if err = json.NewEncoder(h).Encode(manifest); err != nil {
		return err
	}

	// Move the tmpManifest to backupManifest
	path := filepath.Join(uri.Path, backupManifest)
	return os.Rename(tmpPath, path)
}

// GetManifest returns the master manifest, if the directory doesn't contain
// a master manifest, then it will try to return a master manifest by consolidating
// the manifests.
func (h *fileHandler) GetManifest(uri *url.URL) (*MasterManifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Errorf("while GetLatestManifest: %v", err)
	}
	manifest, err := h.getConsolidatedManifest(uri)
	if err != nil {
		return manifest, errors.Wrap(err, "GetManifest failed to get consolidated manifest: ")
	}
	return manifest, nil
}

func (h *fileHandler) GetManifests(uri *url.URL, backupId string,
	backupNum uint64) ([]*Manifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Errorf("while GetManifests: %v", err)
	}

	manifest, err := h.getConsolidatedManifest(uri)
	if err != nil {
		return manifest.Manifests, errors.Wrap(err, "GetManifests failed to get consolidated manifest: ")
	}

	var filtered []*Manifest
	for _, m := range manifest.Manifests {
		path := filepath.Join(uri.Path, m.Path)
		if pathExist(path) {
			filtered = append(filtered, m)
		}
	}

	return getManifests(filtered, backupId, backupNum)
}

// Load uses tries to load any backup files found.
// Returns the maximum value of Since on success, error otherwise.
func (h *fileHandler) Load(uri *url.URL, backupId string, backupNum uint64, fn loadFn) LoadResult {
	manifests, err := h.GetManifests(uri, backupId, backupNum)
	if err != nil {
		return LoadResult{Err: errors.Wrapf(err, "cannot retrieve manifests")}
	}

	// Process each manifest, first check that they are valid and then confirm the
	// backup files for each group exist. Each group in manifest must have a backup file,
	// otherwise this is a failure and the user must remedy.
	var since uint64
	var maxUid, maxNsId uint64
	for i, manifest := range manifests {
		if manifest.Since == 0 || len(manifest.Groups) == 0 {
			continue
		}

		path := filepath.Join(uri.Path, manifests[i].Path)
		for gid := range manifest.Groups {
			file := filepath.Join(path, backupName(manifest.Since, gid))
			fp, err := os.Open(file)
			if err != nil {
				return LoadResult{Err: errors.Wrapf(err, "Failed to open %q", file)}
			}
			defer fp.Close()

			// Only restore the predicates that were assigned to this group at the time
			// of the last backup.
			predSet := manifests[len(manifests)-1].getPredsInGroup(gid)

			groupMaxUid, groupMaxNsId, err := fn(gid,
				&loadBackupInput{r: fp, preds: predSet, dropOperations: manifest.DropOperations,
					isOld: manifest.Version == 0})
			if err != nil {
				return LoadResult{Err: err}
			}
			maxUid = x.Max(maxUid, groupMaxUid)
			maxNsId = x.Max(maxNsId, groupMaxNsId)
		}
		since = manifest.Since
	}

	return LoadResult{Version: since, MaxLeaseUid: maxUid, MaxLeaseNsId: maxNsId}
}

// Verify performs basic checks to decide whether the specified backup can be restored
// to a live cluster.
func (h *fileHandler) Verify(uri *url.URL, req *pb.RestoreRequest, currentGroups []uint32) error {
	manifests, err := h.GetManifests(uri, req.GetBackupId(), req.GetBackupNum())
	if err != nil {
		return errors.Wrapf(err, "while retrieving manifests")
	}
	return verifyRequest(req, manifests, currentGroups)
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

// getConsolidatedManifest walks over all the backup directories and generates a master manifest.
func (h *fileHandler) getConsolidatedManifest(uri *url.URL) (*MasterManifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Wrap(err, "While GetLatestManifest")
	}

	var manifest MasterManifest

	// If there is a master manifest already, we just return it.
	path := filepath.Join(uri.Path, backupManifest)
	if pathExist(path) {
		if err := h.readMasterManifest(path, &manifest); err != nil {
			return nil, errors.Wrap(err, "Get latest manifest failed to read master manifest: ")
		}
		return &manifest, nil
	}

	// Otherwise, we create a master manifest by going through all the backup directories.
	var paths []string
	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	_ = x.WalkPathFunc(uri.Path, func(path string, isdir bool) bool {
		if !isdir && strings.HasSuffix(path, suffix) {
			paths = append(paths, path)
		}
		return false
	})

	sort.Strings(paths)
	var mlist []*Manifest

	for _, path := range paths {
		var m Manifest
		if err := h.readManifest(path, &m); err != nil {
			return nil, errors.Wrap(err, "While Getting latest manifest")
		}
		path = filepath.Dir(path)
		_, path = filepath.Split(path)
		m.Path = path
		mlist = append(mlist, &m)
	}
	manifest.Manifests = mlist
	return &manifest, nil
}
