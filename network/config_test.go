package network

import (
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/state"
	"github.com/stretchr/testify/require"
)

// test buildIdentity method
func TestBuildIdentity(t *testing.T) {
	testDir := path.Join(os.TempDir(), "gossamer-test")
	defer os.RemoveAll(testDir)

	configA := &Config{
		DataDir: testDir,
	}

	err := configA.buildIdentity()
	if err != nil {
		t.Fatal(err)
	}

	configB := &Config{
		DataDir: testDir,
	}

	err = configB.buildIdentity()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(configA.privateKey, configB.privateKey) {
		t.Error("Private keys should match")
	}

	configC := &Config{
		RandSeed: 1,
	}

	err = configC.buildIdentity()
	if err != nil {
		t.Fatal(err)
	}

	configD := &Config{
		RandSeed: 2,
	}

	err = configD.buildIdentity()
	if err != nil {
		t.Fatal(err)
	}

	if reflect.DeepEqual(configC.privateKey, configD.privateKey) {
		t.Error("Private keys should not match")
	}
}

// test build configuration method
func TestBuild(t *testing.T) {
	testDataDir := path.Join(os.TempDir(), "gossamer-test")
	defer os.RemoveAll(testDataDir)

	testBlockState := &state.BlockState{}
	testNetworkState := &state.NetworkState{}

	testRandSeed := int64(1)

	cfg := &Config{
		BlockState:   testBlockState,
		NetworkState: testNetworkState,
		DataDir:      testDataDir,
		RandSeed:     testRandSeed,
	}

	err := cfg.build()
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, testBlockState, cfg.BlockState)
	require.Equal(t, testNetworkState, cfg.NetworkState)
	require.Equal(t, testDataDir, cfg.DataDir)
	require.Equal(t, DefaultRoles, cfg.Roles)
	require.Equal(t, DefaultPort, cfg.Port)
	require.Equal(t, testRandSeed, cfg.RandSeed)
	require.Equal(t, DefaultBootnodes, cfg.Bootnodes)
	require.Equal(t, DefaultProtocolID, cfg.ProtocolID)
	require.Equal(t, false, cfg.NoBootstrap)
	require.Equal(t, false, cfg.NoMdns)
}
