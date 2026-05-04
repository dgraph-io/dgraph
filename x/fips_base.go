/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import "os"

// FIPSEnabled reports whether the dgraph binary in scope is running under
// FIPS 140-3 enforcement. It returns true if either:
//
//   - this binary was compiled with `-tags=fips` (the tag-guarded init()
//     in x/fips.go flips fipsEnabledCompiled to true at package load), or
//   - the runtime DGRAPH_FIPS_BINARY env var is set to "1" (the signal a
//     test harness uses when its own binary isn't fips-tagged but the
//     dgraph subprocess it launches is).
//
// Test code typically calls this in a single guard:
//
//	if x.FIPSEnabled() {
//	    t.Skip("test requires features unavailable under FIPS")
//	}
//
// Upstream (non-tagged) builds with the env var unset always return false;
// no behavior change.
func FIPSEnabled() bool {
	return fipsEnabledCompiled || FIPSBinary()
}

// FIPSBinary is the runtime-only component of [FIPSEnabled]. Exposed for the
// rare caller that wants to distinguish "the dgraph subprocess we're about
// to launch is FIPS" from "this test binary itself is FIPS-tagged" — most
// call sites should prefer [FIPSEnabled], which ORs both signals.
//
// Returns true when DGRAPH_FIPS_BINARY is set to exactly "1". A downstream
// fork sets the env var from its package init() so any test that spawns a
// dgraph subprocess can skip cases the FIPS-tagged binary cannot satisfy —
// e.g. upgrade-path tests that `git checkout` a pre-FIPS upstream SHA and
// build it under a FIPS toolchain.
func FIPSBinary() bool {
	return os.Getenv("DGRAPH_FIPS_BINARY") == "1"
}

// fipsEnabledCompiled is the compile-time component of [FIPSEnabled].
// Default false; the tag-guarded init() in x/fips.go (under //go:build fips)
// sets it to true when -tags=fips is in effect. Read-only after package init.
var fipsEnabledCompiled = false
