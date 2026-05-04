/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

// FIPSEnabled reports whether this binary was built with FIPS 140-3
// enforcement and is therefore restricted to validated cryptography.
// Default false. A downstream fork ships a tag-guarded sibling file in
// this package (e.g. //go:build fips) that flips it to true at package
// load before any caller's main() or test body runs.
//
// Test code uses it to skip cases the FIPS-tagged binary cannot satisfy:
//
//	if buildvars.FIPSEnabled {
//	    t.Skip("test requires features unavailable under FIPS")
//	}
//
// Read-only after package init.
var FIPSEnabled = false
