// Copyright 2020 ChainSafe Systems (ON) Corp.
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

package system

import "github.com/ChainSafe/gossamer/dot/types"

// Service struct to hold rpc service data
type Service struct {
	systemInfo *types.SystemInfo
}

// NewService create a new instance of Service
func NewService(si *types.SystemInfo) *Service {
	return &Service{
		systemInfo: si,
	}
}

// SystemName returns the app name
func (s *Service) SystemName() string {
	return s.systemInfo.SystemName
}

// SystemVersion returns the app version
func (s *Service) SystemVersion() string {
	return s.systemInfo.SystemVersion
}

// NodeName returns the nodeName (chain name)
func (s *Service) NodeName() string {
	return s.systemInfo.NodeName
}

// Properties Get a custom set of properties as a JSON object, defined in the chain spec.
func (s *Service) Properties() map[string]interface{} {
	return s.systemInfo.SystemProperties
}

// Start implements Service interface
func (s *Service) Start() error {
	return nil
}

// Stop implements Service interface
func (s *Service) Stop() error {
	return nil
}
