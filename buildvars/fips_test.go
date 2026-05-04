//go:build fips

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

import "testing"

// TestFIPSEnabled_TrueUnderFIPSTag confirms FIPSEnabled is true under
// -tags=fips (set by buildvars/fips.go's declaration). Companion test
// in buildvars/nofips_test.go covers the default-false case.
func TestFIPSEnabled_TrueUnderFIPSTag(t *testing.T) {
	if !FIPSEnabled {
		t.Fatal("FIPSEnabled must be true under -tags=fips; got false")
	}
}
