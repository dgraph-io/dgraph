/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package version

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/v25/testutil"
)

// Test `dgraph version` with an empty config file.
func TestMain(m *testing.M) {
	m.Run()
}

func TestDgraphVersion(t *testing.T) {
	tmpPath := t.TempDir()
	configPath := filepath.Join(tmpPath, "config.yml")
	configFile, err := os.Create(configPath)
	require.NoError(t, err)
	defer configFile.Close()
	require.NoError(t, testutil.Exec(testutil.DgraphBinaryPath(), "version", "--config", configPath))
}
