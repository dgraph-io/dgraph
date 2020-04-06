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

// TestInitNode
func TestInitNode(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)
}

// TestNodeInitialized

// TODO: improve dot node tests

// TestNewNode
func TestNewNode(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	ks, err := keystore.LoadKeystore("alice")
	require.Nil(t, err)
	require.NotNil(t, ks)

	cfg.Core.Authority = false // TODO: improve dot node tests

	_, err = NewNode(cfg, ks)
	require.Nil(t, err)
}

// TestStartNode
func TestStartNode(t *testing.T) {
	cfg := NewTestConfig(t)
	require.NotNil(t, cfg)

	genFile := NewTestGenesisFile(t, cfg)
	require.NotNil(t, genFile)

	defer utils.RemoveTestDir(t)

	cfg.Init.Genesis = genFile.Name()

	err := InitNode(cfg)
	require.Nil(t, err)

	ks, err := keystore.LoadKeystore("alice")
	require.Nil(t, err)
	require.NotNil(t, ks)

	cfg.Core.Authority = false // TODO: improve dot node tests

	node, err := NewNode(cfg, ks)
	require.Nil(t, err)

	go node.Start()
	<-node.IsStarted

	node.Stop()
	<-node.stop
}

// TestStopNode

// TODO: improve dot node tests
