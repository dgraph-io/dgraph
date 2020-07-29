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

package utils

import (
	"testing"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

// GetPeers calls the endpoint system_peers
func GetPeers(t *testing.T, node *Node) []common.PeerInfo {
	respBody, err := PostRPC("system_peers", NewEndpoint(node.RPCPort), "[]")
	require.NoError(t, err)

	resp := new(modules.SystemPeersResponse)
	err = DecodeRPC(t, respBody, resp)
	require.NoError(t, err)
	require.NotNil(t, resp)

	return resp.Peers
}
