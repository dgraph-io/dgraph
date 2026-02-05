/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanAccess(t *testing.T) {
	tests := []struct {
		name      string
		userLevel string
		predLabel string
		canAccess bool
	}{
		// Same level access
		{"top_secret can access top_secret", "top_secret", "top_secret", true},
		{"secret can access secret", "secret", "secret", true},
		{"classified can access classified", "classified", "classified", true},
		{"unclassified can access unclassified", "unclassified", "unclassified", true},

		// Higher level can access lower
		{"top_secret can access secret", "top_secret", "secret", true},
		{"top_secret can access classified", "top_secret", "classified", true},
		{"top_secret can access unclassified", "top_secret", "unclassified", true},
		{"secret can access classified", "secret", "classified", true},
		{"secret can access unclassified", "secret", "unclassified", true},
		{"classified can access unclassified", "classified", "unclassified", true},

		// Lower level cannot access higher
		{"secret cannot access top_secret", "secret", "top_secret", false},
		{"classified cannot access top_secret", "classified", "top_secret", false},
		{"classified cannot access secret", "classified", "secret", false},
		{"unclassified cannot access top_secret", "unclassified", "top_secret", false},
		{"unclassified cannot access secret", "unclassified", "secret", false},
		{"unclassified cannot access classified", "unclassified", "classified", false},

		// No level = no access to labeled predicates
		{"empty level cannot access top_secret", "", "top_secret", false},
		{"empty level cannot access secret", "", "secret", false},
		{"empty level cannot access classified", "", "classified", false},
		{"empty level cannot access unclassified", "", "unclassified", false},

		// Unknown levels
		{"unknown user level cannot access", "unknown", "secret", false},
		{"user cannot access unknown pred level", "secret", "unknown", false},
		{"both unknown levels", "foo", "bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanAccess(tt.userLevel, tt.predLabel)
			require.Equal(t, tt.canAccess, result)
		})
	}
}

func TestLevelHierarchy(t *testing.T) {
	// Verify the hierarchy is defined correctly
	require.Equal(t, []string{"top_secret", "secret", "classified", "unclassified"}, LevelHierarchy)
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected int
	}{
		{"top_secret is index 0", "top_secret", 0},
		{"secret is index 1", "secret", 1},
		{"classified is index 2", "classified", 2},
		{"unclassified is index 3", "unclassified", 3},
		{"unknown returns -1", "unknown", -1},
		{"empty returns -1", "", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := indexOf(LevelHierarchy, tt.level)
			require.Equal(t, tt.expected, result)
		})
	}
}
