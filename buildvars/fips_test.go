/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package buildvars

import "testing"

// TestFIPSEnabled_DefaultFalse confirms the upstream-pristine default:
// FIPSEnabled is false. Forks that flip it via a tag-guarded sibling
// file run their own assertion under that tag.
func TestFIPSEnabled_DefaultFalse(t *testing.T) {
	if FIPSEnabled {
		t.Fatal("FIPSEnabled must be false in upstream builds; got true")
	}
}
