/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

// FIPS140CryptoRestricted reports whether this binary's cryptographic
// selections are restricted to the FIPS 140 approved-algorithms set.
// Default false. A downstream fork running FIPS-restricted builds sets
// this var to true from a tag-guarded init(), where the tag is whatever
// the fork uses to gate its FIPS-aware code paths. The init() runs
// before any caller's main() or test body.
//
// Test code uses it to skip cases the FIPS-restricted binary cannot
// satisfy:
//
//	if buildvars.FIPS140CryptoRestricted {
//	    t.Skip("test exercises crypto outside the FIPS 140 restrictions")
//	}
//
// This is a build-time fact about which cryptographic choices the
// binary accepts. It does NOT report whether the underlying
// cryptographic implementation is FIPS 140-validated at runtime;
// crypto/fips140.Enabled() answers that.
//
// Read-only after package init.
var FIPS140CryptoRestricted = false
