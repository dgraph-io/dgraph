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
	"os"
	"testing"

	"github.com/ChainSafe/gossamer/internal/api"
	"github.com/ChainSafe/gossamer/internal/services"
	"github.com/ChainSafe/gossamer/p2p"
	"github.com/ChainSafe/gossamer/polkadb"
)

// Creates a Dot with default configurations. Does not include RPC server.
func createTestDot(t *testing.T) *Dot {
	var services []services.Service
	// P2P
	p2pCfg := &p2p.Config{
		BootstrapNodes: []string{},
		Port:           7000,
		RandSeed:       1,
		NoBootstrap:    false,
		NoMdns:         false,
		DataDir:        "",
	}
	p2pSrvc, err := p2p.NewService(p2pCfg, nil)
	services = append(services, p2pSrvc)
	if err != nil {
		t.Fatal(err)
	}

	// DB
	dataDir := "../test_data"
	dbSrv, err := polkadb.NewDbService(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	services = append(services, dbSrv)

	// API
	apiSrvc := api.NewApiService(p2pSrvc, nil)
	services = append(services, apiSrvc)

	return NewDot("gossamer", services, nil)
}

func TestDot_Start(t *testing.T) {
	var availableServices = [...]services.Service{
		&p2p.Service{},
		&api.Service{},
		&polkadb.DbService{},
	}

	dot := createTestDot(t)

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

	defer func() {
		if err := os.RemoveAll("../test_data"); err != nil {
			t.Log("removal of temp directory test_data failed", "error", err)
		}
	}()
}
