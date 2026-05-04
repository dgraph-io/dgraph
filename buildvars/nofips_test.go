//go:build !fips

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

import "testing"

// TestFIPSEnabled_FalseUnderNoFIPSTag confirms the upstream-pristine
// default: without -tags=fips, FIPSEnabled is false. Companion test in
// buildvars/fips_test.go covers the FIPS-on case.
func TestFIPSEnabled_FalseUnderNoFIPSTag(t *testing.T) {
	if FIPSEnabled {
		t.Fatal("FIPSEnabled must be false in non-FIPS builds; got true")
	}
}
