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
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/runtime/extrinsic"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/tests/utils"

	log "github.com/ChainSafe/log15"
	scribble "github.com/nanobox-io/golang-scribble"
	"github.com/stretchr/testify/require"
)

var (
	numNodes   = 3
	maxRetries = 8
)

// compareChainHeads calls getChainHead for each node in the array
// it returns a map of chainHead hashes to node key names, and an error if the hashes don't all match
func compareChainHeads(t *testing.T, nodes []*utils.Node) (map[common.Hash][]string, error) {
	hashes := make(map[common.Hash][]string)
	for _, node := range nodes {
		header := utils.GetChainHead(t, node)
		log.Info("getting header from node", "header", header, "hash", header.Hash(), "node", node.Key)
		hashes[header.Hash()] = append(hashes[header.Hash()], node.Key)
	}

	var err error
	if len(hashes) != 1 {
		err = errors.New("node chain head hashes don't match")
	}

	return hashes, err
}

// compareFinalizedHeads calls getFinalizedHead for each node in the array
// it returns a map of finalizedHead hashes to node key names, and an error if the hashes don't all match
func compareFinalizedHeads(t *testing.T, nodes []*utils.Node) (map[common.Hash][]string, error) {
	hashes := make(map[common.Hash][]string)
	for _, node := range nodes {
		hash := utils.GetFinalizedHead(t, node)
		log.Info("got finalized head from node", "hash", hash, "node", node.Key)
		hashes[hash] = append(hashes[hash], node.Key)
	}

	var err error
	if len(hashes) != 1 {
		err = errors.New("node finalized head hashes don't match")
	}

	return hashes, err
}

// compareChainHeadsWithRetry calls compareChainHeads, retrying up to maxRetries times if it errors.
func compareChainHeadsWithRetry(t *testing.T, nodes []*utils.Node) {
	var hashes map[common.Hash][]string
	var err error

	for i := 0; i < maxRetries; i++ {
		hashes, err = compareChainHeads(t, nodes)
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}
	require.NoError(t, err, hashes)
}

// compareFinalizedHeadsWithRetry calls compareFinalizedHeads, retrying up to maxRetries times if it errors.
// it returns the finalized hash if it succeeds
func compareFinalizedHeadsWithRetry(t *testing.T, nodes []*utils.Node) common.Hash {
	var hashes map[common.Hash][]string
	var err error

	for i := 0; i < maxRetries; i++ {
		hashes, err = compareFinalizedHeads(t, nodes)
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}
	require.NoError(t, err, hashes)

	for h := range hashes {
		return h
	}

	return common.Hash{}
}

func TestMain(m *testing.M) {
	if utils.GOSSAMER_INTEGRATION_TEST_MODE != "stress" {
		_, _ = fmt.Fprintln(os.Stdout, "Going to skip stress test")
		return
	}

	_, _ = fmt.Fprintln(os.Stdout, "Going to start stress test")

	if utils.NETWORK_SIZE != "" {
		var err error
		numNodes, err = strconv.Atoi(utils.NETWORK_SIZE)
		if err == nil {
			_, _ = fmt.Fprintf(os.Stdout, "Going to use custom network size %d\n", numNodes)
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

func getPendingExtrinsics(t *testing.T, node *utils.Node) [][]byte {
	respBody, err := utils.PostRPC(utils.AuthorSubmitExtrinsic, utils.NewEndpoint(node.RPCPort), "[]")
	require.NoError(t, err)

	exts := new(modules.PendingExtrinsicsResponse)
	err = utils.DecodeRPC(t, respBody, exts)
	require.NoError(t, err)

	return *exts
}

func TestStressSync(t *testing.T) {
	nodes, err := utils.InitializeAndStartNodes(t, numNodes, utils.GenesisDefault)
	require.NoError(t, err)

	tempDir, err := ioutil.TempDir("", "gossamer-stress-db")
	require.NoError(t, err)

	db, err := scribble.New(tempDir, nil)
	require.NoError(t, err)

	for _, node := range nodes {
		header := utils.GetChainHead(t, node)

		err = db.Write("blocks_"+node.Key, header.Number.String(), header)
		require.NoError(t, err)
	}

	compareChainHeadsWithRetry(t, nodes)

	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}

func TestRestartNode(t *testing.T) {
	nodes, err := utils.InitNodes(numNodes)
	require.NoError(t, err)

	err = utils.StartNodes(t, nodes)
	require.NoError(t, err)

	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)

	err = utils.StartNodes(t, nodes)
	require.NoError(t, err)

	errList = utils.TearDown(t, nodes)
	require.Len(t, errList, 0)

}

// submitExtrinsicAssertInclusion submits an extrinsic to a random node and asserts that the extrinsic was included in some block
// and that the nodes remain synced
func submitExtrinsicAssertInclusion(t *testing.T, nodes []*utils.Node, ext extrinsic.Extrinsic) {
	tx, err := ext.Encode()
	require.NoError(t, err)

	txStr := hex.EncodeToString(tx)
	log.Info("submitting transaction", "tx", txStr)

	// send extrinsic to random node
	idx := rand.Intn(len(nodes))
	prevHeader := utils.GetChainHead(t, nodes[idx]) // get starting header so that we can lookup blocks by number later
	respBody, err := utils.PostRPC(utils.AuthorSubmitExtrinsic, utils.NewEndpoint(nodes[idx].RPCPort), "\"0x"+txStr+"\"")
	require.NoError(t, err)

	var hash modules.ExtrinsicHashResponse
	err = utils.DecodeRPC(t, respBody, &hash)
	require.Nil(t, err)
	log.Info("submitted transaction", "hash", hash, "node", nodes[idx].Key)
	t.Logf("submitted transaction to node %s", nodes[idx].Key)

	// wait for nodes to build block + sync, then get headers
	time.Sleep(time.Second * 10)

	for i := 0; i < maxRetries; i++ {
		exts := getPendingExtrinsics(t, nodes[idx])
		if len(exts) == 0 {
			break
		}

		time.Sleep(time.Second)
	}

	header := utils.GetChainHead(t, nodes[idx])
	log.Info("got header from node", "header", header, "hash", header.Hash(), "node", nodes[idx].Key)

	// search from child -> parent blocks for extrinsic
	var resExts []types.Extrinsic
	i := 0
	for header.ExtrinsicsRoot == trie.EmptyHash && i != maxRetries {
		// check all nodes, since it might have been included on any of the block producers
		var block *types.Block

		for j := 0; j < len(nodes); j++ {
			block = utils.GetBlock(t, nodes[j], header.ParentHash)
			if block == nil {
				// couldn't get block, increment retry counter
				i++
				continue
			}

			header = block.Header
			log.Info("got block from node", "hash", header.Hash(), "node", nodes[j].Key)
			log.Debug("got block from node", "header", header, "body", block.Body, "hash", header.Hash(), "node", nodes[j].Key)

			if block.Body != nil && !bytes.Equal(*(block.Body), []byte{0}) {
				resExts, err = block.Body.AsExtrinsics()
				require.NoError(t, err, block.Body)
				break
			}

			if header.Hash() == prevHeader.Hash() && j == len(nodes)-1 {
				t.Fatal("could not find extrinsic in any blocks")
			}
		}

		if block != nil && block.Body != nil && !bytes.Equal(*(block.Body), []byte{0}) {
			break
		}
	}

	// assert that the extrinsic included is the one we submitted
	require.Equal(t, 1, len(resExts), "did not find extrinsic in block on any node")
	require.Equal(t, resExts[0], types.Extrinsic(tx))
}

func TestStress_IncludeData(t *testing.T) {
	t.Skip()

	nodes, err := utils.InitializeAndStartNodes(t, numNodes, utils.GenesisDefault)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	// create IncludeData extrnsic
	ext := extrinsic.NewIncludeDataExt([]byte("nootwashere"))
	submitExtrinsicAssertInclusion(t, nodes, ext)

	compareChainHeadsWithRetry(t, nodes)

	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}

func TestStress_StorageChange(t *testing.T) {
	t.Skip()

	nodes, err := utils.InitializeAndStartNodes(t, numNodes, utils.GenesisDefault)
	require.NoError(t, err)

	defer func() {
		//TODO: #803 cleanup optimization
		errList := utils.TearDown(t, nodes)
		require.Len(t, errList, 0)
	}()

	time.Sleep(5 * time.Second)

	// create IncludeData extrnsic
	key := []byte("noot")
	value := []byte("washere")
	ext := extrinsic.NewStorageChangeExt(key, optional.NewBytes(true, value))
	submitExtrinsicAssertInclusion(t, nodes, ext)

	time.Sleep(10 * time.Second)

	// for each node, check that storage was updated accordingly
	errs := []error{}
	for _, node := range nodes {
		log.Info("getting storage from node", "node", node.Key)
		res := utils.GetStorage(t, node, key)

		// TODO: why does finalize_block modify the storage value?
		if bytes.Equal(res, []byte{}) {
			t.Logf("could not get storage value from node %s", node.Key)
			errs = append(errs, fmt.Errorf("could not get storage value from node %s\n", node.Key)) //nolint
		} else {
			t.Logf("got storage value from node %s: %v", node.Key, res)
		}
	}

	require.Equal(t, 0, len(errs), errs)
	compareChainHeadsWithRetry(t, nodes)
}

func TestStress_Grandpa_OneAuthority(t *testing.T) {
	numNodes = 1
	nodes, err := utils.InitializeAndStartNodes(t, numNodes, utils.GenesisOneAuth)
	require.NoError(t, err)

	time.Sleep(time.Second * 10)

	compareChainHeadsWithRetry(t, nodes)
	prev := compareFinalizedHeadsWithRetry(t, nodes)

	time.Sleep(time.Second * 10)
	curr := compareFinalizedHeadsWithRetry(t, nodes)
	require.NotEqual(t, prev, curr)

	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}

func TestStress_Grandpa_ThreeAuthorities(t *testing.T) {
	t.Skip() // this is blocked by #923
	numNodes = 3
	nodes, err := utils.InitializeAndStartNodes(t, numNodes, utils.GenesisThreeAuths)
	require.NoError(t, err)

	time.Sleep(time.Second * 10)

	compareChainHeadsWithRetry(t, nodes)
	prev := compareFinalizedHeadsWithRetry(t, nodes)

	time.Sleep(time.Second * 20)
	curr := compareFinalizedHeadsWithRetry(t, nodes)
	require.NotEqual(t, prev, curr)

	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}
