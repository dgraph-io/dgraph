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
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/utils"
	"github.com/gorilla/websocket"
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

	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	// TODO: improve dot tests #687
	require.NotNil(t, stateSrvc)
}

// Core Service

// TestCreateCoreService tests the createCoreService method
func TestCreateCoreService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	// TODO: improve dot tests #687
	cfg.Core.Authority = false
	cfg.Core.BabeAuthority = false
	cfg.Core.GrandpaAuthority = false
	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.NoError(t, err)

	stateSrvc, err := createStateService(cfg)
	require.NoError(t, err)

	ks := keystore.NewKeystore()
	require.NotNil(t, ks)
	rt, err := createRuntime(cfg, stateSrvc, ks)
	require.NoError(t, err)

	coreMsgs := make(chan network.Message)
	networkMsgs := make(chan network.Message)
	syncChan := make(chan *big.Int)

	coreSrvc, err := createCoreService(cfg, nil, nil, rt, ks, stateSrvc, coreMsgs, networkMsgs, syncChan)
	require.Nil(t, err)

	// TODO: improve dot tests #687
	require.NotNil(t, coreSrvc)
}

// Network Service

// TestCreateNetworkService tests the createNetworkService method
func TestCreateNetworkService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	coreMsgs := make(chan network.Message)
	networkMsgs := make(chan network.Message)
	syncChan := make(chan *big.Int)

	networkSrvc, err := createNetworkService(cfg, stateSrvc, coreMsgs, networkMsgs, syncChan)
	require.Nil(t, err)

	// TODO: improve dot tests #687
	require.NotNil(t, networkSrvc)
}

// RPC Service

// TestCreateRPCService tests the createRPCService method
func TestCreateRPCService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	// TODO: improve dot tests #687
	cfg.Core.Authority = false
	cfg.Core.BabeAuthority = false
	cfg.Core.GrandpaAuthority = false
	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	coreMsgs := make(chan network.Message)
	networkMsgs := make(chan network.Message)

	ks := keystore.NewKeystore()
	rt, err := createRuntime(cfg, stateSrvc, ks)
	require.NoError(t, err)

	coreSrvc, err := createCoreService(cfg, nil, nil, rt, ks, stateSrvc, coreMsgs, networkMsgs, make(chan *big.Int))
	require.Nil(t, err)

	networkSrvc := &network.Service{} // TODO: rpc service without network service

	sysSrvc := createSystemService(&cfg.System)

	rpcSrvc, err := createRPCService(cfg, stateSrvc, coreSrvc, networkSrvc, nil, rt, sysSrvc)
	require.Nil(t, err)

	// TODO: improve dot tests #687
	require.NotNil(t, rpcSrvc)
}

func TestCreateBABEService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	// TODO: improve dot tests #687
	cfg.Core.Authority = true
	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	kr, err := keystore.NewSr25519Keyring()
	require.Nil(t, err)
	ks.Insert(kr.Alice)

	rt, err := createRuntime(cfg, stateSrvc, ks)
	require.NoError(t, err)

	bs, err := createBABEService(cfg, rt, stateSrvc, ks)
	require.NoError(t, err)
	require.NotNil(t, bs)
}

func TestCreateGrandpaService(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	// TODO: improve dot tests #687
	cfg.Core.Authority = true
	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	ks := keystore.NewKeystore()
	kr, err := keystore.NewEd25519Keyring()
	require.Nil(t, err)
	ks.Insert(kr.Alice)

	rt, err := createRuntime(cfg, stateSrvc, ks)
	require.NoError(t, err)

	gs, err := createGRANDPAService(cfg, rt, stateSrvc, ks)
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

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Core.Authority = false
	cfg.Core.BabeAuthority = false
	cfg.Core.GrandpaAuthority = false
	cfg.Init.Genesis = genFile.Name()
	cfg.RPC.WSEnabled = true
	cfg.System.SystemName = "gossamer"

	err := InitNode(cfg)
	require.Nil(t, err)

	stateSrvc, err := createStateService(cfg)
	require.Nil(t, err)

	coreMsgs := make(chan network.Message)
	networkMsgs := make(chan network.Message)

	ks := keystore.NewKeystore()
	rt, err := createRuntime(cfg, stateSrvc, ks)
	require.NoError(t, err)

	coreSrvc, err := createCoreService(cfg, nil, nil, rt, ks, stateSrvc, coreMsgs, networkMsgs, make(chan *big.Int))
	require.Nil(t, err)

	networkSrvc := &network.Service{}

	sysSrvc := createSystemService(&cfg.System)

	rpcSrvc, err := createRPCService(cfg, stateSrvc, coreSrvc, networkSrvc, nil, rt, sysSrvc)
	require.Nil(t, err)

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
