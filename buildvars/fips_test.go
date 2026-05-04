//go:build !fips

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

import "testing"

// TestFIPSEnabled_DefaultFalse confirms the default: without -tags=fips,
// FIPSEnabled is false. Tagged !fips because the //go:build fips init
// in buildvars/fips_on.go flips the flag; the FIPS-on assertion lives
// in the corresponding buildvars/fips_on_test.go (also //go:build fips).
func TestFIPSEnabled_DefaultFalse(t *testing.T) {
	if FIPSEnabled {
		t.Fatal("FIPSEnabled must be false in non-FIPS builds; got true")
	}
}
