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
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/rpc/modules"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/runtime/extrinsic"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/tests/utils"

	log "github.com/ChainSafe/log15"
	scribble "github.com/nanobox-io/golang-scribble"
	"github.com/stretchr/testify/require"
)

var (
	numNodes               = 3
	maxRetries             = 8
	chain_getBlock         = "chain_getBlock"
	chain_getHeader        = "chain_getHeader"
	author_submitExtrinsic = "author_submitExtrinsic"
)

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

// TODO: move to utils, use in RPC tests
func endpoint(node *utils.Node) string {
	return "http://" + utils.HOSTNAME + ":" + node.RPCPort
}

// getBlock calls the endpoint chain_getBlock
func getBlock(t *testing.T, node *utils.Node, hash common.Hash) *types.Block {
	respBody, err := utils.PostRPC(t, chain_getBlock, endpoint(node), "[\""+hash.String()+"\"]")
	require.NoError(t, err)

	block := new(modules.ChainBlockResponse)
	err = utils.DecodeRPC(t, respBody, block)
	if err != nil {
		return nil
	}

	header := block.Block.Header

	parentHash, err := common.HexToHash(header.ParentHash)
	require.NoError(t, err)

	nb, err := common.HexToBytes(header.Number)
	require.NoError(t, err)
	number := big.NewInt(0).SetBytes(nb)

	stateRoot, err := common.HexToHash(header.StateRoot)
	require.NoError(t, err)

	extrinsicsRoot, err := common.HexToHash(header.ExtrinsicsRoot)
	require.NoError(t, err)

	digest := [][]byte{}

	for _, l := range header.Digest.Logs {
		var d []byte
		d, err = common.HexToBytes(l)
		require.NoError(t, err)
		digest = append(digest, d)
	}

	h, err := types.NewHeader(parentHash, number, stateRoot, extrinsicsRoot, digest)
	require.NoError(t, err)

	b, err := types.NewBodyFromExtrinsicStrings(block.Block.Body)
	require.NoError(t, err, fmt.Sprintf("%v", block.Block.Body))

	return &types.Block{
		Header: h,
		Body:   b,
	}
}

// headerResponseToHeader converts a *ChainBlockHeaderResponse to a *types.Header
func headerResponseToHeader(t *testing.T, header *modules.ChainBlockHeaderResponse) *types.Header {
	parentHash, err := common.HexToHash(header.ParentHash)
	require.NoError(t, err)

	nb, err := common.HexToBytes(header.Number)
	require.NoError(t, err)
	number := big.NewInt(0).SetBytes(nb)

	stateRoot, err := common.HexToHash(header.StateRoot)
	require.NoError(t, err)

	extrinsicsRoot, err := common.HexToHash(header.ExtrinsicsRoot)
	require.NoError(t, err)

	digest := [][]byte{}

	for _, l := range header.Digest.Logs {
		var d []byte
		d, err = common.HexToBytes(l)
		require.NoError(t, err)
		digest = append(digest, d)
	}

	h, err := types.NewHeader(parentHash, number, stateRoot, extrinsicsRoot, digest)
	require.NoError(t, err)
	return h
}

// getHeader calls the endpoint chain_getHeader
func getHeader(t *testing.T, node *utils.Node, hash common.Hash) *types.Header { //nolint
	respBody, err := utils.PostRPC(t, chain_getHeader, endpoint(node), "[\""+hash.String()+"\"]")
	require.NoError(t, err)

	header := new(modules.ChainBlockHeaderResponse)
	utils.DecodeRPC(t, respBody, header)
	return headerResponseToHeader(t, header)
}

// getChainHead calls the endpoint chain_getHeader to get the latest chain head
func getChainHead(t *testing.T, node *utils.Node) *types.Header {
	respBody, err := utils.PostRPC(t, chain_getHeader, endpoint(node), "[]")
	require.NoError(t, err)

	header := new(modules.ChainBlockHeaderResponse)
	utils.DecodeRPC(t, respBody, header)
	return headerResponseToHeader(t, header)
}

// compareChainHeads calls getChainHead for each node in the array
// it returns a map of chainHead hashes to node key names, and an error if the hashes don't all match
func compareChainHeads(t *testing.T, nodes []*utils.Node) (map[common.Hash][]string, error) {
	hashes := make(map[common.Hash][]string)
	for _, node := range nodes {
		header := getChainHead(t, node)
		log.Info("getting header from node", "header", header, "hash", header.Hash(), "node", node.Key)
		hashes[header.Hash()] = append(hashes[header.Hash()], node.Key)
	}

	var err error
	if len(hashes) != 1 {
		err = errors.New("node chain head hashes don't match")
	}

	return hashes, err
}

func TestStressSync(t *testing.T) {
	t.Log("going to start TestStressSync")
	nodes, err := utils.StartNodes(t, numNodes)
	require.NoError(t, err)

	tempDir, err := ioutil.TempDir("", "gossamer-stress-db")
	require.NoError(t, err)
	t.Log("going to start a JSON database to track all chains", "tempDir", tempDir)

	db, err := scribble.New(tempDir, nil)
	require.NoError(t, err)

	for _, node := range nodes {
		header := getChainHead(t, node)

		err = db.Write("blocks_"+node.Key, header.Number.String(), header)
		require.NoError(t, err)
	}

	//TODO: #803 cleanup optimization
	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}

func TestStress_IncludeData(t *testing.T) {
	nodes, err := utils.StartNodes(t, numNodes)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	// create IncludeData extrnsic
	ext := extrinsic.NewIncludeDataExt([]byte("nootwashere"))
	tx, err := ext.Encode()
	require.NoError(t, err)

	txStr := hex.EncodeToString(tx)
	log.Info("submitting transaction", "tx", txStr)

	// send extrinsic to random node
	idx := rand.Intn(len(nodes))
	prevHeader := getChainHead(t, nodes[idx]) // get starting header so that we can lookup blocks by number later
	respBody, err := utils.PostRPC(t, author_submitExtrinsic, endpoint(nodes[idx]), "\"0x"+txStr+"\"")
	require.NoError(t, err)

	var hash modules.ExtrinsicHashResponse
	utils.DecodeRPC(t, respBody, &hash)
	log.Info("submitted transaction", "hash", hash)

	// wait for nodes to build block + sync, then get headers
	time.Sleep(time.Second * 5)
	var hashes map[common.Hash][]string
	for i := 0; i < maxRetries; i++ {
		hashes, err = compareChainHeads(t, nodes)
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}
	require.NoError(t, err, hashes)

	header := getChainHead(t, nodes[idx])
	log.Info("got header from node", "header", header, "hash", header.Hash(), "node", nodes[idx].Key)

	// search from child -> parent blocks for extrinsic
	time.Sleep(time.Second * 5)
	var resExts []types.Extrinsic
	i := 0
	for header.ExtrinsicsRoot == trie.EmptyHash && i != maxRetries {
		block := getBlock(t, nodes[idx], header.ParentHash)
		if block == nil {
			// couldn't get block, increment retry counter
			i++
			continue
		}

		header = block.Header
		log.Info("got header from node", "header", header, "hash", header.Hash(), "node", nodes[idx].Key)

		if block.Body != nil && !bytes.Equal(*(block.Body), []byte{0}) {
			resExts, err = block.Body.AsExtrinsics()
			require.NoError(t, err, block.Body)
			break
		}

		if header.Hash() == prevHeader.Hash() {
			t.Fatal("could not find extrinsic in any blocks")
		}
	}

	// assert that the extrinsic included is the one we submitted
	require.Equal(t, resExts[0], types.Extrinsic(tx))

	// repeat sync check for sanity
	time.Sleep(time.Second * 5)
	for i = 0; i < maxRetries; i++ {
		hashes, err = compareChainHeads(t, nodes)
		if err == nil {
			break
		}

		time.Sleep(time.Second)
	}
	require.NoError(t, err, hashes)

	//TODO: #803 cleanup optimization
	errList := utils.TearDown(t, nodes)
	require.Len(t, errList, 0)
}
