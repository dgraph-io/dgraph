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

// Service couples all components required for the API.
type Service struct {
	Api *Api
	err <-chan error
}

// Api contains all the available modules
type Api struct {
	System *systemModule
}

// P2pApi is the interface expected to implemented by `p2p` package
type P2pApi interface {
	PeerCount() int
}

// RuntimeApi is the interface expected to implemented by `runtime` package
type RuntimeApi interface {
	Version() string
}

// Module represents a collection of API endpoints.
type Module string

// NewApiService creates a new API instance.
func NewApiService(p2p P2pApi, rt RuntimeApi) *Service {
	return &Service{
		&Api{
			System: &systemModule{
				p2p,
				rt,
			},
		}, nil,
	}
}

// Start creates, stores and returns an error channel
func (s *Service) Start() <-chan error {
	s.err = make(chan error)
	return s.err
}

func (s *Service) Stop() <-chan error {
	return nil
}
