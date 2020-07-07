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

	"github.com/ChainSafe/gossamer/tests/utils"
	"github.com/stretchr/testify/require"
)

func TestAuthorRPC(t *testing.T) {
	if utils.MODE != rpcSuite {
		_, _ = fmt.Fprintln(os.Stdout, "Going to skip RPC suite tests")
		return
	}

	testCases := []*testCase{
		{ //TODO
			description: "test author_submitExtrinsic",
			method:      "author_submitExtrinsic",
			skip:        true,
		},
		{ //TODO
			description: "test author_pendingExtrinsics",
			method:      "author_pendingExtrinsics",
			skip:        true,
		},
		{ //TODO
			description: "test author_removeExtrinsic",
			method:      "author_removeExtrinsic",
			skip:        true,
		},
		{ //TODO
			description: "test author_insertKey",
			method:      "author_insertKey",
			skip:        true,
		},
		{ //TODO
			description: "test author_rotateKeys",
			method:      "author_rotateKeys",
			skip:        true,
		},
		{ //TODO
			description: "test author_hasSessionKeys",
			method:      "author_hasSessionKeys",
			skip:        true,
		},
		{ //TODO
			description: "test author_hasKey",
			method:      "author_hasKey",
			skip:        true,
		},
	}

	t.Log("starting gossamer...")
	nodes, err := utils.InitializeAndStartNodes(t, 1, utils.GenesisDefault, utils.ConfigDefault)
	require.Nil(t, err)

	time.Sleep(time.Second) // give server a second to start

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			_ = getResponse(t, test)
		})
	}

	t.Log("going to tear down gossamer...")
	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}
