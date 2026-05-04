//go:build requirefips

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

// init flips the compile-time component of [FIPSEnabled] to true whenever
// the `requirefips` build tag is in effect. Runs at package load before
// any caller's main() or test body, so [FIPSEnabled] reads consistently
// from the start.
func init() {
	fipsEnabledCompiled = true
}
