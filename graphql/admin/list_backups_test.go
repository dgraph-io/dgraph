/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/graphql/schema"
	"github.com/dgraph-io/dgraph/v25/worker"
)

// stubField implements schema.Field with only Name() defined.
// All other methods panic — they must not be called by needsFullManifest.
type stubField struct {
	name string
	schema.Field
}

func (s stubField) Name() string { return s.name }

func fields(names ...string) []schema.Field {
	out := make([]schema.Field, len(names))
	for i, n := range names {
		out[i] = stubField{name: n}
	}
	return out
}

func TestParseGraphQLDate(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantYear  int
		wantMonth time.Month
		wantDay   int
		wantHour  int
		wantMin   int
	}{
		{
			name:      "YYYY-MM-DD format",
			input:     "2026-04-15",
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   15,
		},
		{
			name:      "RFC3339 UTC",
			input:     "2026-01-01T00:00:00Z",
			wantYear:  2026,
			wantMonth: time.January,
			wantDay:   1,
		},
		{
			name:      "RFC3339 with positive offset normalised to UTC",
			input:     "2026-04-15T06:30:00+05:30", // 06:30 IST == 01:00 UTC
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   15,
			wantHour:  1,
			wantMin:   0,
		},
		{
			name:      "RFC3339 with negative offset normalised to UTC",
			input:     "2026-04-15T00:00:00-05:00", // midnight EST == 05:00 UTC
			wantYear:  2026,
			wantMonth: time.April,
			wantDay:   15,
			wantHour:  5,
		},
		{
			name:    "invalid date string",
			input:   "not-a-date",
			wantErr: true,
		},
		{
			name:    "partial date without day",
			input:   "2026-04",
			wantErr: true,
		},
		{
			name:    "RFC3339 without timezone is invalid",
			input:   "2026-04-15T10:00:00",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGraphQLDate(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				if tc.input == "" {
					require.Contains(t, err.Error(), "empty")
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, time.UTC, got.Location(), "result must be UTC")
			require.Equal(t, tc.wantYear, got.Year())
			require.Equal(t, tc.wantMonth, got.Month())
			require.Equal(t, tc.wantDay, got.Day())
			require.Equal(t, tc.wantHour, got.Hour())
			require.Equal(t, tc.wantMin, got.Minute())
		})
	}
}

func TestBuildBackupDateFilter(t *testing.T) {
	t.Run("no fields set returns empty filter", func(t *testing.T) {
		filter, err := buildBackupDateFilter(&lsBackupInput{})
		require.NoError(t, err)
		require.Nil(t, filter.Since)
		require.Nil(t, filter.Until)
	})

	t.Run("sinceDate YYYY-MM-DD sets Since to midnight UTC", func(t *testing.T) {
		filter, err := buildBackupDateFilter(&lsBackupInput{SinceDate: "2026-03-01"})
		require.NoError(t, err)
		require.NotNil(t, filter.Since)
		require.Equal(t, 2026, filter.Since.Year())
		require.Equal(t, time.March, filter.Since.Month())
		require.Equal(t, 1, filter.Since.Day())
		require.Equal(t, 0, filter.Since.Hour(), "sinceDate must be at midnight UTC")
		require.Nil(t, filter.Until)
	})

	t.Run("sinceDate RFC3339 uses exact time", func(t *testing.T) {
		filter, err := buildBackupDateFilter(&lsBackupInput{SinceDate: "2026-03-01T08:00:00Z"})
		require.NoError(t, err)
		require.NotNil(t, filter.Since)
		require.Equal(t, 8, filter.Since.Hour())
		require.Equal(t, 0, filter.Since.Minute())
	})

	t.Run("untilDate YYYY-MM-DD sets Until to end of that day", func(t *testing.T) {
		filter, err := buildBackupDateFilter(&lsBackupInput{UntilDate: "2026-03-31"})
		require.NoError(t, err)
		require.Nil(t, filter.Since)
		require.NotNil(t, filter.Until)
		// Until = 2026-03-31 00:00:00 + 24h - 1ms = 2026-03-31 23:59:59.999
		require.Equal(t, 2026, filter.Until.Year())
		require.Equal(t, time.March, filter.Until.Month())
		require.Equal(t, 31, filter.Until.Day())
		require.Equal(t, 23, filter.Until.Hour())
		require.Equal(t, 59, filter.Until.Minute())
		require.Equal(t, 59, filter.Until.Second())
	})

	t.Run("untilDate RFC3339 uses exact time without end-of-day extension", func(t *testing.T) {
		filter, err := buildBackupDateFilter(&lsBackupInput{UntilDate: "2026-03-31T12:00:00Z"})
		require.NoError(t, err)
		require.NotNil(t, filter.Until)
		require.Equal(t, 12, filter.Until.Hour())
		require.Equal(t, 0, filter.Until.Minute())
		require.Equal(t, 0, filter.Until.Second())
	})

	t.Run("lastNDays=7 sets Since to 7 days ago midnight UTC", func(t *testing.T) {
		expected := time.Now().UTC().AddDate(0, 0, -7).Truncate(24 * time.Hour)
		filter, err := buildBackupDateFilter(&lsBackupInput{LastNDays: 7})
		require.NoError(t, err)
		require.NotNil(t, filter.Since)
		require.Nil(t, filter.Until)
		require.WithinDuration(t, expected, *filter.Since, time.Second)
		require.Equal(t, 0, filter.Since.Hour(), "Since must be at midnight UTC")
	})

	t.Run("lastNDays=0 does not set Since", func(t *testing.T) {
		filter, err := buildBackupDateFilter(&lsBackupInput{LastNDays: 0})
		require.NoError(t, err)
		require.Nil(t, filter.Since)
	})

	t.Run("lastNDays and sinceDate together return error", func(t *testing.T) {
		_, err := buildBackupDateFilter(&lsBackupInput{
			LastNDays: 30,
			SinceDate: "2026-01-01",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "sinceDate")
		require.Contains(t, err.Error(), "lastNDays")
	})

	t.Run("negative lastNDays returns error", func(t *testing.T) {
		_, err := buildBackupDateFilter(&lsBackupInput{LastNDays: -1})
		require.Error(t, err)
		require.Contains(t, err.Error(), "lastNDays")
	})

	t.Run("sinceDate and untilDate both set", func(t *testing.T) {
		filter, err := buildBackupDateFilter(&lsBackupInput{
			SinceDate: "2026-01-01",
			UntilDate: "2026-01-31",
		})
		require.NoError(t, err)
		require.NotNil(t, filter.Since)
		require.NotNil(t, filter.Until)
		require.Equal(t, time.January, filter.Since.Month())
		require.Equal(t, 1, filter.Since.Day())
		// Until is 2026-01-31 + 24h - 1ms = 2026-01-31 23:59:59.999
		require.Equal(t, time.January, filter.Until.Month())
		require.Equal(t, 31, filter.Until.Day())
		require.Equal(t, 23, filter.Until.Hour())
		require.Equal(t, 59, filter.Until.Second())
	})

	t.Run("invalid sinceDate returns error with field name", func(t *testing.T) {
		_, err := buildBackupDateFilter(&lsBackupInput{SinceDate: "not-a-date"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "sinceDate")
	})

	t.Run("invalid untilDate returns error with field name", func(t *testing.T) {
		_, err := buildBackupDateFilter(&lsBackupInput{UntilDate: "not-a-date"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "untilDate")
	})
}

func TestConvertManifests(t *testing.T) {
	t.Run("nil input returns empty slice", func(t *testing.T) {
		result := convertManifests(nil)
		require.Empty(t, result)
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		result := convertManifests([]*worker.Manifest{})
		require.NotNil(t, result)
		require.Equal(t, 0, len(result))
	})

	t.Run("single full manifest without groups", func(t *testing.T) {
		m := &worker.Manifest{
			ManifestBase: worker.ManifestBase{
				Type:              "full",
				BackupId:          "backup-abc",
				BackupNum:         1,
				Path:              "dgraph.20260101.000000.000",
				ReadTs:            500,
				SinceTsDeprecated: 0,
				Encrypted:         true,
			},
		}
		result := convertManifests([]*worker.Manifest{m})
		require.Equal(t, 1, len(result))
		r := result[0]
		require.Equal(t, "full", r.Type)
		require.Equal(t, "backup-abc", r.BackupId)
		require.Equal(t, uint64(1), r.BackupNum)
		require.Equal(t, "dgraph.20260101.000000.000", r.Path)
		require.Equal(t, uint64(500), r.ReadTs)
		require.Equal(t, uint64(0), r.Since)
		require.True(t, r.Encrypted)
		require.Empty(t, r.Groups)
	})

	t.Run("Since maps from SinceTsDeprecated", func(t *testing.T) {
		m := &worker.Manifest{
			ManifestBase: worker.ManifestBase{
				Type:              "incremental",
				BackupId:          "id1",
				BackupNum:         2,
				ReadTs:            800,
				SinceTsDeprecated: 500,
			},
		}
		result := convertManifests([]*worker.Manifest{m})
		require.Equal(t, 1, len(result))
		require.Equal(t, uint64(500), result[0].Since)
		require.Equal(t, uint64(800), result[0].ReadTs)
	})

	t.Run("manifest with multiple groups", func(t *testing.T) {
		m := &worker.Manifest{
			ManifestBase: worker.ManifestBase{
				Type:      "incremental",
				BackupId:  "backup-def",
				BackupNum: 2,
				Path:      "dgraph.20260115.000000.000",
				ReadTs:    800,
			},
			Groups: map[uint32][]string{
				1: {"0-name", "0-age"},
				2: {"0-email"},
			},
		}
		result := convertManifests([]*worker.Manifest{m})
		require.Equal(t, 1, len(result))
		r := result[0]
		require.Equal(t, 2, len(r.Groups))

		groupMap := make(map[uint32][]string)
		for _, g := range r.Groups {
			groupMap[g.GroupId] = g.Predicates
		}
		require.ElementsMatch(t, []string{"0-name", "0-age"}, groupMap[1])
		require.ElementsMatch(t, []string{"0-email"}, groupMap[2])
	})

	t.Run("manifest with nil groups yields empty Groups slice", func(t *testing.T) {
		m := &worker.Manifest{
			ManifestBase: worker.ManifestBase{
				Type: "full", BackupId: "id2", BackupNum: 1,
			},
			Groups: nil,
		}
		result := convertManifests([]*worker.Manifest{m})
		require.Equal(t, 1, len(result))
		require.Empty(t, result[0].Groups)
	})

	t.Run("multiple manifests preserves order and fields", func(t *testing.T) {

		manifests := []*worker.Manifest{
			{
				ManifestBase: worker.ManifestBase{
					Type: "full", BackupId: "id1", BackupNum: 1, ReadTs: 100,
					Path: "dgraph.20260101.000000.000",
				},
			},
			{
				ManifestBase: worker.ManifestBase{
					Type: "incremental", BackupId: "id1", BackupNum: 2, ReadTs: 200,
					SinceTsDeprecated: 100,
					Path:              "dgraph.20260115.000000.000",
				},
			},
		}
		result := convertManifests(manifests)
		require.Equal(t, 2, len(result))
		require.Equal(t, "full", result[0].Type)
		require.Equal(t, uint64(100), result[0].ReadTs)
		require.Equal(t, "incremental", result[1].Type)
		require.Equal(t, uint64(200), result[1].ReadTs)
		require.Equal(t, uint64(100), result[1].Since)
	})
}

func TestNeedsFullManifest(t *testing.T) {
	t.Run("false when flag is false and groups not in selection", func(t *testing.T) {
		require.False(t, needsFullManifest(false, fields("backupId", "type", "path")))
	})

	t.Run("true when fullManifest flag is true regardless of selection", func(t *testing.T) {
		require.True(t, needsFullManifest(true, fields("backupId", "type")))
	})

	t.Run("true when groups is in selection even without fullManifest flag", func(t *testing.T) {
		require.True(t, needsFullManifest(false, fields("backupId", "type", "groups")))
	})

	t.Run("true when groups is the only selected field", func(t *testing.T) {
		require.True(t, needsFullManifest(false, fields("groups")))
	})

	t.Run("false when selection is empty and flag is false", func(t *testing.T) {
		require.False(t, needsFullManifest(false, fields()))
	})

	t.Run("false when selection has similar but not exact name", func(t *testing.T) {
		require.False(t, needsFullManifest(false, fields("groupId", "backupGroups")))
	})
}
