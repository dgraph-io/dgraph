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

package rpc

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"

	"github.com/ChainSafe/gossamer/tests/utils"
	"github.com/stretchr/testify/require"
)

func TestChainRPC(t *testing.T) {
	if utils.MODE != rpcSuite {
		_, _ = fmt.Fprintln(os.Stdout, "Going to skip RPC suite tests")
		return
	}

	testCases := []*testCase{
		{
			description: "test chain_getHeader",
			method:      "chain_getHeader",
			expected: modules.ChainBlockHeaderResponse{
				Number: "1",
			},
			params: "[]",
		},
		{
			description: "test chain_getBlock",
			method:      "chain_getBlock",
			expected: modules.ChainBlockResponse{
				Block: modules.ChainBlock{
					Header: modules.ChainBlockHeaderResponse{
						Number: "1",
					},
					Body: []string{},
				},
			},
			params: "[]",
		},
		{
			description: "test chain_getBlockHash",
			method:      "chain_getBlockHash",
			expected:    "",
			params:      "[]",
		},
		{ //TODO
			description: "test chain_getFinalizedHead",
			method:      "chain_getFinalizedHead",
			skip:        true,
		},
	}

	t.Log("starting gossamer...")
	nodes, err := utils.InitializeAndStartNodes(t, 1, utils.GenesisDefault, utils.ConfigBABEMaxThreshold)
	require.Nil(t, err)

	time.Sleep(time.Second) // give server a second to start

	chainBlockHeaderHash := ""
	for _, test := range testCases {

		t.Run(test.description, func(t *testing.T) {

			// set params for chain_getBlock from previous chain_getHeader call
			if chainBlockHeaderHash != "" {
				test.params = "[\"" + chainBlockHeaderHash + "\"]"
			}

			target := getResponse(t, test)

			switch v := target.(type) {
			case *modules.ChainBlockHeaderResponse:
				t.Log("Will assert ChainBlockHeaderResponse", "value", v)

				require.GreaterOrEqual(t, test.expected.(modules.ChainBlockHeaderResponse).Number, v.Number)

				require.NotNil(t, test.expected.(modules.ChainBlockHeaderResponse).ParentHash)
				require.NotNil(t, test.expected.(modules.ChainBlockHeaderResponse).StateRoot)
				require.NotNil(t, test.expected.(modules.ChainBlockHeaderResponse).ExtrinsicsRoot)
				require.NotNil(t, test.expected.(modules.ChainBlockHeaderResponse).Digest)

				//save for chain_getBlock
				chainBlockHeaderHash = v.ParentHash
			case *modules.ChainBlockResponse:
				t.Log("Will assert ChainBlockResponse", "value", v.Block)

				//reset
				chainBlockHeaderHash = ""

				require.NotNil(t, test.expected.(modules.ChainBlockResponse).Block)

				require.GreaterOrEqual(t, test.expected.(modules.ChainBlockResponse).Block.Header.Number, v.Block.Header.Number)

				require.NotNil(t, test.expected.(modules.ChainBlockResponse).Block.Header.ParentHash)
				require.NotNil(t, test.expected.(modules.ChainBlockResponse).Block.Header.StateRoot)
				require.NotNil(t, test.expected.(modules.ChainBlockResponse).Block.Header.ExtrinsicsRoot)
				require.NotNil(t, test.expected.(modules.ChainBlockResponse).Block.Header.Digest)

				require.NotNil(t, test.expected.(modules.ChainBlockResponse).Block.Body)
				require.GreaterOrEqual(t, len(test.expected.(modules.ChainBlockResponse).Block.Body), 0)

			case *string:
				t.Log("Will assert ChainBlockNumberRequest", "value", *v)
				require.NotNil(t, v)
				require.GreaterOrEqual(t, len(*v), 66)

			}

		})
	}

	t.Log("going to tear down gossamer...")
	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}
