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
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthorRPC(t *testing.T) {
	if GOSSAMER_INTEGRATION_TEST_MODE != rpcSuite {
		_, _ = fmt.Fprintln(os.Stdout, "Going to skip RPC suite tests")
		return
	}
	testsCases := []struct {
		description string
		method      string
		expected    interface{}
		skip        bool
	}{
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

	t.Log("going to start gossamer")

	localPidList, err := StartNodes(t, make([]*exec.Cmd, 1))
	require.Nil(t, err)

	time.Sleep(time.Second) // give server a second to start

	for _, test := range testsCases {
		t.Run(test.description, func(t *testing.T) {
			if test.skip {
				t.Skip("RPC endpoint not yet implemented")
				return
			}

			respBody, err := PostRPC(t, test.method, "http://"+GOSSAMER_NODE_HOST+":"+currentPort, "{}")
			require.Nil(t, err)

			target := reflect.New(reflect.TypeOf(test.expected)).Interface()
			DecodeRPC(t, respBody, target)

			require.NotNil(t, target)

		})
	}

	t.Log("going to TearDown Gossamer node")

	errList := TearDown(t, localPidList)
	require.Len(t, errList, 0)
}
