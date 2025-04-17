//go:build integration

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package protos

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/testutil"
)

func TestProtosRegenerate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping test on non-Linux platform")
	}

	err := testutil.Exec("make", "regenerate")
	require.NoError(t, err, "Got error while regenerating protos: %v\n", err)

	generatedProtos := filepath.Join("pb", "pb.pb.go")
	err = testutil.Exec("git", "diff", "-b", "--quiet", "--", generatedProtos)
	require.NoError(t, err, "pb.pb.go changed after regenerating")
}
