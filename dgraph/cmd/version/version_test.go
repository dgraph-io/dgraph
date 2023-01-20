package version

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/testutil"
)

// Test `dgraph version` with an empty config file.
func TestDgraphVersion(t *testing.T) {
	tmpPath, err := ioutil.TempDir("", "test.tmp-")
	require.NoError(t, err)
	defer os.RemoveAll(tmpPath)

	configPath := filepath.Join(tmpPath, "config.yml")
	configFile, err := os.Create(configPath)
	require.NoError(t, err)
	defer configFile.Close()

	err = testutil.Exec(testutil.DgraphBinaryPath(), "version", "--config", configPath)
	require.NoError(t, err)
}
