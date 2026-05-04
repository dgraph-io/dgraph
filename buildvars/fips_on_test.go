//go:build fips

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

import "testing"

// TestFIPSEnabled_TrueUnderFIPSTag confirms the //go:build fips init in
// buildvars/fips_on.go flipped FIPSEnabled to true. The companion
// fips_test.go (//go:build !fips) asserts the default-false case.
func TestFIPSEnabled_TrueUnderFIPSTag(t *testing.T) {
	if !FIPSEnabled {
		t.Fatal("FIPSEnabled must be true under -tags=fips; got false")
	}
}
