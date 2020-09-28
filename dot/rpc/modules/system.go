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

package modules

import (
	"fmt"
	"net/http"

	"github.com/ChainSafe/gossamer/lib/common"
)

// SystemModule is an RPC module providing access to core API points
type SystemModule struct {
	networkAPI NetworkAPI
	systemAPI  SystemAPI
}

// EmptyRequest represents an RPC request with no fields
type EmptyRequest struct{}

// StringResponse holds the string response
type StringResponse string

// SystemHealthResponse struct to marshal json
type SystemHealthResponse struct {
	Health common.Health `json:"health"`
}

// NetworkStateString Network State represented as string so JSON encode/decoding works
type NetworkStateString struct {
	PeerID     string
	Multiaddrs []string
}

// SystemNetworkStateResponse struct to marshal json
type SystemNetworkStateResponse struct {
	NetworkState NetworkStateString `json:"networkState"`
}

// SystemPeersResponse struct to marshal json
type SystemPeersResponse struct {
	Peers []common.PeerInfo `json:"peers"`
}

// NewSystemModule creates a new API instance
func NewSystemModule(net NetworkAPI, sys SystemAPI) *SystemModule {
	return &SystemModule{
		networkAPI: net, // TODO: migrate to network state
		systemAPI:  sys,
	}
}

// Chain returns the runtime chain
func (sm *SystemModule) Chain(r *http.Request, req *EmptyRequest, res *string) error {
	*res = sm.systemAPI.NodeName()
	return nil
}

// Name returns the runtime name
func (sm *SystemModule) Name(r *http.Request, req *EmptyRequest, res *string) error {
	*res = sm.systemAPI.SystemName()
	return nil
}

// Properties returns the runtime properties
func (sm *SystemModule) Properties(r *http.Request, req *EmptyRequest, res *interface{}) error {
	*res = sm.systemAPI.Properties()
	return nil
}

// Version returns the runtime version
func (sm *SystemModule) Version(r *http.Request, req *EmptyRequest, res *string) error {
	*res = sm.systemAPI.SystemVersion()
	return nil
}

// Health returns the information about the health of the network
func (sm *SystemModule) Health(r *http.Request, req *EmptyRequest, res *SystemHealthResponse) error {
	health := sm.networkAPI.Health()
	res.Health = health
	return nil
}

// NetworkState returns the network state (basic information about the host)
func (sm *SystemModule) NetworkState(r *http.Request, req *EmptyRequest, res *SystemNetworkStateResponse) error {
	networkState := sm.networkAPI.NetworkState()
	res.NetworkState.PeerID = networkState.PeerID
	for _, v := range networkState.Multiaddrs {
		fmt.Printf("addr %v\n", v)
	}
	return nil
}

// Peers returns peer information for each connected and confirmed peer
func (sm *SystemModule) Peers(r *http.Request, req *EmptyRequest, res *SystemPeersResponse) error {
	peers := sm.networkAPI.Peers()
	res.Peers = peers
	return nil
}

// NodeRoles Returns the roles the node is running as.
func (sm *SystemModule) NodeRoles(r *http.Request, req *EmptyRequest, res *[]interface{}) error {
	resultArray := []interface{}{}

	role := sm.networkAPI.NodeRoles()
	switch role {
	case 1:
		resultArray = append(resultArray, "Full")
	case 2:
		resultArray = append(resultArray, "LightClient")
	case 4:
		resultArray = append(resultArray, "Authority")
	default:
		resultArray = append(resultArray, "UnknownRole")
		uknrole := []interface{}{}
		uknrole = append(uknrole, role)
		resultArray = append(resultArray, uknrole)
	}

	*res = resultArray
	return nil
}
