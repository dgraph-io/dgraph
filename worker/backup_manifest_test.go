/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
)

func TestParseBackupTime(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantErr   bool
		wantYear  int
		wantMonth time.Month
		wantDay   int
	}{
		{
			name:      "full timestamp",
			path:      "dgraph.20260101.120000.000",
			wantYear:  2026,
			wantMonth: time.January,
			wantDay:   1,
		},
		{
			name:      "mid-year timestamp",
			path:      "dgraph.20230415.093045.123",
			wantYear:  2023,
			wantMonth: time.April,
			wantDay:   15,
		},
		{
			name:    "missing dgraph prefix",
			path:    "backup.20260101.120000.000",
			wantErr: true,
		},
		{
			name:    "empty timestamp after prefix",
			path:    "dgraph.",
			wantErr: true,
		},
		{
			name:    "no prefix at all",
			path:    "somebackup",
			wantErr: true,
		},
		{
			name:    "malformed timestamp",
			path:    "dgraph.notadate",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBackupTime(tc.path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantYear, got.Year())
			require.Equal(t, tc.wantMonth, got.Month())
			require.Equal(t, tc.wantDay, got.Day())
		})
	}
}

func TestFilterManifestsByDate(t *testing.T) {
	tp := func(year, month, day int) *time.Time {
		ts := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		return &ts
	}

	manifests := []*Manifest{
		{ManifestBase: ManifestBase{Path: "dgraph.20260101.000000.000", BackupId: "a", Type: "full"}},
		{ManifestBase: ManifestBase{Path: "dgraph.20260115.000000.000", BackupId: "a", Type: "incremental"}},
		{ManifestBase: ManifestBase{Path: "dgraph.20260201.000000.000", BackupId: "b", Type: "full"}},
		{ManifestBase: ManifestBase{Path: "dgraph.20260215.000000.000", BackupId: "b", Type: "incremental"}},
		// unparseable path — always included (fail-open)
		{ManifestBase: ManifestBase{Path: "unknown_format", BackupId: "c", Type: "full"}},
	}

	t.Run("no filter returns all", func(t *testing.T) {
		got := FilterManifestsByDate(manifests, BackupDateFilter{})
		require.Equal(t, len(manifests), len(got))
	})

	t.Run("since filter", func(t *testing.T) {
		filter := BackupDateFilter{Since: tp(2026, 2, 1)}
		got := FilterManifestsByDate(manifests, filter)
		// Feb 1, Feb 15, unparseable
		require.Equal(t, 3, len(got))
		require.Equal(t, "dgraph.20260201.000000.000", got[0].Path)
		require.Equal(t, "dgraph.20260215.000000.000", got[1].Path)
	})

	t.Run("until filter", func(t *testing.T) {
		filter := BackupDateFilter{Until: tp(2026, 1, 15)}
		got := FilterManifestsByDate(manifests, filter)
		// Jan 1, Jan 15, unparseable
		require.Equal(t, 3, len(got))
		require.Equal(t, "dgraph.20260101.000000.000", got[0].Path)
		require.Equal(t, "dgraph.20260115.000000.000", got[1].Path)
	})

	t.Run("since and until range", func(t *testing.T) {
		filter := BackupDateFilter{Since: tp(2026, 1, 10), Until: tp(2026, 1, 31)}
		got := FilterManifestsByDate(manifests, filter)
		// Jan 15 + unparseable
		require.Equal(t, 2, len(got))
		require.Equal(t, "dgraph.20260115.000000.000", got[0].Path)
		require.Equal(t, "unknown_format", got[1].Path)
	})

	t.Run("no parseable results only unparseable remains", func(t *testing.T) {
		filter := BackupDateFilter{Since: tp(2030, 1, 1)}
		got := FilterManifestsByDate(manifests, filter)
		require.Equal(t, 1, len(got))
		require.Equal(t, "unknown_format", got[0].Path)
	})

	t.Run("nil manifests", func(t *testing.T) {
		got := FilterManifestsByDate(nil, BackupDateFilter{Since: tp(2026, 1, 1)})
		require.Nil(t, got)
	})
}

func TestComputeBackupListStats(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		stats := ComputeBackupListStats(nil)
		require.Equal(t, 0, stats.Total)
		require.Equal(t, 0, stats.BackupSeriesCount)
		require.Nil(t, stats.OldestBackup)
		require.Nil(t, stats.NewestBackup)
	})

	t.Run("full stats", func(t *testing.T) {
		manifests := []*Manifest{
			{ManifestBase: ManifestBase{Path: "dgraph.20260101.000000.000", BackupId: "a", Type: "full"}},
			{ManifestBase: ManifestBase{Path: "dgraph.20260115.000000.000", BackupId: "a", Type: "incremental"}},
			{ManifestBase: ManifestBase{Path: "dgraph.20260201.000000.000", BackupId: "b", Type: "full"}},
			{ManifestBase: ManifestBase{Path: "dgraph.20260215.000000.000", BackupId: "b", Type: "incremental"}},
		}
		stats := ComputeBackupListStats(manifests)

		require.Equal(t, 4, stats.Total)
		require.Equal(t, 2, stats.BackupSeriesCount)

		require.NotNil(t, stats.OldestBackup)
		require.Equal(t, 2026, stats.OldestBackup.Year())
		require.Equal(t, time.January, stats.OldestBackup.Month())
		require.Equal(t, 1, stats.OldestBackup.Day())

		require.NotNil(t, stats.NewestBackup)
		require.Equal(t, time.February, stats.NewestBackup.Month())
		require.Equal(t, 15, stats.NewestBackup.Day())

		require.NotNil(t, stats.LastFullBackup)
		require.Equal(t, time.February, stats.LastFullBackup.Month())
		require.Equal(t, 1, stats.LastFullBackup.Day())

		require.NotNil(t, stats.LastIncrBackup)
		require.Equal(t, time.February, stats.LastIncrBackup.Month())
		require.Equal(t, 15, stats.LastIncrBackup.Day())
	})

	t.Run("only full backups", func(t *testing.T) {
		manifests := []*Manifest{
			{ManifestBase: ManifestBase{Path: "dgraph.20260101.000000.000", BackupId: "a", Type: "full"}},
			{ManifestBase: ManifestBase{Path: "dgraph.20260201.000000.000", BackupId: "b", Type: "full"}},
		}
		stats := ComputeBackupListStats(manifests)
		require.Equal(t, 2, stats.Total)
		require.Equal(t, 2, stats.BackupSeriesCount)
		require.NotNil(t, stats.LastFullBackup)
		require.Nil(t, stats.LastIncrBackup)
	})

	t.Run("unparseable paths do not affect timestamps", func(t *testing.T) {
		manifests := []*Manifest{
			{ManifestBase: ManifestBase{Path: "legacy_backup", BackupId: "a", Type: "full"}},
		}
		stats := ComputeBackupListStats(manifests)
		require.Equal(t, 1, stats.Total)
		require.Equal(t, 1, stats.BackupSeriesCount)
		require.Nil(t, stats.OldestBackup)
		require.Nil(t, stats.LastFullBackup)
	})
}

func TestSummariesToManifests(t *testing.T) {
	summaries := []*ManifestSummary{
		{
			ManifestBase: ManifestBase{
				Type:              "full",
				BackupId:          "abc123",
				BackupNum:         1,
				Path:              "dgraph.20260101.000000.000",
				ReadTs:            1000,
				SinceTsDeprecated: 0,
				Encrypted:         true,
				Compression:       "snappy",
				Version:           2105,
			},
		},
	}

	got := summariesToManifests(summaries)
	require.Equal(t, 1, len(got))
	m := got[0]
	require.Equal(t, "full", m.Type)
	require.Equal(t, "abc123", m.BackupId)
	require.Equal(t, uint64(1), m.BackupNum)
	require.Equal(t, "dgraph.20260101.000000.000", m.Path)
	require.Equal(t, uint64(1000), m.ReadTs)
	require.Equal(t, uint64(0), m.SinceTsDeprecated)
	require.True(t, m.Encrypted)
	require.Equal(t, "snappy", m.Compression)
	// Groups and DropOperations must be absent (summary never stores them)
	require.Nil(t, m.Groups)
	require.Nil(t, m.DropOperations)
}

// testFileHandlerForDir returns a fileHandler and parsed URL for a temp directory.
func testFileHandlerForDir(t *testing.T, dir string) (*fileHandler, *url.URL) {
	t.Helper()
	uri, err := url.Parse(dir)
	require.NoError(t, err)
	return NewFileHandler(uri), uri
}

// writeMasterManifestToDir writes a MasterManifest to dir/manifest.json.
func writeMasterManifestToDir(t *testing.T, dir string, manifests []*Manifest) {
	t.Helper()
	master := &MasterManifest{Manifests: manifests}
	b, err := json.Marshal(master)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, backupManifest), b, 0644))
}

func TestCreateManifestSummary(t *testing.T) {
	tmpDir := t.TempDir()
	h, _ := testFileHandlerForDir(t, tmpDir)

	master := &MasterManifest{
		Manifests: []*Manifest{
			{
				ManifestBase: ManifestBase{
					Type: "full", BackupId: "id1", BackupNum: 1,
					Path: "dgraph.20260101.000000.000", ReadTs: 100,
				},
				Groups: map[uint32][]string{1: {"name", "age", "email"}},
			},
			{
				ManifestBase: ManifestBase{
					Type: "incremental", BackupId: "id1", BackupNum: 2,
					Path: "dgraph.20260115.000000.000", ReadTs: 200,
					SinceTsDeprecated: 100,
				},
				Groups: map[uint32][]string{1: {"name"}},
			},
		},
	}

	require.NoError(t, CreateManifestSummary(h, master))

	summaryPath := filepath.Join(tmpDir, backupManifestSummary)
	require.FileExists(t, summaryPath)
	// Temp file must be cleaned up after rename
	require.NoFileExists(t, filepath.Join(tmpDir, tmpManifestSummary))

	raw, err := os.ReadFile(summaryPath)
	require.NoError(t, err)

	var got MasterManifestSummary
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, 2, len(got.Manifests))

	s0 := got.Manifests[0]
	require.Equal(t, "full", s0.Type)
	require.Equal(t, "id1", s0.BackupId)
	require.Equal(t, uint64(1), s0.BackupNum)
	require.Equal(t, uint64(100), s0.ReadTs)

	// The raw JSON must never contain the groups or drop_operations keys.
	rawStr := string(raw)
	require.NotContains(t, rawStr, `"groups"`)
	require.NotContains(t, rawStr, `"drop_operations"`)
}

func TestListBackupManifests_UsesSummaryWhenPresent(t *testing.T) {
	tmpDir := t.TempDir()
	h, _ := testFileHandlerForDir(t, tmpDir)

	manifests := []*Manifest{
		{
			ManifestBase: ManifestBase{
				Type: "full", BackupId: "id1", BackupNum: 1,
				Path: "dgraph.20260101.000000.000", ReadTs: 100,
			},
			Groups: map[uint32][]string{1: {"name", "age"}},
		},
	}
	// Write both full manifest and summary
	writeMasterManifestToDir(t, tmpDir, manifests)
	require.NoError(t, CreateManifestSummary(h, &MasterManifest{Manifests: manifests}))

	// Default (fullManifest=false) should use the summary.
	got, err := ListBackupManifests(tmpDir, nil, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, "full", got[0].Type)
	require.Equal(t, "id1", got[0].BackupId)
	// Summary path omits Groups — verify listing path does not expose them
	require.Nil(t, got[0].Groups)
}

func TestListBackupManifests_FullManifestFlag(t *testing.T) {
	tmpDir := t.TempDir()
	h, _ := testFileHandlerForDir(t, tmpDir)

	manifests := []*Manifest{
		{
			ManifestBase: ManifestBase{
				Type: "full", BackupId: "id1", BackupNum: 1,
				Path: "dgraph.20260101.000000.000", ReadTs: 100, Version: 2105,
			},
			Groups: map[uint32][]string{1: {"0-name", "0-age"}},
		},
	}
	writeMasterManifestToDir(t, tmpDir, manifests)
	require.NoError(t, CreateManifestSummary(h, &MasterManifest{Manifests: manifests}))

	// fullManifest=true must bypass the summary and return Groups.
	got, err := ListBackupManifests(tmpDir, nil, true)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.NotNil(t, got[0].Groups, "full manifest flag must return Groups")
}

func TestListBackupManifests_FallsBackToFullManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Version 2105 so upgradeManifest is a no-op and predicates are returned as-is.
	manifests := []*Manifest{
		{
			ManifestBase: ManifestBase{
				Type: "full", BackupId: "id2", BackupNum: 1,
				Path: "dgraph.20260101.000000.000", ReadTs: 100, Version: 2105,
			},
			Groups: map[uint32][]string{1: {"0-pred1", "0-pred2"}},
		},
	}
	// Write only the full manifest — no summary present
	writeMasterManifestToDir(t, tmpDir, manifests)

	got, err := ListBackupManifests(tmpDir, nil, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, "id2", got[0].BackupId)
	// Full manifest includes Groups
	require.NotNil(t, got[0].Groups)
	require.Equal(t, []string{"0-pred1", "0-pred2"}, got[0].Groups[1])
}

func TestListBackupManifests_CorruptSummaryFallsBackToFullManifest(t *testing.T) {
	tmpDir := t.TempDir()

	manifests := []*Manifest{
		{
			ManifestBase: ManifestBase{
				Type: "full", BackupId: "id3", BackupNum: 1,
				Path: "dgraph.20260101.000000.000", ReadTs: 100, Version: 2105,
			},
			Groups: map[uint32][]string{1: {"0-pred1"}},
		},
	}
	writeMasterManifestToDir(t, tmpDir, manifests)

	// Write deliberately corrupt summary
	summaryPath := filepath.Join(tmpDir, backupManifestSummary)
	require.NoError(t, os.WriteFile(summaryPath, []byte("not valid json {{"), 0644))

	got, err := ListBackupManifests(tmpDir, nil, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(got))
	require.Equal(t, "id3", got[0].BackupId)
	require.NotNil(t, got[0].Groups)
}

func TestListBackupManifests_EmptyLocation(t *testing.T) {
	tmpDir := t.TempDir()
	// No manifest files — getConsolidatedManifest returns empty MasterManifest
	_, err := ListBackupManifests(tmpDir, nil, false)
	require.NoError(t, err)
}

// TestRestorePathContainmentEndToEnd drives the restore-side flow over a real
// backup layout: the master manifest is read back from the backup location and
// its "path" field feeds getManifestsToRestore and the handler reads. A
// manifest planted with a traversal path must not make the handler see or
// stream files outside its root, while a legitimate layout must restore
// unchanged.
func TestRestorePathContainmentEndToEnd(t *testing.T) {
	req := &pb.RestoreRequest{}

	t.Run("planted traversal manifest is contained", func(t *testing.T) {
		base := t.TempDir()
		backupRoot := filepath.Join(base, "backups")
		outside := filepath.Join(base, "outside")
		require.NoError(t, os.MkdirAll(backupRoot, 0755))
		require.NoError(t, os.MkdirAll(outside, 0755))

		// Plant the target where "../outside" used to resolve before containment.
		outsideFile := filepath.Join(outside, backupName(5, 1))
		require.NoError(t, os.WriteFile(outsideFile, []byte("outside-data"), 0644))

		evil := &Manifest{
			ManifestBase: ManifestBase{
				Type: "full", BackupId: "evil", BackupNum: 1,
				Path: "../outside", ReadTs: 5, Version: 2105,
			},
			Groups: map[uint32][]string{1: {"name"}},
		}
		writeMasterManifestToDir(t, backupRoot, []*Manifest{evil})

		h, uri := testFileHandlerForDir(t, backupRoot)
		manifests, err := getManifestsToRestore(h, uri, req)
		require.NoError(t, err)
		require.Empty(t, manifests)

		// The planted file exists on disk, but the handler must not reach it.
		require.FileExists(t, outsideFile)
		file := filepath.Join(evil.Path, backupName(5, 1))
		require.False(t, h.FileExists(file))
		_, err = h.Stream(file)
		require.Error(t, err)
	})

	t.Run("legitimate backup restores unchanged", func(t *testing.T) {
		base := t.TempDir()
		backupRoot := filepath.Join(base, "backups")
		backupDir := "dgraph.20260101.000000.000"
		require.NoError(t, os.MkdirAll(filepath.Join(backupRoot, backupDir), 0755))

		backupFile := filepath.Join(backupRoot, backupDir, backupName(5, 1))
		require.NoError(t, os.WriteFile(backupFile, []byte("backup-data"), 0644))

		good := &Manifest{
			ManifestBase: ManifestBase{
				Type: "full", BackupId: "ok", BackupNum: 1,
				Path: backupDir, ReadTs: 5, Version: 2105,
			},
			Groups: map[uint32][]string{1: {"name"}},
		}
		writeMasterManifestToDir(t, backupRoot, []*Manifest{good})

		h, uri := testFileHandlerForDir(t, backupRoot)
		manifests, err := getManifestsToRestore(h, uri, req)
		require.NoError(t, err)
		require.Len(t, manifests, 1)

		m := manifests[0]
		rc, err := h.Stream(filepath.Join(m.Path, backupName(m.ValidReadTs(), 1)))
		require.NoError(t, err)
		defer func() { require.NoError(t, rc.Close()) }()
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.Equal(t, "backup-data", string(data))
	})
}
