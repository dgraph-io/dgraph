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

package stress

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/tests/utils"
	"github.com/stretchr/testify/require"

	scribble "github.com/nanobox-io/golang-scribble"
)

var (
	numNodes  = 3
	getHeader = "chain_getHeader"
)

func TestMain(m *testing.M) {
	if utils.GOSSAMER_INTEGRATION_TEST_MODE != "stress" {
		_, _ = fmt.Fprintln(os.Stdout, "Going to skip stress test")
		return
	}

	_, _ = fmt.Fprintln(os.Stdout, "Going to start stress test")

	if utils.NETWORK_SIZE != "" {
		currentNetworkSize, err := strconv.Atoi(utils.NETWORK_SIZE)
		if err == nil {
			_, _ = fmt.Fprintln(os.Stdout, "Going to use custom network size", "currentNetworkSize", currentNetworkSize)
			numNodes = currentNetworkSize
		}
	}

	if utils.HOSTNAME == "" {
		_, _ = fmt.Fprintln(os.Stdout, "HOSTNAME is not set, skipping stress test")
		return
	}

	// Start all tests
	code := m.Run()
	os.Exit(code)
}

func TestStressSync(t *testing.T) {
	t.Log("going to start TestStressSync")
	nodes, err := utils.StartNodes(t, numNodes)
	require.Nil(t, err)

	tempDir, err := ioutil.TempDir("", "gossamer-stress-db")
	require.Nil(t, err)
	t.Log("going to start a JSON database to track all chains", "tempDir", tempDir)

	db, err := scribble.New(tempDir, nil)
	require.Nil(t, err)

	for i, node := range nodes {
		t.Log("going to get HighestBlockHash from node", "i", i, "key", node.Key)

		//Get HighestBlockHash
		respBody, err := utils.PostRPC(t, getHeader, "http://"+utils.HOSTNAME+":"+node.RPCPort, "[]")
		require.Nil(t, err)

		// decode resp
		chainBlockResponse := new(modules.ChainBlockHeaderResponse)
		utils.DecodeRPC(t, respBody, chainBlockResponse)

		err = db.Write("blocks_"+node.Key, chainBlockResponse.Number, chainBlockResponse)
		require.Nil(t, err)

	}

	//TODO: #803 cleanup optimization
	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}
