//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package live

import (
	"testing"

	"github.com/dgraph-io/dgraph/v25/systest/bulk_live/common"
)

func TestLiveCases(t *testing.T) {
	t.Run("live test cases", common.RunLiveCases)
}
