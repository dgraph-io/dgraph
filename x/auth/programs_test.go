/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasProgram(t *testing.T) {
	tests := []struct {
		name         string
		userPrograms []string
		predPrograms []string
		hasAccess    bool
	}{
		// Exact match
		{"single program exact match", []string{"alpha"}, []string{"alpha"}, true},

		// User has more programs than required
		{"user has multiple, pred has one", []string{"alpha", "beta"}, []string{"alpha"}, true},
		{"user has multiple, matches second", []string{"gamma", "beta"}, []string{"beta"}, true},

		// OR logic - user needs at least one matching program
		{"OR logic - one match is enough", []string{"alpha"}, []string{"alpha", "beta"}, true},
		{"OR logic - match second required", []string{"beta"}, []string{"alpha", "beta"}, true},
		{"OR logic - multiple matches", []string{"alpha", "beta"}, []string{"alpha", "beta"}, true},

		// No match
		{"no matching programs", []string{"gamma"}, []string{"alpha", "beta"}, false},
		{"completely different programs", []string{"x", "y", "z"}, []string{"a", "b", "c"}, false},

		// Edge cases with nil/empty
		{"user has no programs, pred requires some", nil, []string{"alpha"}, false},
		{"user has empty programs, pred requires some", []string{}, []string{"alpha"}, false},
		{"user has programs, pred requires none", []string{"alpha"}, nil, true},
		{"user has programs, pred has empty", []string{"alpha"}, []string{}, true},
		{"both nil - no programs required", nil, nil, true},
		{"both empty - no programs required", []string{}, []string{}, true},
		{"user nil, pred empty", nil, []string{}, true},
		{"user empty, pred nil", []string{}, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasProgram(tt.userPrograms, tt.predPrograms)
			require.Equal(t, tt.hasAccess, result)
		})
	}
}
