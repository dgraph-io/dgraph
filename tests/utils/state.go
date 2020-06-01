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

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/stretchr/testify/require"
)

// GetStorage calls the endpoint state_getStorage
func GetStorage(t *testing.T, node *Node, key []byte) []byte {
	respBody, err := PostRPC(t, StateGetStorage, NewEndpoint(node.RPCPort), "[\""+common.BytesToHex(key)+"\"]")
	require.NoError(t, err)

	v := new(string)
	err = DecodeRPC(t, respBody, v)
	require.NoError(t, err)
	if *v == "" {
		return []byte{}
	}

	value, err := common.HexToBytes(*v)
	require.NoError(t, err)

	return value
}
