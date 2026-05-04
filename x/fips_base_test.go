/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"os"
	"testing"
)

// TestFIPSEnabledDefault confirms the upstream-pristine default: without any
// FIPS-enforcing tag in effect, FIPSEnabled is false. A tag-guarded sibling
// file in a downstream fork flips it to true via package init.
func TestFIPSEnabledDefault(t *testing.T) {
	if FIPSEnabled {
		t.Fatal("FIPSEnabled must default to false in non-FIPS builds; got true")
	}
}

// TestFIPSBinaryEnv confirms FIPSBinary() reads DGRAPH_FIPS_BINARY: true only
// when set to exactly "1". Covers the common states: unset, empty, "0", "1",
// and a non-"1" value.
func TestFIPSBinaryEnv(t *testing.T) {
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
	origHadValue := false
	orig := ""
	if v, ok := os.LookupEnv("DGRAPH_FIPS_BINARY"); ok {
		origHadValue = true
		orig = v
	}
	t.Cleanup(func() {
		if origHadValue {
			_ = os.Setenv("DGRAPH_FIPS_BINARY", orig)
		} else {
			_ = os.Unsetenv("DGRAPH_FIPS_BINARY")
		}
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "unset" {
				_ = os.Unsetenv("DGRAPH_FIPS_BINARY")
			} else {
				_ = os.Setenv("DGRAPH_FIPS_BINARY", tc.setValue)
			}
			got := FIPSBinary()
			if got != tc.want {
				t.Errorf("FIPSBinary() = %v; want %v (env=%q)", got, tc.want, tc.setValue)
			}
		})
	}
}
