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
	"flag"
	"net/url"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/utils"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// TestCreateStateService tests the createStateService method
func TestCreateStateService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisRawFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.GenesisRaw = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	// TODO: improve dot tests #687
	require.NotNil(t, stateSrvc)
}

// TestCreateCoreService tests the createCoreService method
func TestCreateCoreService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisRawFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	// TODO: improve dot tests #687
	cfg.Core.Roles = types.FullNodeRole
	cfg.Core.BabeAuthority = false
	cfg.Core.GrandpaAuthority = false
	cfg.Init.GenesisRaw = genFile.Name()

	err := InitNode(cfg)
	require.NoError(t, err)

	stateSrvc, err := createStateService(cfg)
	require.NoError(t, err)

	ks := keystore.NewGlobalKeystore()
	require.NotNil(t, ks)
	ed25519Keyring, _ := keystore.NewEd25519Keyring()
	ks.Gran.Insert(ed25519Keyring.Alice())

	networkSrvc := &network.Service{}

	rt, err := createRuntime(cfg, stateSrvc, ks.Acco.(*keystore.GenericKeystore), networkSrvc)
	require.NoError(t, err)

	dh, err := createDigestHandler(stateSrvc, nil, nil)
	require.NoError(t, err)

	gs, err := createGRANDPAService(cfg, rt, stateSrvc, dh, ks.Gran)
	require.NoError(t, err)

	coreSrvc, err := createCoreService(cfg, nil, gs, nil, rt, ks, stateSrvc, networkSrvc)
	require.Nil(t, err)

	// TODO: improve dot tests #687
	require.NotNil(t, coreSrvc)
}

func TestCreateBlockVerifier(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.GenesisRaw = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.NoError(t, err)

	ks := keystore.NewGlobalKeystore()
	require.NotNil(t, ks)
	rt, err := createRuntime(cfg, stateSrvc, ks.Acco.(*keystore.GenericKeystore), &network.Service{})
	require.NoError(t, err)

	cfg.Core.BabeThreshold = nil
	_, err = createBlockVerifier(cfg, stateSrvc, rt)
	require.NoError(t, err)
}

func TestCreateSyncService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.GenesisRaw = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.NoError(t, err)

	ks := keystore.NewGlobalKeystore()
	require.NotNil(t, ks)
	rt, err := createRuntime(cfg, stateSrvc, ks.Acco.(*keystore.GenericKeystore), &network.Service{})
	require.NoError(t, err)

	cfg.Core.BabeThreshold = nil
	ver, err := createBlockVerifier(cfg, stateSrvc, rt)
	require.NoError(t, err)

	_, err = createSyncService(cfg, stateSrvc, nil, nil, ver, rt)
	require.NoError(t, err)
}

// TestCreateNetworkService tests the createNetworkService method
func TestCreateNetworkService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisRawFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.GenesisRaw = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	networkSrvc, err := createNetworkService(cfg, stateSrvc, nil)
	require.Nil(t, err)

	// TODO: improve dot tests #687
	require.NotNil(t, networkSrvc)
}

// TestCreateRPCService tests the createRPCService method
func TestCreateRPCService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisRawFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	// TODO: improve dot tests #687
	cfg.Core.Roles = types.FullNodeRole
	cfg.Core.BabeAuthority = false
	cfg.Core.GrandpaAuthority = false
	cfg.Init.GenesisRaw = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	networkSrvc := &network.Service{}

	ks := keystore.NewGlobalKeystore()
	ed25519Keyring, _ := keystore.NewEd25519Keyring()
	ks.Gran.Insert(ed25519Keyring.Alice())

	rt, err := createRuntime(cfg, stateSrvc, ks.Acco.(*keystore.GenericKeystore), networkSrvc)
	require.NoError(t, err)

	dh, err := createDigestHandler(stateSrvc, nil, nil)
	require.NoError(t, err)

	gs, err := createGRANDPAService(cfg, rt, stateSrvc, dh, ks.Gran)
	require.NoError(t, err)

	coreSrvc, err := createCoreService(cfg, nil, gs, nil, rt, ks, stateSrvc, networkSrvc)
	require.Nil(t, err)

	sysSrvc := createSystemService(&cfg.System)

	rpcSrvc := createRPCService(cfg, stateSrvc, coreSrvc, networkSrvc, nil, rt, sysSrvc)
	require.NotNil(t, rpcSrvc)
}

func TestCreateBABEService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisRawFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	// TODO: improve dot tests #687
	cfg.Core.Roles = types.FullNodeRole
	cfg.Init.GenesisRaw = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	ks := keystore.NewGlobalKeystore()
	kr, err := keystore.NewSr25519Keyring()
	require.Nil(t, err)
	ks.Babe.Insert(kr.Alice())

	rt, err := createRuntime(cfg, stateSrvc, ks.Acco.(*keystore.GenericKeystore), &network.Service{})
	require.NoError(t, err)

	bs, err := createBABEService(cfg, rt, stateSrvc, ks.Babe)
	require.NoError(t, err)
	require.NotNil(t, bs)
}

func TestCreateGrandpaService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisRawFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	// TODO: improve dot tests #687
	cfg.Core.Roles = types.AuthorityRole
	cfg.Init.GenesisRaw = genFile.Name()

	err := InitNode(cfg)
	require.NoError(t, err)

	stateSrvc, err := createStateService(cfg)
	require.NoError(t, err)

	ks := keystore.NewGlobalKeystore()
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)
	ks.Gran.Insert(kr.Alice())

	rt, err := createRuntime(cfg, stateSrvc, ks.Acco.(*keystore.GenericKeystore), &network.Service{})
	require.NoError(t, err)

	dh, err := createDigestHandler(stateSrvc, nil, nil)
	require.NoError(t, err)

	gs, err := createGRANDPAService(cfg, rt, stateSrvc, dh, ks.Gran)
	require.NoError(t, err)
	require.NotNil(t, gs)
}

var addr = flag.String("addr", "localhost:8546", "http service address")
var testCalls = []struct {
	call     []byte
	expected []byte
}{
	{[]byte(`{"jsonrpc":"2.0","method":"system_name","params":[],"id":1}`), []byte(`{"id":1,"jsonrpc":"2.0","result":"gossamer"}` + "\n")},                                                            // working request
	{[]byte(`{"jsonrpc":"2.0","method":"unknown","params":[],"id":2}`), []byte(`{"error":{"code":-32000,"data":null,"message":"rpc error method unknown not found"},"id":2,"jsonrpc":"2.0"}` + "\n")}, // unknown method
	{[]byte{}, []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid request"},"id":0}` + "\n")},                                                                                         // empty request
	{[]byte(`{"jsonrpc":"2.0","method":"chain_subscribeNewHeads","params":[],"id":3}`), []byte(`{"jsonrpc":"2.0","result":1,"id":3}` + "\n")},
	{[]byte(`{"jsonrpc":"2.0","method":"state_subscribeStorage","params":[],"id":4}`), []byte(`{"jsonrpc":"2.0","result":2,"id":4}` + "\n")},
}

func TestNewWebSocketServer(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisRawFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Core.Roles = types.FullNodeRole
	cfg.Core.BabeAuthority = false
	cfg.Core.GrandpaAuthority = false
	cfg.Init.GenesisRaw = genFile.Name()
	cfg.RPC.WSEnabled = true
	cfg.System.SystemName = "gossamer"

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	networkSrvc := &network.Service{}

	ks := keystore.NewGlobalKeystore()
	ed25519Keyring, _ := keystore.NewEd25519Keyring()
	ks.Gran.Insert(ed25519Keyring.Alice())
	rt, err := createRuntime(cfg, stateSrvc, ks.Acco.(*keystore.GenericKeystore), networkSrvc)
	require.NoError(t, err)

	dh, err := createDigestHandler(stateSrvc, nil, nil)
	require.NoError(t, err)

	gs, err := createGRANDPAService(cfg, rt, stateSrvc, dh, ks.Gran)
	require.NoError(t, err)

	coreSrvc, err := createCoreService(cfg, nil, gs, nil, rt, ks, stateSrvc, networkSrvc)
	require.Nil(t, err)

	sysSrvc := createSystemService(&cfg.System)

	rpcSrvc := createRPCService(cfg, stateSrvc, coreSrvc, networkSrvc, nil, rt, sysSrvc)
	err = rpcSrvc.Start()
	require.Nil(t, err)

	time.Sleep(time.Second) // give server a second to start

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/"}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c.Close()

	for _, item := range testCalls {
		err = c.WriteMessage(websocket.TextMessage, item.call)
		require.Nil(t, err)

		_, message, err := c.ReadMessage()
		require.Nil(t, err)
		require.Equal(t, item.expected, message)
	}
}
