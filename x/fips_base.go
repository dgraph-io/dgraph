/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import "os"

// FIPSEnabled reports whether this binary was built with the FIPS-enforcement
// build tag (requirefips) and is therefore restricted to FIPS 140-3 validated
// cryptography. Declared as a var (not a const) so a FIPS-tagged sibling file
// can flip the value from package init without the cost of a build-tag split
// at every call site.
//
// Upstream (non-tagged) builds: FIPSEnabled == false, unchanged behavior.
// FIPS-tagged builds: an init() in the tag-guarded x/fips.go sets it to true
// before any test or main() body runs, so `if x.FIPSEnabled { ... }` guards
// behave identically to a compile-time constant for practical purposes.
//
// The value is only written during package init; after init it is effectively
// read-only. No synchronization is required.
var FIPSEnabled = false

// FIPSBinary reports whether the dgraph binary under test (not necessarily the
// test binary itself) is a FIPS-restricted build. Returns true when the
// environment variable DGRAPH_FIPS_BINARY is set to "1".
//
// Test helpers use this in tandem with FIPSEnabled: FIPSEnabled is the
// compile-time signal (this binary is FIPS), FIPSBinary is the runtime signal
// (the child/cluster binary this test will launch is FIPS). A downstream fork
// sets the env var from its package init() so any test that spawns a dgraph
// subprocess can skip cases the FIPS-tagged binary cannot satisfy — e.g.
// upgrade-path tests that `git checkout` a pre-FIPS upstream SHA and build
// it under a FIPS toolchain.
//
// Upstream users: leave DGRAPH_FIPS_BINARY unset (or "0") for normal behavior.
// Set to "1" when running a test harness against a FIPS-restricted dgraph
// binary to opt into the FIPS-aware test skips.
func FIPSBinary() bool {
	return os.Getenv("DGRAPH_FIPS_BINARY") == "1"
}
