// Copyright 2019 ChainSafe Systems (ON) Corp.
// This file is part of gossamer.

// The gossamer library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// The gossamer library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.

// You should have received a copy of the GNU Lesser General Public License
// along with the gossamer library. If not, see <http://www.gnu.org/licenses/>.

package api

import (
	apiModule "github.com/ChainSafe/gossamer/internal/api/modules"
	"github.com/ChainSafe/gossamer/internal/services"
)

var _ services.Service = &Service{}

// Service couples all components required for the API.
type Service struct {
	API *API
}

// API contains all the available modules
type API struct {
	NetworkModule *apiModule.NetworkModule
	RuntimeModule *apiModule.RuntimeModule
}

// Module represents a collection of API endpoints.
type Module string

// NewAPIService creates a new API instance.
func NewAPIService(networkAPI apiModule.NetworkAPI, runtimeAPI apiModule.RuntimeAPI) *Service {
	return &Service{
		&API{
			NetworkModule: &apiModule.NetworkModule{
				NetworkAPI: networkAPI,
			},
			RuntimeModule: &apiModule.RuntimeModule{
				RuntimeAPI: runtimeAPI,
			},
		},
	}
}

func (s *Service) Start() error {
	return nil
}

func (s *Service) Stop() error {
	return nil
}
