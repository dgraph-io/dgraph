/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

// FIPSEnabled reports whether this binary was built with -tags=fips and
// is therefore restricted to FIPS 140-3 validated cryptography. Default
// false; the //go:build fips init() in buildvars/fips_on.go sets it to
// true at package load before any caller's main() or test body runs.
//
// Test code uses it to skip cases the FIPS-tagged binary cannot satisfy:
//
//	if buildvars.FIPSEnabled {
//	    t.Skip("test requires features unavailable under FIPS")
//	}
//
// Read-only after package init.
var FIPSEnabled = false
