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
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/tests/utils"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

func TestNetwork_MaxPeers(t *testing.T) {
	numNodes = 9 // 9 block producers
	utils.SetLogLevel(log.LvlInfo)
	nodes, err := utils.InitializeAndStartNodes(t, numNodes, utils.GenesisDefault, utils.ConfigDefault)
	require.NoError(t, err)

	defer func() {
		errList := utils.TearDown(t, nodes)
		require.Len(t, errList, 0)
	}()

	// wait for nodes to connect
	time.Sleep(time.Second * 10)

	for i, node := range nodes {
		peers := utils.GetPeers(t, node)
		t.Logf("node %d: peer count=%d", i, len(peers))
		require.LessOrEqual(t, len(peers), 5)
	}
}
