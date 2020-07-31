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
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/runtime/extrinsic"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/tests/utils"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

var (
	numNodes    = 3
	maxRetries  = 32
	testTimeout = time.Minute
	logger      = log.New("pkg", "tests/stress")
)

// compareChainHeads calls getChainHead for each node in the array
// it returns a map of chainHead hashes to node key names, and an error if the hashes don't all match
func compareChainHeads(t *testing.T, nodes []*utils.Node) (map[common.Hash][]string, error) {
	hashes := make(map[common.Hash][]string)
	for _, node := range nodes {
		header := utils.GetChainHead(t, node)
		logger.Info("got header from node", "hash", header.Hash(), "node", node.Key)
		hashes[header.Hash()] = append(hashes[header.Hash()], node.Key)
	}

	var err error
	if len(hashes) != 1 {
		err = errChainHeadMismatch
	}

	return hashes, err
}

// compareChainHeadsWithRetry calls compareChainHeads, retrying up to maxRetries times if it errors.
func compareChainHeadsWithRetry(t *testing.T, nodes []*utils.Node) error {
	var hashes map[common.Hash][]string
	var err error

	for i := 0; i < maxRetries; i++ {
		hashes, err = compareChainHeads(t, nodes)
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}

	if err != nil {
		err = fmt.Errorf("%w: hashes=%v", err, hashes)
	}

	return err
}

// compareBlocksByNumber calls getBlockByNumber for each node in the array
// it returns a map of block hashes to node key names, and an error if the hashes don't all match
func compareBlocksByNumber(t *testing.T, nodes []*utils.Node, num string) (map[common.Hash][]string, error) {
	hashes := make(map[common.Hash][]string)
	errs := []error{}

	var mapMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(nodes))

	for _, node := range nodes {
		go func(node *utils.Node) {
			hash, err := utils.GetBlockHash(t, node, num)
			if err != nil {
				errs = append(errs, err)
				wg.Done()
				return
			}
			logger.Debug("getting hash from node", "hash", hash, "node", node.Key)

			mapMu.Lock()
			hashes[hash] = append(hashes[hash], node.Key)
			mapMu.Unlock()
			wg.Done()
		}(node)
	}
	wg.Wait()

	var err error
	if len(errs) != 0 {
		err = errBlocksAtNumberMismatch
	}

	if len(hashes) == 0 {
		err = errNoBlockAtNumber
	}

	if len(hashes) > 1 {
		err = errBlocksAtNumberMismatch
	}

	return hashes, err
}

// compareBlocksByNumberWithRetry calls compareChainHeads, retrying up to maxRetries times if it errors.
func compareBlocksByNumberWithRetry(t *testing.T, nodes []*utils.Node, num string) error {
	var hashes map[common.Hash][]string
	var err error

	for i := 0; i < maxRetries; i++ {
		hashes, err = compareBlocksByNumber(t, nodes, num)
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}

	if err != nil {
		err = fmt.Errorf("%w: hashes=%v", err, hashes)
	}

	return err
}

// compareFinalizedHeads calls getFinalizedHeadByRound for each node in the array
// it returns a map of finalizedHead hashes to node key names, and an error if the hashes don't all match
func compareFinalizedHeads(t *testing.T, nodes []*utils.Node) (map[common.Hash][]string, error) {
	hashes := make(map[common.Hash][]string)
	for _, node := range nodes {
		hash := utils.GetFinalizedHead(t, node)
		logger.Info("got finalized head from node", "hash", hash, "node", node.Key)
		hashes[hash] = append(hashes[hash], node.Key)
	}

	var err error
	if len(hashes) == 0 {
		err = errNoFinalizedBlock
	}

	if len(hashes) > 1 {
		err = errFinalizedBlockMismatch
	}

	return hashes, err
}

// compareFinalizedHeadsByRound calls getFinalizedHeadByRound for each node in the array
// it returns a map of finalizedHead hashes to node key names, and an error if the hashes don't all match
func compareFinalizedHeadsByRound(t *testing.T, nodes []*utils.Node, round uint64) (map[common.Hash][]string, error) {
	hashes := make(map[common.Hash][]string)
	for _, node := range nodes {
		hash, err := utils.GetFinalizedHeadByRound(t, node, round)
		if err != nil {
			continue
		}

		logger.Info("got finalized head from node", "hash", hash, "node", node.Key, "round", round)
		hashes[hash] = append(hashes[hash], node.Key)
	}

	var err error
	if len(hashes) == 0 {
		err = errNoFinalizedBlock
	}

	if len(hashes) > 1 {
		err = errFinalizedBlockMismatch
	}

	return hashes, err
}

// compareFinalizedHeadsWithRetry calls compareFinalizedHeadsByRound, retrying up to maxRetries times if it errors.
// it returns the finalized hash if it succeeds
func compareFinalizedHeadsWithRetry(t *testing.T, nodes []*utils.Node, round uint64) (common.Hash, error) {
	var hashes map[common.Hash][]string
	var err error

	for i := 0; i < maxRetries; i++ {
		hashes, err = compareFinalizedHeadsByRound(t, nodes, round)
		if err == nil {
			break
		}

		if err == errFinalizedBlockMismatch {
			return common.Hash{}, fmt.Errorf("%w: round=%d hashes=%v", err, round, hashes)
		}

		time.Sleep(3 * time.Second)
	}

	if err != nil {
		return common.Hash{}, fmt.Errorf("%w: round=%d hashes=%v", err, round, hashes)
	}

	for h := range hashes {
		return h, nil
	}

	return common.Hash{}, nil
}

func getPendingExtrinsics(t *testing.T, node *utils.Node) [][]byte { //nolint
	respBody, err := utils.PostRPC(utils.AuthorSubmitExtrinsic, utils.NewEndpoint(node.RPCPort), "[]")
	require.NoError(t, err)

	exts := new(modules.PendingExtrinsicsResponse)
	err = utils.DecodeRPC(t, respBody, exts)
	require.NoError(t, err)

	return *exts
}

// submitExtrinsicAssertInclusion submits an extrinsic to a random node and asserts that the extrinsic was included in some block
// and that the nodes remain synced
func submitExtrinsicAssertInclusion(t *testing.T, nodes []*utils.Node, ext extrinsic.Extrinsic) { //nolint
	tx, err := ext.Encode()
	require.NoError(t, err)

	txStr := hex.EncodeToString(tx)
	logger.Info("submitting transaction", "tx", txStr)

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
	logger.Info("got header from node", "header", header, "hash", header.Hash(), "node", nodes[idx].Key)

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
			logger.Info("got block from node", "hash", header.Hash(), "node", nodes[j].Key)
			logger.Debug("got block from node", "header", header, "body", block.Body, "hash", header.Hash(), "node", nodes[j].Key)

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
