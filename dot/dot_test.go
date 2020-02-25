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
	"math/big"
	"os"
	"path"
	"testing"

	"github.com/ChainSafe/gossamer/core/types"
	"github.com/ChainSafe/gossamer/internal/services"
	"github.com/ChainSafe/gossamer/network"
	"github.com/ChainSafe/gossamer/state"
	"github.com/ChainSafe/gossamer/trie"
)

// Creates a Dot with default configurations. Does not include RPC server.
func createTestDot(t *testing.T, testDir string) *Dot {
	var services []services.Service

	// Network
	networkCfg := &network.Config{
		BlockState:   &state.BlockState{}, // required
		NetworkState: &state.NetworkState{},
		DataDir:      testDir, // default "~/.gossamer"
		Roles:        1,       // required
		RandSeed:     1,       // default 0
	}
	networkSrvc, err := network.NewService(networkCfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	services = append(services, networkSrvc)

	// DB
	dbSrv := state.NewService(testDir)
	err = dbSrv.Initialize(&types.Header{
		Number:    big.NewInt(0),
		StateRoot: trie.EmptyHash,
	}, trie.NewEmptyTrie(nil))
	if err != nil {
		t.Fatal(err)
	}
	services = append(services, dbSrv)

	return NewDot("gossamer", services)
}

func TestDot_Start(t *testing.T) {
	testDir := path.Join(os.TempDir(), "gossamer-test")
	defer os.RemoveAll(testDir)

	availableServices := [...]services.Service{
		&network.Service{},
		&state.Service{},
	}

	dot := createTestDot(t, testDir)

	go dot.Start()

	// Wait until dot.Start() is finished
	<-dot.IsStarted

	for _, srvc := range availableServices {
		s := dot.Services.Get(srvc)
		if s == nil {
			t.Fatalf("error getting service: %T", srvc)
		}
	}

	dot.Stop()

	// Wait for everything to finish
	<-dot.stop
}
