//go:build integration2

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/dgraph-io/dgraph/v25/dgraphapi"
	"github.com/dgraph-io/dgraph/v25/dgraphtest"
)

// manifestEntry is a minimal representation of one entry in manifest.json.
type manifestEntry struct {
	Type      string `json:"type"`
	BackupId  string `json:"backup_id"`
	BackupNum uint64 `json:"backup_num"`
	ReadTs    uint64 `json:"read_ts"`
}

// masterManifest mirrors the top-level shape of manifest.json (capital "Manifests").
type masterManifest struct {
	Manifests []manifestEntry `json:"Manifests"`
}

// masterManifestSummary mirrors the top-level shape of manifest_summary.json
// (capital "Manifests", matching Go's default JSON encoding for MasterManifestSummary).
type masterManifestSummary struct {
	Manifests []manifestEntry `json:"Manifests"`
}

// queryNames runs {q(func: has(name)) {name}} and returns a sorted list of
// name values.
func queryNames(t *testing.T, gc *dgraphapi.GrpcClient) []string {
	t.Helper()
	resp, err := gc.Query(`{q(func: has(name)) {name}}`)
	require.NoError(t, err)

	var result struct {
		Q []struct {
			Name string `json:"name"`
		} `json:"q"`
	}
	require.NoError(t, json.Unmarshal(resp.Json, &result))

	names := make([]string, 0, len(result.Q))
	for _, r := range result.Q {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return names
}

// insertName adds a single <name> triple and commits.
func insertName(t *testing.T, gc *dgraphapi.GrpcClient, name string) {
	t.Helper()
	_, err := gc.Mutate(&api.Mutation{
		SetNquads: []byte(fmt.Sprintf(`_:x <name> %q .`, name)),
		CommitNow: true,
	})
	require.NoError(t, err)
}

// restoreAndWait triggers an online restore and blocks until it finishes.
func restoreAndWait(t *testing.T, hc *dgraphapi.HTTPClient, c *dgraphtest.LocalCluster,
	backupId string, backupNum int) {
	t.Helper()
	require.NoError(t, hc.Restore(c, dgraphtest.DefaultBackupDir, backupId, 0, backupNum))
	require.NoError(t, dgraphapi.WaitForRestore(c))
}

// TestManifestSummaryBackwardCompat verifies two things in one scenario:
//
//  1. manifest_summary.json accumulates entries from backups taken with an
//     older Dgraph binary (no summary feature) alongside new-binary backups.
//
//  2. All four backups (2 old + 2 new) are fully restorable and produce the
//     correct data at each checkpoint, confirming that adding the summary
//     file does not corrupt or alter the backup/restore pipeline.
//
// Timeline:
//
//	[old binary v24.0.0]
//	  schema setup → insert "alice"
//	  backup-1 (full)   → only manifest.json written
//	  insert "bob"
//	  backup-2 (incr)   → only manifest.json written
//	[in-place upgrade to local build]
//	  insert "carol"
//	  backup-3 (full)   → manifest.json + manifest_summary.json (3 entries)
//	  insert "dave"
//	  backup-4 (incr)   → manifest.json + manifest_summary.json (4 entries)
//
// Restore checkpoints verified:
//
//	old series  backup-2 → {alice, bob}
//	new series  backup-3 → {alice, bob, carol}
//	new series  backup-4 → {alice, bob, carol, dave}
func TestManifestSummaryBackwardCompat(t *testing.T) {
	conf := dgraphtest.NewClusterConfig().
		WithNumAlphas(1).WithNumZeros(1).WithReplicas(1).
		WithVersion("v24.0.0")
	c, err := dgraphtest.NewLocalCluster(conf)
	require.NoError(t, err)
	defer func() { c.Cleanup(t.Failed()) }()
	require.NoError(t, c.Start())

	gc, cleanup, err := c.Client()
	require.NoError(t, err)
	defer cleanup()

	hc, err := c.HTTPClient()
	require.NoError(t, err)

	// ── Old-binary phase ─────────────────────────────────────────────────────

	require.NoError(t, gc.SetupSchema(`name: string @index(exact) .`))
	insertName(t, gc, "alice")

	// backup-1: full backup with old binary; creates manifest.json only.
	require.NoError(t, hc.Backup(c, true, dgraphtest.DefaultBackupDir))

	insertName(t, gc, "bob")

	// backup-2: incremental backup with old binary.
	require.NoError(t, hc.Backup(c, false, dgraphtest.DefaultBackupDir))

	// Sanity: no manifest_summary.json should exist yet.
	_, err = c.ReadFileFromContainer(dgraphtest.DefaultBackupDir + "/manifest_summary.json")
	require.Error(t, err, "manifest_summary.json must not exist after old-binary backups")

	// ── In-place upgrade ─────────────────────────────────────────────────────

	// InPlace stops containers, swaps the binary, and restarts. The backup
	// volume and alpha posting data are both preserved.
	require.NoError(t, c.Upgrade("local", dgraphtest.InPlace))

	// Re-create clients — container ports may have been remapped.
	cleanup()
	gc, cleanup, err = c.Client()
	require.NoError(t, err)
	defer cleanup()

	hc, err = c.HTTPClient()
	require.NoError(t, err)

	// ── New-binary phase ─────────────────────────────────────────────────────

	insertName(t, gc, "carol")

	// backup-3: full backup; CompleteBackup reads 2 old entries + new = 3 total;
	// writes both manifest.json and manifest_summary.json.
	require.NoError(t, hc.Backup(c, true, dgraphtest.DefaultBackupDir))

	insertName(t, gc, "dave")

	// backup-4: incremental backup; summary grows to 4 entries.
	require.NoError(t, hc.Backup(c, false, dgraphtest.DefaultBackupDir))

	// ── Verify manifest_summary.json ─────────────────────────────────────────

	summaryRaw, err := c.ReadFileFromContainer(
		dgraphtest.DefaultBackupDir + "/manifest_summary.json")
	require.NoError(t, err, "manifest_summary.json must exist after new-binary backups")

	var summary masterManifestSummary
	require.NoError(t, json.Unmarshal(summaryRaw, &summary))
	require.Equal(t, 4, len(summary.Manifests),
		"summary must contain all 4 entries: 2 old-binary + 2 new-binary")

	types := make(map[string]int)
	for _, m := range summary.Manifests {
		types[m.Type]++
	}
	require.Equal(t, 2, types["full"], "expected 2 full-backup entries in summary")
	require.Equal(t, 2, types["incremental"], "expected 2 incremental-backup entries in summary")

	// ── Identify backup series IDs ────────────────────────────────────────────
	// Read manifest.json (has backup_id per entry) to identify the two series.

	manifestRaw, err := c.ReadFileFromContainer(
		dgraphtest.DefaultBackupDir + "/manifest.json")
	require.NoError(t, err)

	var master masterManifest
	require.NoError(t, json.Unmarshal(manifestRaw, &master))
	require.Equal(t, 4, len(master.Manifests))

	// Collect unique backup_ids in order of appearance (oldest first).
	seen := make(map[string]bool)
	var seriesIds []string
	for _, m := range master.Manifests {
		if !seen[m.BackupId] {
			seen[m.BackupId] = true
			seriesIds = append(seriesIds, m.BackupId)
		}
	}
	require.Equal(t, 2, len(seriesIds),
		"expected exactly 2 distinct backup series")

	oldSeriesId := seriesIds[0] // taken with v24.0.0
	newSeriesId := seriesIds[1] // taken with local build

	t.Run("restore old-series backup-2 yields alice and bob", func(t *testing.T) {
		restoreAndWait(t, hc, c, oldSeriesId, 2)

		names := queryNames(t, gc)
		require.ElementsMatch(t, []string{"alice", "bob"}, names)
	})

	t.Run("restore new-series backup-3 (full) yields alice bob carol", func(t *testing.T) {
		restoreAndWait(t, hc, c, newSeriesId, 1)

		names := queryNames(t, gc)
		require.ElementsMatch(t, []string{"alice", "bob", "carol"}, names)
	})

	t.Run("restore new-series backup-4 (full+incr) yields all four names", func(t *testing.T) {
		restoreAndWait(t, hc, c, newSeriesId, 2)

		names := queryNames(t, gc)
		require.ElementsMatch(t, []string{"alice", "bob", "carol", "dave"}, names)
	})
}
