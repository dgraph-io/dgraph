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
	"os/exec"
	"strconv"
	"testing"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/tests/rpc"
	"github.com/stretchr/testify/require"

	scribble "github.com/nanobox-io/golang-scribble"
)

var (
	pidList = make([]*exec.Cmd, 3)
)

func TestMain(m *testing.M) {
	if rpc.GOSSAMER_INTEGRATION_TEST_MODE != "stress" {
		_, _ = fmt.Fprintln(os.Stdout, "Going to skip stress test")
		return
	}

	_, _ = fmt.Fprintln(os.Stdout, "Going to start stress test")

	if rpc.NETWORK_SIZE_STR != "" {
		currentNetworkSize, err := strconv.Atoi(rpc.NETWORK_SIZE_STR)
		if err == nil {
			_, _ = fmt.Fprintln(os.Stdout, "Going to custom network size ... ", "currentNetworkSize", currentNetworkSize)
			pidList = make([]*exec.Cmd, currentNetworkSize)
		}
	}

	if rpc.GOSSAMER_NODE_HOST == "" {
		_, _ = fmt.Fprintln(os.Stdout, "GOSSAMER_NODE_HOST is not set, Going to skip stress test")
		return
	}

	// Start all tests
	code := m.Run()
	os.Exit(code)
}

func TestStressSync(t *testing.T) {
	t.Log("going to start TestStressSync")
	localPidList, err := rpc.StartNodes(t, pidList)
	require.Nil(t, err)

	tempDir, err := ioutil.TempDir("", "gossamer-stress-db")
	require.Nil(t, err)
	t.Log("going to start a JSON simple database to track all chains", "tempDir", tempDir)

	db, err := scribble.New(tempDir, nil)
	require.Nil(t, err)

	blockHighestBlockHash := "chain_getHeader"

	for i, v := range localPidList {

		t.Log("going to get HighestBlockHash from node", "i", i, "v", v)

		//Get HighestBlockHash
		respBody, err := rpc.PostRPC(t, blockHighestBlockHash, "http://"+rpc.GOSSAMER_NODE_HOST+":854"+strconv.Itoa(i), "[]")
		require.Nil(t, err)

		// decode resp
		chainBlockResponse := new(modules.ChainBlockHeaderResponse)
		rpc.DecodeRPC(t, respBody, chainBlockResponse)

		//TODO: #802 use the name of the authority here, this requires a map implementation (map process/pid/authority)
		err = db.Write("blocks_"+strconv.Itoa(v.Process.Pid),
			chainBlockResponse.Number, chainBlockResponse)
		require.Nil(t, err)

	}

	//// Read a block header from the database (passing a hash by reference)
	//if err := db.Read("blocks_"+strconv.Itoa(v.Process.Pid), chainBlockResponse.Number.String(), &blockHeader); err != nil {
	//	fmt.Println("Error", err)
	//}

	//TODO: further implement test
	// iterate over db
	// see if the same or not
	// kill some nodes, start others, make sure things still move forward

	//TODO: #803 cleanup optimization
	errList := rpc.TearDown(t, localPidList)
	require.Len(t, errList, 0)
}
