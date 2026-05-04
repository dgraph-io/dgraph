/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package backup

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseFilterDate(t *testing.T) {
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
			name:      "YYYY-MM-DD",
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
			name:    "empty string returns error",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid string",
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFilterDate(tc.input)
			if tc.wantErr {
				require.Error(t, err)
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

func TestBuildLsFilter(t *testing.T) {
	t.Run("negative lastNDays returns error", func(t *testing.T) {
		_, err := buildLsFilter("", "", -1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--last-n-days")
		require.Contains(t, err.Error(), "-1")
	})

	t.Run("lastNDays and sinceDate together return error", func(t *testing.T) {
		_, err := buildLsFilter("2026-01-01", "", 7)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--last-n-days")
		require.Contains(t, err.Error(), "--since-date")
	})

	t.Run("invalid sinceDate returns error naming the flag", func(t *testing.T) {
		_, err := buildLsFilter("not-a-date", "", 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--since-date")
	})

	t.Run("invalid untilDate returns error naming the flag", func(t *testing.T) {
		_, err := buildLsFilter("", "not-a-date", 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "--until-date")
	})

	t.Run("all empty returns filter with nil Since and Until", func(t *testing.T) {
		f, err := buildLsFilter("", "", 0)
		require.NoError(t, err)
		require.Nil(t, f.Since)
		require.Nil(t, f.Until)
	})

	t.Run("lastNDays=0 does not set Since", func(t *testing.T) {
		f, err := buildLsFilter("", "", 0)
		require.NoError(t, err)
		require.Nil(t, f.Since)
	})

	t.Run("lastNDays=7 sets Since to 7 days ago midnight UTC", func(t *testing.T) {
		expected := time.Now().UTC().AddDate(0, 0, -7).Truncate(24 * time.Hour)
		f, err := buildLsFilter("", "", 7)
		require.NoError(t, err)
		require.NotNil(t, f.Since)
		require.Nil(t, f.Until)
		require.WithinDuration(t, expected, *f.Since, time.Second)
		require.Equal(t, 0, f.Since.Hour(), "Since must be at midnight")
	})

	t.Run("sinceDate YYYY-MM-DD sets Since to midnight UTC of that date", func(t *testing.T) {
		f, err := buildLsFilter("2026-03-01", "", 0)
		require.NoError(t, err)
		require.NotNil(t, f.Since)
		require.Nil(t, f.Until)
		require.Equal(t, 2026, f.Since.Year())
		require.Equal(t, time.March, f.Since.Month())
		require.Equal(t, 1, f.Since.Day())
		require.Equal(t, 0, f.Since.Hour())
	})

	t.Run("sinceDate RFC3339 uses exact time without midnight forcing", func(t *testing.T) {
		f, err := buildLsFilter("2026-03-01T08:30:00Z", "", 0)
		require.NoError(t, err)
		require.NotNil(t, f.Since)
		require.Equal(t, 8, f.Since.Hour())
		require.Equal(t, 30, f.Since.Minute())
		require.Equal(t, 0, f.Since.Second())
		require.Equal(t, time.UTC, f.Since.Location())
	})

	t.Run("untilDate YYYY-MM-DD extends to end of that calendar day", func(t *testing.T) {
		f, err := buildLsFilter("", "2026-03-31", 0)
		require.NoError(t, err)
		require.Nil(t, f.Since)
		require.NotNil(t, f.Until)
		// Should be 2026-03-31 23:59:59.999 UTC
		require.Equal(t, 2026, f.Until.Year())
		require.Equal(t, time.March, f.Until.Month())
		require.Equal(t, 31, f.Until.Day())
		require.Equal(t, 23, f.Until.Hour())
		require.Equal(t, 59, f.Until.Minute())
		require.Equal(t, 59, f.Until.Second())
	})

	t.Run("untilDate RFC3339 uses exact time without end-of-day extension", func(t *testing.T) {
		f, err := buildLsFilter("", "2026-03-31T12:00:00Z", 0)
		require.NoError(t, err)
		require.NotNil(t, f.Until)
		// Must be exactly 12:00:00, not extended to 23:59:59
		require.Equal(t, 12, f.Until.Hour())
		require.Equal(t, 0, f.Until.Minute())
		require.Equal(t, 0, f.Until.Second())
	})

	t.Run("sinceDate and untilDate both set", func(t *testing.T) {
		f, err := buildLsFilter("2026-01-01", "2026-12-31", 0)
		require.NoError(t, err)
		require.NotNil(t, f.Since)
		require.NotNil(t, f.Until)
		require.Equal(t, time.January, f.Since.Month())
		require.Equal(t, 1, f.Since.Day())
		require.Equal(t, time.December, f.Until.Month())
		require.Equal(t, 31, f.Until.Day())
		require.Equal(t, 23, f.Until.Hour())
	})

}
