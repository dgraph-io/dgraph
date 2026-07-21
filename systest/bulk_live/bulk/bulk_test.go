//go:build integration

/*
 * SPDX-FileCopyrightText: © 2017-2026 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package main

import (
	"testing"

	"github.com/dgraph-io/dgraph/v25/systest/bulk_live/common"
)

func TestBulkCases(t *testing.T) {
	t.Run("bulk test cases", common.RunBulkCases)
}

func TestBulkCasesAcl(t *testing.T) {
	t.Run("bulk test cases with acl", common.RunBulkCasesAcl)
}
