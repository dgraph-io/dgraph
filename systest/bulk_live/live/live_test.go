//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package live

import (
	"testing"

	"github.com/hypermodeinc/dgraph/v25/systest/bulk_live/common"
)

func TestLiveCases(t *testing.T) {
	t.Run("live test cases", common.RunLiveCases)
}
