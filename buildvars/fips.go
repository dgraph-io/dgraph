/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

// FIPSEnabled reports whether this binary was built with FIPS 140-3
// enforcement and is restricted to validated cryptography. Default
// false. A downstream fork running FIPS-enforced builds sets this var
// to true from a tag-guarded init(), where the tag is whatever the fork
// uses to gate its FIPS-enforcing code paths. The init() runs before
// any caller's main() or test body.
//
// Test code uses it to skip cases the FIPS-tagged binary cannot satisfy:
//
//	if buildvars.FIPSEnabled {
//	    t.Skip("test requires features unavailable under FIPS")
//	}
//
// Read-only after package init.
var FIPSEnabled = false
