/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

import "os"

// FIPSEnabled reports whether the dgraph binary in scope is running under
// FIPS 140-3 enforcement. It returns true if either:
//
//   - this binary was compiled with `-tags=requirefips` (the tag-guarded
//     init() in buildvars/fips_requirefips.go flips an internal flag at
//     package load), or
//   - the environment variable DGRAPH_FIPS_BINARY is set to "1" (the
//     runtime signal a test harness uses when its own binary isn't
//     FIPS-tagged but the dgraph subprocess it launches is).
//
// Test code typically calls this in a single guard:
//
//	if buildvars.FIPSEnabled() {
//	    t.Skip("test requires features unavailable under FIPS")
//	}
//
// Upstream (non-tagged) builds with the env var unset always return false;
// no behavior change.
func FIPSEnabled() bool {
	return fipsEnabledCompiled || os.Getenv("DGRAPH_FIPS_BINARY") == "1"
}

// fipsEnabledCompiled is the compile-time component of [FIPSEnabled].
// Default false; the tag-guarded init() in buildvars/fips_requirefips.go
// sets it to true when -tags=requirefips is in effect. Read-only after
// package init.
var fipsEnabledCompiled = false
