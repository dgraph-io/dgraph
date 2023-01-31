//go:build !oss
// +build !oss

/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

func verifyManifests(manifests []*Manifest) error {
	if len(manifests) == 0 {
		return nil
	}

	lastIndex := len(manifests) - 1
	if manifests[lastIndex].BackupNum != 1 {
		return errors.Errorf("expected a BackupNum value of 1 for first manifest but got %d",
			manifests[lastIndex].BackupNum)
	}

	backupId := manifests[lastIndex].BackupId
	backupNum := uint64(len(manifests))
	for _, manifest := range manifests {
		if manifest.BackupId != backupId {
			return errors.Errorf("found a manifest with backup ID %s but expected %s",
				manifest.BackupId, backupId)
		}

		if manifest.BackupNum != backupNum {
			return errors.Errorf("found a manifest with backup number %d but expected %d",
				manifest.BackupNum, backupNum)
		}
		backupNum--
	}

	return nil
}

func getManifestsToRestore(
	h UriHandler, uri *url.URL, req *pb.RestoreRequest) ([]*Manifest, error) {

	if !h.DirExists("") {
		return nil, errors.Errorf("getManifestsToRestore: The uri path: %q doesn't exists",
			uri.Path)
	}
	manifest, err := getConsolidatedManifest(h, uri)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get consolidated manifest")
	}
	return getFilteredManifests(h, manifest.Manifests, req)
}

func getFilteredManifests(h UriHandler, manifests []*Manifest,
	req *pb.RestoreRequest) ([]*Manifest, error) {

	// filter takes a list of manifests and returns the list of manifests
	// that should be considered during a restore.
	filter := func(manifests []*Manifest, backupId string) ([]*Manifest, error) {
		// Go through the files in reverse order and stop when the latest full backup is found.
		var out []*Manifest
		for i := len(manifests) - 1; i >= 0; i-- {
			// If backupId is not empty, skip all the manifests that do not match the given
			// backupId. If it's empty, do not skip any manifests as the default behavior is
			// to restore the latest series of backups.
			if len(backupId) > 0 && manifests[i].BackupId != backupId {
				continue
			}

			out = append(out, manifests[i])
			if manifests[i].Type == "full" {
				break
			}
		}

		if err := verifyManifests(out); err != nil {
			return nil, err
		}
		return out, nil
	}

	// validManifests are the ones for which the corresponding backup files exists.
	var validManifests []*Manifest
	for _, m := range manifests {
		missingFiles := false
		for g := range m.Groups {
			path := filepath.Join(m.Path, backupName(m.ValidReadTs(), g))
			if !h.FileExists(path) {
				missingFiles = true
				break
			}
		}
		if !missingFiles {
			validManifests = append(validManifests, m)
		}
	}
	manifests, err := filter(validManifests, req.BackupId)
	if err != nil {
		return nil, err
	}

	if req.BackupNum > 0 {
		if len(manifests) < int(req.BackupNum) {
			return nil, errors.Errorf("not enough backups to restore manifest with backupNum %d",
				req.BackupNum)
		}
		manifests = manifests[len(manifests)-int(req.BackupNum):]
	}
	return manifests, nil
}

// getConsolidatedManifest walks over all the backup directories and generates a master manifest.
func getConsolidatedManifest(h UriHandler, uri *url.URL) (*MasterManifest, error) {
	// If there is a master manifest already, we just return it.
	if h.FileExists(backupManifest) {
		manifest, err := readMasterManifest(h, backupManifest)
		if err != nil {
			return &MasterManifest{}, errors.Wrap(err, "failed to read master manifest: ")
		}
		return manifest, nil
	}

	// Otherwise, we create a master manifest by going through all the backup directories.
	paths := h.ListPaths("")

	var manifestPaths []string
	suffix := filepath.Join(string(filepath.Separator), backupManifest)
	for _, p := range paths {
		if strings.HasSuffix(p, suffix) {
			manifestPaths = append(manifestPaths, p)
		}
	}

	sort.Strings(manifestPaths)
	var mlist []*Manifest

	for _, path := range manifestPaths {
		path = filepath.Dir(path)
		_, path = filepath.Split(path)
		m, err := readManifest(h, filepath.Join(path, backupManifest))
		if err != nil {
			return nil, errors.Wrap(err, "while Getting latest manifest: ")
		}
		m.Path = path
		mlist = append(mlist, m)
	}
	return &MasterManifest{Manifests: mlist}, nil
}

func readManifest(h UriHandler, path string) (*Manifest, error) {
	var m Manifest
	b, err := h.Read(path)
	if err != nil {
		return &m, errors.Wrap(err, "readManifest failed to read the file: ")
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return &m, errors.Wrap(err, "readManifest failed to unmarshal: ")
	}
	return &m, nil
}

func GetLatestManifest(h UriHandler, uri *url.URL) (*Manifest, error) {
	manifest, err := GetManifest(h, uri)
	if err != nil {
		return &Manifest{}, errors.Wrap(err, "failed to get the manifest: ")
	}
	if len(manifest.Manifests) == 0 {
		return &Manifest{}, nil
	}
	return manifest.Manifests[len(manifest.Manifests)-1], nil
}

func readMasterManifest(h UriHandler, path string) (*MasterManifest, error) {
	var m MasterManifest
	b, err := h.Read(path)
	if err != nil {
		return &m, errors.Wrap(err, "readMasterManifest failed to read the file: ")
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return &m, errors.Wrap(err, "readMasterManifest failed to unmarshal: ")
	}
	return &m, nil
}

func GetManifest(h UriHandler, uri *url.URL) (*MasterManifest, error) {
	if !h.DirExists("") {
		return &MasterManifest{}, errors.Errorf("getManifest: The uri path: %q doesn't exists",
			uri.Path)
	}
	manifest, err := getConsolidatedManifest(h, uri)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get consolidated manifest: ")
	}
	return manifest, nil
}

func createManifest(h UriHandler, uri *url.URL, manifest *MasterManifest) error {
	w, err := h.CreateFile(tmpManifest)
	if err != nil {
		return errors.Wrap(err, "createManifest failed to create tmp path: ")
	}
	if err = json.NewEncoder(w).Encode(manifest); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	// Move the tmpManifest to backupManifest, this operation is not atomic for s3.
	// We try our best to move the file but if it fails then the user must move it manually.
	err = h.Rename(tmpManifest, backupManifest)
	return errors.Wrapf(err, "MOVING TEMPORARY MANIFEST TO MAIN MANIFEST FAILED!\n"+
		"It is possible that the manifest would have been corrupted. You must move "+
		"the file: %s to: %s in order to "+
		"fix the backup manifest.", tmpManifest, backupManifest)
}

// ListBackupManifests scans location l for backup files and returns the list of manifests.
func ListBackupManifests(l string, creds *x.MinioCredentials) ([]*Manifest, error) {
	uri, err := url.Parse(l)
	if err != nil {
		return nil, err
	}

	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return nil, errors.Wrap(err, "error in listBackupManifests")
	}

	m, err := GetManifest(h, uri)
	if err != nil {
		return nil, err
	}
	return m.Manifests, nil
}
