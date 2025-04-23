/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package version

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hypermodeinc/dgraph/v25/testutil"
)

// Test `dgraph version` with an empty config file.
func TestDgraphVersion(t *testing.T) {
	tmpPath := t.TempDir()
	configPath := filepath.Join(tmpPath, "config.yml")
	configFile, err := os.Create(configPath)
	require.NoError(t, err)
	defer configFile.Close()
	require.NoError(t, testutil.Exec(testutil.DgraphBinaryPath(), "version", "--config", configPath))
}
