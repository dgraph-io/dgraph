//go:build fips

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

// init flips [FIPSEnabled] to true under -tags=fips. Runs at package
// load before any caller's main() or test body, so reads are consistent
// from the start.
func init() {
	FIPSEnabled = true
}
