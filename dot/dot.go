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
	"github.com/ChainSafe/gossamer/internal/services"
	"github.com/ChainSafe/gossamer/rpc"
	log "github.com/ChainSafe/log15"
)

// Dot is a container for all the components of a node.
type Dot struct {
	Services  *services.ServiceRegistry // Registry of all core services
	Rpc       *rpc.HttpServer           // HTTP instance for RPC server
	IsStarted chan struct{}             // Signals node startup complete
	stop      chan struct{}             // Used to signal node shutdown
}

// NewDot initializes a Dot with provided components.
func NewDot(srvcs []services.Service, rpc *rpc.HttpServer) *Dot {
	d := &Dot{
		Services:  services.NewServiceRegistry(),
		Rpc:       rpc,
		IsStarted: make(chan struct{}),
		stop:      nil,
	}

	for _, srvc := range srvcs {
		d.Services.RegisterService(srvc)
	}

	return d
}

// Start starts all services. API service is started last.
func (d *Dot) Start() {
	log.Debug("Starting core services.")
	d.Services.StartAll()
	if d.Rpc != nil {
		d.Rpc.Start()
	}

	d.stop = make(chan struct{})
	d.IsStarted <- struct{}{}
	d.Wait()
}

// Wait is used to force the node to stay alive until a signal is passed into `Dot.stop`
func (d *Dot) Wait() {
	<-d.stop
}

func (d *Dot) Stop() {
	// TODO: Shutdown services and exit
	d.stop <- struct{}{}
}
