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
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"github.com/pkg/errors"
)

// fileHandler is used for 'file:' URI scheme.
type fileHandler struct {
	fp *os.File
}

// BackupExporter is an alias of fileHandler so that this struct can be used
// by the export_backup command.
type BackupExporter struct {
	fileHandler
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

func (h *fileHandler) GetManifests(uri *url.URL, backupId string,
	backupNum uint64) ([]*Manifest, error) {
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

	return getManifests(manifests, backupId, backupNum)
}

// Load uses tries to load any backup files found.
// Returns the maximum value of Since on success, error otherwise.
func (h *fileHandler) Load(uri *url.URL, backupId string, backupNum uint64, fn loadFn) LoadResult {
	manifests, err := h.GetManifests(uri, backupId, backupNum)
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
func (h *fileHandler) Verify(uri *url.URL, req *pb.RestoreRequest, currentGroups []uint32) error {
	manifests, err := h.GetManifests(uri, req.GetBackupId(), req.GetBackupNum())
	if err != nil {
		return errors.Wrapf(err, "while retrieving manifests")
	}
	return verifyRequest(req, manifests, currentGroups)
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

func (h *fileHandler) ExportBackup(backupDir, exportDir, format string,
	key x.SensitiveByteSlice) error {
	if format != "json" && format != "rdf" {
		return errors.Errorf("invalid format %s", format)
	}

	// Create exportDir and temporary folder to store the restored backup.
	var err error
	exportDir, err = filepath.Abs(exportDir)
	if err != nil {
		return errors.Wrapf(err, "cannot convert path %s to absolute path", exportDir)
	}
	if err := os.MkdirAll(exportDir, 0755); err != nil {
		return errors.Wrapf(err, "cannot create dir %s", exportDir)
	}
	tmpDir, err := ioutil.TempDir("", "export_backup")
	if err != nil {
		return errors.Wrapf(err, "cannot create temp dir")
	}

	// Function to load the a single backup file.
	loadFn := func(r io.Reader, groupId uint32, preds predicateSet) (uint64, error) {
		dir := filepath.Join(tmpDir, fmt.Sprintf("p%d", groupId))

		r, err := enc.GetReader(key, r)
		if err != nil {
			return 0, err
		}

		gzReader, err := gzip.NewReader(r)
		if err != nil {
			if len(key) != 0 {
				err = errors.Wrap(err,
					"Unable to read the backup. Ensure the encryption key is correct.")
			}
			return 0, errors.Wrapf(err, "cannot create gzip reader")
		}
		// The badger DB should be opened only after creating the backup
		// file reader and verifying the encryption in the backup file.
		db, err := badger.OpenManaged(badger.DefaultOptions(dir).
			WithSyncWrites(false).
			WithValueThreshold(1 << 10).
			WithNumVersionsToKeep(math.MaxInt32).
			WithEncryptionKey(key))

		if err != nil {
			return 0, errors.Wrapf(err, "cannot open DB at %s", dir)
		}
		defer db.Close()
		_, err = loadFromBackup(db, gzReader, 0, preds)
		if err != nil {
			return 0, errors.Wrapf(err, "cannot load backup")
		}
		return 0, x.WriteGroupIdFile(dir, uint32(groupId))
	}

	// Read manifest from folder.
	manifest := &Manifest{}
	manifestPath := filepath.Join(backupDir, backupManifest)
	if err := h.ReadManifest(manifestPath, manifest); err != nil {
		return errors.Wrapf(err, "cannot read manifest at %s", manifestPath)
	}
	manifest.Path = manifestPath
	if manifest.Since == 0 || len(manifest.Groups) == 0 {
		return errors.Errorf("no data found in backup")
	}

	// Restore backup to disk.
	for gid := range manifest.Groups {
		file := filepath.Join(backupDir, backupName(manifest.Since, gid))
		fp, err := os.Open(file)
		if err != nil {
			return errors.Wrapf(err, "cannot open backup file at %s", file)
		}
		defer fp.Close()

		// Only restore the predicates that were assigned to this group at the time
		// of the last backup.
		predSet := manifest.getPredsInGroup(gid)

		_, err = loadFn(fp, gid, predSet)
		if err != nil {
			return err
		}
	}

	// Export the data from the p directories produced by the last step.
	ch := make(chan error, len(manifest.Groups))
	for gid := range manifest.Groups {
		go func(group uint32) {
			dir := filepath.Join(tmpDir, fmt.Sprintf("p%d", group))
			db, err := badger.OpenManaged(badger.DefaultOptions(dir).
				WithSyncWrites(false).
				WithValueThreshold(1 << 10).
				WithNumVersionsToKeep(math.MaxInt32).
				WithEncryptionKey(key))

			if err != nil {
				ch <- errors.Wrapf(err, "cannot open DB at %s", dir)
				return
			}
			defer db.Close()

			req := &pb.ExportRequest{
				GroupId:     group,
				ReadTs:      manifest.Since,
				UnixTs:      time.Now().Unix(),
				Format:      format,
				Destination: exportDir,
			}

			_, err = exportInternal(context.Background(), req, db, true)
			ch <- errors.Wrapf(err, "cannot export data inside DB at %s", dir)
		}(gid)
	}

	for i := 0; i < len(manifest.Groups); i++ {
		err := <-ch
		if err != nil {
			return err
		}
	}

	// Clean up temporary directory.
	if err := os.RemoveAll(tmpDir); err != nil {
		return errors.Wrapf(err, "cannot remove temp directory at %s", tmpDir)
	}

	return nil
}
