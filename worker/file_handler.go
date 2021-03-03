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

	"github.com/dgraph-io/badger/v3"
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
	var manifest MasterManifest

	// If there is no master manifest, create one using old manifests.
	path := filepath.Join(uri.Path, backupManifest)
	if !pathExist(path) {
		m, err := h.getConsolidatedManifest(uri)
		if err != nil {
			return nil, errors.Wrap(err, "Get latest manifest failed while consolidation: ")
		}
		manifest.Manifests = m.Manifests
	} else {
		if err := h.readMasterManifest(path, &manifest); err != nil {
			return nil, errors.Wrap(err, "Get latest manifest failed to read master manifest: ")
		}
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
	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

// GetManifest returns the master manifest, if the directory doesn't contain
// a master manifest, then it will try to return a master manifest by consolidating
// the manifests.
func (h *fileHandler) GetManifest(uri *url.URL) (*MasterManifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Errorf("while GetLatestManifest: %v", err)
	}
	var manifest MasterManifest
	path := filepath.Join(uri.Path, backupManifest)
	if !pathExist(path) {
		m, err := h.getConsolidatedManifest(uri)
		if err != nil {
			return nil, errors.Wrap(err, "Get latest manifest failed while consolidation: ")
		}
		manifest.Manifests = m.Manifests
	} else {
		if err := h.readMasterManifest(path, &manifest); err != nil {
			return nil, err
		}
	}
	return &manifest, nil
}

func (h *fileHandler) GetManifests(uri *url.URL, backupId string,
	backupNum uint64) ([]*Manifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Errorf("while GetManifests: %v", err)
	}

	var manifest MasterManifest
	path := filepath.Join(uri.Path, backupManifest)
	if !pathExist(path) {
		var err error
		m, err := h.getConsolidatedManifest(uri)
		if err != nil {
			errors.Wrap(err, "Get manifests failed to get consolidated manifests: ")
		}
		manifest.Manifests = m.Manifests
	} else {
		if err := h.readMasterManifest(path, &manifest); err != nil {
			return nil, errors.Wrap(err, "Get manifests failed: ")
		}
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

		path := manifests[i].Path
		path = filepath.Join(uri.Path, path)
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
	loadFn := func(r io.Reader, groupId uint32, preds predicateSet, isOld bool) (uint64, error) {
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
			WithNumVersionsToKeep(math.MaxInt32).
			WithEncryptionKey(key).
			WithNamespaceOffset(x.NamespaceOffset))

		if err != nil {
			return 0, errors.Wrapf(err, "cannot open DB at %s", dir)
		}
		defer db.Close()
		_, _, err = loadFromBackup(db, &loadBackupInput{
			// TODO(Naman): Why is drop operations nil here?
			r: gzReader, restoreTs: 0, preds: preds, dropOperations: nil, isOld: isOld,
		})
		if err != nil {
			return 0, errors.Wrapf(err, "cannot load backup")
		}
		return 0, x.WriteGroupIdFile(dir, uint32(groupId))
	}

	// Read manifest from folder.
	manifest := &Manifest{}
	manifestPath := filepath.Join(backupDir, backupManifest)
	if err := h.readManifest(manifestPath, manifest); err != nil {
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

		_, err = loadFn(fp, gid, predSet, manifest.Version == 0)
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
				WithNumVersionsToKeep(math.MaxInt32).
				WithEncryptionKey(key))

			if err != nil {
				ch <- errors.Wrapf(err, "cannot open DB at %s", dir)
				return
			}

			req := &pb.ExportRequest{
				GroupId:     group,
				ReadTs:      manifest.Since,
				UnixTs:      time.Now().Unix(),
				Format:      format,
				Namespace:   math.MaxUint64, // Export all the namespaces.
				Destination: exportDir,
			}

			_, err = exportInternal(context.Background(), req, db, true)
			// It is important to close the db before sending err to ch. Else, we will see a memory
			// leak.
			db.Close()
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

// getConsolidatedManifest walks over all the backup directories and generates a master manifest.
func (h *fileHandler) getConsolidatedManifest(uri *url.URL) (*MasterManifest, error) {
	if err := createIfNotExists(uri.Path); err != nil {
		return nil, errors.Wrap(err, "While GetLatestManifest")
	}
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
	var manifest MasterManifest

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
