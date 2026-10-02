/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAnonymousFullIsTheZeroValue pins the property the v25 rollout rests on: a
// WorkerOptions that was never populated, and a --security string that never
// mentions the key, both behave exactly as they did before the flag existed.
func TestAnonymousFullIsTheZeroValue(t *testing.T) {
	var p AnonymousPosture
	require.Equal(t, AnonymousFull, p)
	require.False(t, p.RequiresIdentityForCapability())
	require.False(t, p.RequiresIdentityForData())

	parsed, err := ParseAnonymousPosture("")
	require.NoError(t, err)
	require.Equal(t, AnonymousFull, parsed)
}

func TestParseAnonymousPosture(t *testing.T) {
	tests := []struct {
		in      string
		want    AnonymousPosture
		wantErr bool
	}{
		{in: "", want: AnonymousFull},
		{in: "full", want: AnonymousFull},
		{in: "FULL", want: AnonymousFull},
		{in: "  full  ", want: AnonymousFull},
		{in: "data", want: AnonymousData},
		{in: "Data", want: AnonymousData},
		{in: "none", want: AnonymousNone},
		{in: "NONE", want: AnonymousNone},
		// A typo must fail at startup rather than silently selecting a posture the
		// operator did not ask for. "off" and "all" are the plausible ones.
		{in: "off", wantErr: true},
		{in: "all", wantErr: true},
		{in: "true", wantErr: true},
		{in: "admin", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseAnonymousPosture(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				// A rejected value still reports the safe posture, so a caller that
				// ignores the error does not accidentally get a closed cluster.
				require.Equal(t, AnonymousFull, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAnonymousPostureEnforcement(t *testing.T) {
	tests := []struct {
		posture    AnonymousPosture
		str        string
		capability bool
		data       bool
	}{
		{posture: AnonymousFull, str: "full", capability: false, data: false},
		{posture: AnonymousData, str: "data", capability: true, data: false},
		{posture: AnonymousNone, str: "none", capability: true, data: true},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			require.Equal(t, tt.str, tt.posture.String())
			require.Equal(t, tt.capability, tt.posture.RequiresIdentityForCapability())
			require.Equal(t, tt.data, tt.posture.RequiresIdentityForData())

			// Every value this type can hold must survive a round trip through the
			// superflag, since that is the only way one is ever constructed.
			parsed, err := ParseAnonymousPosture(tt.posture.String())
			require.NoError(t, err)
			require.Equal(t, tt.posture, parsed)
		})
	}
}
