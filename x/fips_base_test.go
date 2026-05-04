/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"os"
	"testing"
)

// TestFIPSEnabled_DefaultFalse confirms the upstream-pristine default:
// without -tags=fips and with DGRAPH_FIPS_BINARY unset, FIPSEnabled
// returns false. Under -tags=fips, the tag-guarded init in x/fips.go
// flips fipsEnabledCompiled to true and the untagged build of this test
// isn't compiled — so it's safe to assert false here unconditionally.
func TestFIPSEnabled_DefaultFalse(t *testing.T) {
	saveAndUnsetEnv(t, "DGRAPH_FIPS_BINARY")
	if FIPSEnabled() {
		t.Fatal("FIPSEnabled() must return false in non-FIPS builds with " +
			"DGRAPH_FIPS_BINARY unset; got true")
	}
}

// TestFIPSEnabled_ReadsDgraphFIPSBinary confirms the runtime signal:
// FIPSEnabled() returns true when DGRAPH_FIPS_BINARY is set to exactly
// "1", regardless of the compile-time tag. Covers the common edge cases:
// unset, empty, "0", "1", and other non-"1" values.
func TestFIPSEnabled_ReadsDgraphFIPSBinary(t *testing.T) {
	saveAndUnsetEnv(t, "DGRAPH_FIPS_BINARY")
	cases := []struct {
		name     string
		setValue string // empty string means unset
		want     bool
	}{
		{"unset", "", false},
		{"empty", "", false},
		{"zero", "0", false},
		{"one", "1", true},
		{"true-literal", "true", false},
		{"leading-space", " 1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "unset" {
				_ = os.Unsetenv("DGRAPH_FIPS_BINARY")
			} else {
				t.Setenv("DGRAPH_FIPS_BINARY", tc.setValue)
			}
			if got := FIPSEnabled(); got != tc.want {
				t.Errorf("FIPSEnabled() = %v; want %v (env=%q)",
					got, tc.want, tc.setValue)
			}
			// FIPSBinary should agree with the runtime-only side
			// regardless of compile-time tag — verify directly.
			if got := FIPSBinary(); got != tc.want {
				t.Errorf("FIPSBinary() = %v; want %v (env=%q)",
					got, tc.want, tc.setValue)
			}
		})
	}
}

// saveAndUnsetEnv captures the current value of name (if any), unsets it,
// and registers a cleanup to restore the original state. Avoids leaking
// test-set env values across cases.
func saveAndUnsetEnv(t *testing.T, name string) {
	t.Helper()
	orig, had := os.LookupEnv(name)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(name, orig)
		} else {
			_ = os.Unsetenv(name)
		}
	})
	_ = os.Unsetenv(name)
}
