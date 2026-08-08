/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package worker

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckBackupReadTsAdvanced(t *testing.T) {
	tests := []struct {
		name           string
		latestManifest *Manifest
		readTs         uint64
		wantErr        bool
	}{
		{
			// First backup ever: no prior manifest exists, any ReadTs is fine.
			name:           "no prior manifest",
			latestManifest: &Manifest{},
			readTs:         100,
			wantErr:        false,
		},
		{
			name: "normal incremental advances chain",
			latestManifest: &Manifest{
				ManifestBase: ManifestBase{Type: "full", ReadTs: 100},
			},
			readTs:  200,
			wantErr: false,
		},
		{
			// Repro of the customer's chain corruption: chain reached read_ts
			// 32M, then Zero state was lost; new ReadTs is far below.
			name: "regressed ReadTs (Zero state reset)",
			latestManifest: &Manifest{
				ManifestBase: ManifestBase{Type: "incremental", ReadTs: 32000000},
			},
			readTs:  147784,
			wantErr: true,
		},
		{
			// Equal ReadTs means the incremental would capture nothing AND
			// indicates ordering ambiguity — treat as regression.
			name: "equal ReadTs",
			latestManifest: &Manifest{
				ManifestBase: ManifestBase{Type: "full", ReadTs: 1000},
			},
			readTs:  1000,
			wantErr: true,
		},
		{
			// Pre-21.03 manifests store the timestamp in SinceTsDeprecated
			// rather than ReadTs; ValidReadTs() falls back to that. Make
			// sure regression detection still triggers.
			name: "regression detected via deprecated since field",
			latestManifest: &Manifest{
				ManifestBase: ManifestBase{Type: "incremental", SinceTsDeprecated: 500},
			},
			readTs:  400,
			wantErr: true,
		},
		{
			// Same as above but ReadTs DOES advance — must not trigger.
			name: "advance detected via deprecated since field",
			latestManifest: &Manifest{
				ManifestBase: ManifestBase{Type: "incremental", SinceTsDeprecated: 500},
			},
			readTs:  600,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkBackupReadTsAdvanced(tc.latestManifest, tc.readTs)
			if tc.wantErr {
				require.Error(t, err)
				// Operator needs to know how to recover.
				require.Contains(t, err.Error(), "forceFull",
					"error message should direct operator to forceFull")
				return
			}
			require.NoError(t, err)
		})
	}
}
