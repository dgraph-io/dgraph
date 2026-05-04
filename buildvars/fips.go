//go:build fips

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

// FIPSEnabled reports whether this binary was built with -tags=fips and
// is therefore restricted to FIPS 140-3 validated cryptography.
//
// Test code uses it to skip cases the FIPS-tagged binary cannot satisfy:
//
//	if buildvars.FIPSEnabled {
//	    t.Skip("test requires features unavailable under FIPS")
//	}
//
// Companion declaration in buildvars/nofips.go (//go:build !fips) sets
// the var to false; only one of the two files compiles into any given
// build, matching Go's stdlib boring/notboring convention.
var FIPSEnabled = true
