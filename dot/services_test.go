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

package dot

import (
	"testing"

	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/stretchr/testify/require"
)

// State Service

// TestCreateStateService tests the createStateService method
func TestCreateStateService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Global.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	// TODO: improve dot service tests
	require.NotNil(t, stateSrvc)
}

// Network Service

// TestCreateNetworkService tests the createNetworkService method
func TestCreateNetworkService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Global.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	networkSrvc, networkMsgSend, networkMsgRec := createNetworkService(cfg, stateSrvc)
	require.Nil(t, err)

	// TODO: improve dot service tests
	require.NotNil(t, networkSrvc)
	require.NotNil(t, networkMsgSend)
	require.NotNil(t, networkMsgRec)
}

// Core Service

// TestCreateCoreService tests the createCoreService method
func TestCreateCoreService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Core.Authority = false // TODO: improve dot service tests
	cfg.Global.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	_, networkMsgSend, networkMsgRec := createNetworkService(cfg, stateSrvc)
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	require.NotNil(t, ks)

	coreSrvc, err := createCoreService(cfg, ks, stateSrvc, networkMsgSend, networkMsgRec)
	require.Nil(t, err)

	// TODO: improve dot service tests
	require.NotNil(t, coreSrvc)
}

// RPC Service

// TestCreateRPCService tests the createRPCService method
func TestCreateRPCService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Core.Authority = false // TODO: improve dot service tests
	cfg.Global.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	networkSrvc, networkMsgSend, networkMsgRec := createNetworkService(cfg, stateSrvc)
	require.Nil(t, err)

	ks := keystore.NewKeystore()

	coreSrvc, err := createCoreService(cfg, ks, stateSrvc, networkMsgSend, networkMsgRec)
	require.Nil(t, err)

	rpcSrvc := createRPCService(cfg, stateSrvc, networkSrvc, coreSrvc)
	require.Nil(t, err)

	// TODO: improve dot service tests
	require.NotNil(t, rpcSrvc)
}
