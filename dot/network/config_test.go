// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.
//
// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package network

import (
	"reflect"
	"testing"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/utils"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

// test buildIdentity method
func TestBuildIdentity(t *testing.T) {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)

	configA := &Config{
		logger:   log.New("srvc", "NET"),
		BasePath: testDir,
	}

	err := configA.buildIdentity()
	if err != nil {
		t.Fatal(err)
	}

	configB := &Config{
		logger:   log.New("srvc", "NET"),
		BasePath: testDir,
	}

	err = configB.buildIdentity()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(configA.privateKey, configB.privateKey) {
		t.Error("Private keys should match")
	}

	configC := &Config{
		logger:   log.New("srvc", "NET"),
		RandSeed: 1,
	}

	err = configC.buildIdentity()
	if err != nil {
		t.Fatal(err)
	}

	configD := &Config{
		logger:   log.New("srvc", "NET"),
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
	testBasePath := utils.NewTestBasePath(t, "node")
	defer utils.RemoveTestDir(t)

	testBlockState := &state.BlockState{}
	testNetworkState := &state.NetworkState{}

	testRandSeed := int64(1)

	cfg := &Config{
		logger:       log.New("srvc", "NET"),
		BlockState:   testBlockState,
		NetworkState: testNetworkState,
		BasePath:     testBasePath,
		RandSeed:     testRandSeed,
	}

	err := cfg.build()
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, testBlockState, cfg.BlockState)
	require.Equal(t, testNetworkState, cfg.NetworkState)
	require.Equal(t, testBasePath, cfg.BasePath)
	require.Equal(t, DefaultRoles, cfg.Roles)
	require.Equal(t, DefaultPort, cfg.Port)
	require.Equal(t, testRandSeed, cfg.RandSeed)
	require.Equal(t, DefaultBootnodes, cfg.Bootnodes)
	require.Equal(t, DefaultProtocolID, cfg.ProtocolID)
	require.Equal(t, false, cfg.NoBootstrap)
	require.Equal(t, false, cfg.NoMDNS)
}
