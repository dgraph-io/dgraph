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
	"net/http"

	"github.com/ChainSafe/gossamer/p2p"
)

// PublicP2PService offers network related RPC methods
type PublicP2PService struct {
	Net *p2p.Service
}

// PublicP2PResponse represents response from RPC call
type PublicP2PResponse struct {
	Count int
}

// PublicP2PRequest represents RPC request type
type PublicP2PRequest struct{}

// NewPublicRPC creates a new net API instance.
func NewPublicP2PService(net *p2p.Service) *PublicP2PService {
	return &PublicP2PService{
		Net: net,
	}
}

// PeerCount returns the number of connected peers
func (s *PublicP2PService) PeerCount(r *http.Request, args *PublicP2PRequest, res *PublicP2PResponse) error {
	res.Count = s.Net.PeerCount()
	return nil
}
