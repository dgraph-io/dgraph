//go:build !fips

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

// FIPSEnabled is false in non-FIPS builds (the upstream-pristine
// default). Companion declaration in buildvars/fips.go (//go:build fips)
// sets it to true; see that file for the docstring.
var FIPSEnabled = false
