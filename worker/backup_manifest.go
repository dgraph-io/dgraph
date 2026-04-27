/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	"github.com/dgraph-io/dgraph/v25/x"
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

func getManifestsToRestore(h UriHandler, uri *url.URL, req *pb.RestoreRequest) ([]*Manifest, error) {
	manifest, err := GetManifest(h, uri)
	if err != nil {
		return nil, err
	}
	manifests := manifest.Manifests

	// filter takes a list of manifests and returns the list of manifests
	// that should be considered during a restore.
	filter := func(mfs []*Manifest, backupId string) ([]*Manifest, error) {
		// Go through the files in reverse order and stop when the latest full backup is found.
		var out []*Manifest
		for i := len(mfs) - 1; i >= 0; i-- {
			// If backupId is not empty, skip all the manifests that do not match the given
			// backupId. If it's empty, do not skip any mfs manifests the default behavior is
			// to restore the latest series of backups.
			if len(backupId) > 0 && mfs[i].BackupId != backupId {
				continue
			}

			out = append(out, mfs[i])
			if mfs[i].Type == "full" {
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
				glog.Warningf("backup file [%v] missing for backupId [%v] and backupNum [%v]",
					path, m.BackupId, m.BackupNum)
				missingFiles = true
				break
			}
		}
		if !missingFiles {
			validManifests = append(validManifests, m)
		}
	}

	manifests, err = filter(validManifests, req.BackupId)
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

// upgradeManifest updates the in-memory manifest from various versions to the latest version.
// If the manifest version is 0 (dgraph version < v21.03), attach namespace to the predicates and
// the drop data/attr operation.
// If the manifest version is 2103, convert the format of predicate from <ns bytes>|<attr> to
// <ns string>-<attr>. This is because of a bug for namespace greater than 127.
// See https://github.com/dgraph-io/dgraph/pull/7810
// NOTE: Do not use the upgraded manifest to overwrite the non-upgraded manifest.
func upgradeManifest(m *Manifest) error {
	switch m.Version {
	case 0:
		for gid, preds := range m.Groups {
			parsedPreds := preds[:0]
			for _, pred := range preds {
				parsedPreds = append(parsedPreds, x.AttrInRootNamespace(pred))
			}
			m.Groups[gid] = parsedPreds
		}
		for _, op := range m.DropOperations {
			switch op.DropOp {
			case pb.DropOperation_DATA:
				op.DropValue = fmt.Sprintf("%#x", x.RootNamespace)
			case pb.DropOperation_ATTR:
				op.DropValue = x.AttrInRootNamespace(op.DropValue)
			default:
				// do nothing for drop all and drop namespace.
			}
		}
	case 2103:
		for gid, preds := range m.Groups {
			parsedPreds := preds[:0]
			for _, pred := range preds {
				ns_attr, err := x.AttrFrom2103(pred)
				if err != nil {
					return errors.Errorf("while parsing predicate got: %q", err)
				}
				parsedPreds = append(parsedPreds, ns_attr)
			}
			m.Groups[gid] = parsedPreds
		}
		for _, op := range m.DropOperations {
			// We have a cluster wide drop data in v21.03.
			if op.DropOp == pb.DropOperation_ATTR {
				ns_attr, err := x.AttrFrom2103(op.DropValue)
				if err != nil {
					return errors.Errorf("while parsing the drop operation %+v got: %q",
						op, err)
				}
				op.DropValue = ns_attr
			}
		}
	case 2105:
		// pass
	}
	return nil
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

// GetManifestNoUpgrade returns the master manifest using the given handler and uri.
func GetManifestNoUpgrade(h UriHandler, uri *url.URL) (*MasterManifest, error) {
	if !h.DirExists("") {
		return &MasterManifest{},
			errors.Errorf("getManifestWithoutUpgrade: The uri path: %q doesn't exists", uri.Path)
	}
	manifest, err := getConsolidatedManifest(h, uri)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get consolidated manifest: ")
	}
	return manifest, nil
}

// GetManifest returns the master manifest using the given handler and uri. Additionally, it also
// upgrades the manifest for the in-memory processing.
// Note: This function must not be used when using the returned manifest for the purpose of
// overwriting the old manifest.
func GetManifest(h UriHandler, uri *url.URL) (*MasterManifest, error) {
	manifest, err := GetManifestNoUpgrade(h, uri)
	if err != nil {
		return manifest, err
	}
	for _, m := range manifest.Manifests {
		if err := upgradeManifest(m); err != nil {
			return manifest, errors.Wrapf(err, "getManifest: failed to upgrade")
		}
	}
	return manifest, nil
}

func CreateManifest(h UriHandler, uri *url.URL, manifest *MasterManifest) error {
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

// BackupDateFilter holds optional date-range criteria for listing backups.
type BackupDateFilter struct {
	Since *time.Time
	Until *time.Time
}

// parseBackupTime extracts the UTC timestamp from a backup directory path like
// "dgraph.20260101.120000.000".
func parseBackupTime(path string) (time.Time, error) {
	const prefix = "dgraph."
	if !strings.HasPrefix(path, prefix) {
		return time.Time{}, fmt.Errorf("unexpected backup path format: %s", path)
	}
	t, err := time.Parse("20060102.150405.000", strings.TrimPrefix(path, prefix))
	return t.UTC(), err
}

// FilterManifestsByDate returns only the manifests whose path timestamps fall
// within the inclusive [filter.Since, filter.Until] window. Manifests whose
// path cannot be parsed are always included (fail-open for old path formats).
func FilterManifestsByDate(manifests []*Manifest, filter BackupDateFilter) []*Manifest {
	if filter.Since == nil && filter.Until == nil {
		return manifests
	}
	var out []*Manifest
	for _, m := range manifests {
		t, err := parseBackupTime(m.Path)
		if err != nil {
			glog.Warningf("FilterManifestsByDate: cannot parse backup path %q, including it: %v", m.Path, err)
			out = append(out, m)
			continue
		}
		if filter.Since != nil && t.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && t.After(*filter.Until) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// BackupListStats holds summary statistics derived from a manifest list.
type BackupListStats struct {
	Total             int
	LastFullBackup    *time.Time
	LastIncrBackup    *time.Time
	OldestBackup      *time.Time
	NewestBackup      *time.Time
	BackupSeriesCount int
}

// ComputeBackupListStats derives statistics from a list of manifests.
func ComputeBackupListStats(manifests []*Manifest) BackupListStats {
	stats := BackupListStats{Total: len(manifests)}
	ids := make(map[string]struct{})
	for _, m := range manifests {
		if m.BackupId != "" {
			ids[m.BackupId] = struct{}{}
		}
		t, err := parseBackupTime(m.Path)
		if err != nil {
			continue
		}
		tc := t
		if stats.OldestBackup == nil || t.Before(*stats.OldestBackup) {
			stats.OldestBackup = &tc
		}
		if stats.NewestBackup == nil || t.After(*stats.NewestBackup) {
			stats.NewestBackup = &tc
		}
		if m.Type == "full" && (stats.LastFullBackup == nil || t.After(*stats.LastFullBackup)) {
			stats.LastFullBackup = &tc
		}
		if m.Type == "incremental" && (stats.LastIncrBackup == nil || t.After(*stats.LastIncrBackup)) {
			stats.LastIncrBackup = &tc
		}
	}
	stats.BackupSeriesCount = len(ids)
	return stats
}

// readMasterManifestSummary reads the lightweight manifest_summary.json file.
func readMasterManifestSummary(h UriHandler) (*MasterManifestSummary, error) {
	var ms MasterManifestSummary
	b, err := h.Read(backupManifestSummary)
	if err != nil {
		return nil, errors.Wrap(err, "readMasterManifestSummary failed to read")
	}
	if err := json.Unmarshal(b, &ms); err != nil {
		return nil, errors.Wrap(err, "readMasterManifestSummary failed to unmarshal")
	}
	return &ms, nil
}

// summariesToManifests converts lightweight summaries back to full Manifest structs
// (with nil Groups/DropOperations) so the listing path is type-compatible.
func summariesToManifests(summaries []*ManifestSummary) []*Manifest {
	out := make([]*Manifest, len(summaries))
	for i, s := range summaries {
		out[i] = &Manifest{ManifestBase: s.ManifestBase}
	}
	return out
}

// CreateManifestSummary writes a lightweight manifest_summary.json that omits
// Groups and DropOperations. The full manifest.json is left untouched.
func CreateManifestSummary(h UriHandler, master *MasterManifest) (retErr error) {
	summary := &MasterManifestSummary{
		Manifests: make([]*ManifestSummary, len(master.Manifests)),
	}
	for i, m := range master.Manifests {
		summary.Manifests[i] = &ManifestSummary{ManifestBase: m.ManifestBase}
	}
	w, err := h.CreateFile(tmpManifestSummary)
	if err != nil {
		return errors.Wrap(err, "createManifestSummary failed to create tmp file")
	}
	defer func() {
		if err := w.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()
	if err = json.NewEncoder(w).Encode(summary); err != nil {
		return err
	}
	return errors.Wrapf(h.Rename(tmpManifestSummary, backupManifestSummary),
		"MOVING TEMPORARY SUMMARY MANIFEST FAILED! Move %s to %s manually.",
		tmpManifestSummary, backupManifestSummary)
}

// ListBackupManifests scans location l for backup files and returns the list of manifests.
//
// When fullManifest is false (the default for listing), it tries the lightweight
// manifest_summary.json first; if absent or unreadable it falls back to manifest.json.
// The summary path omits Groups and DropOperations, making it fast even for 500 MB+ manifests.
//
// When fullManifest is true, the full manifest.json is always read — useful when the caller
// needs predicate or DROP-operation data (e.g. --verbose listing, restore planning).
func ListBackupManifests(l string, creds *x.MinioCredentials, fullManifest bool) ([]*Manifest, error) {
	uri, err := url.Parse(l)
	if err != nil {
		return nil, err
	}

	h, err := NewUriHandler(uri, creds)
	if err != nil {
		return nil, errors.Wrap(err, "error in listBackupManifests")
	}

	// Fast path: summary manifest (unless caller explicitly wants the full manifest).
	if !fullManifest && h.FileExists(backupManifestSummary) {
		ms, err := readMasterManifestSummary(h)
		if err == nil {
			glog.V(2).Infof("ListBackupManifests: using summary manifest (%d entries)",
				len(ms.Manifests))
			return summariesToManifests(ms.Manifests), nil
		}
		glog.Warningf("Summary manifest unreadable, falling back to full manifest: %v", err)
	}

	// Full manifest.json path.
	m, err := GetManifest(h, uri)
	if err != nil {
		return nil, err
	}
	return m.Manifests, nil
}
