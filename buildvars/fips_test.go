/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

import "testing"

// TestFIPS140CryptoRestricted_DefaultFalse confirms the upstream-pristine
// default: FIPS140CryptoRestricted is false. Forks that flip it via a
// tag-guarded sibling file run their own assertion under that tag.
func TestFIPS140CryptoRestricted_DefaultFalse(t *testing.T) {
	if FIPS140CryptoRestricted {
		t.Fatal("FIPS140CryptoRestricted must be false in upstream builds; got true")
	}
}
