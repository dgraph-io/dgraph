package modules

import (
	"math/big"
	"testing"

	"github.com/ChainSafe/gossamer/dot/core/types"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/lib/utils"
	"github.com/stretchr/testify/require"
)

func newChainService(t *testing.T) *state.Service {
	testDir := utils.NewTestDir(t)
	defer utils.RemoveTestDir(t)
	stateSrvc := state.NewService(testDir)
	genesisHeader, err := types.NewHeader(common.NewHash([]byte{0}), big.NewInt(0), trie.EmptyHash, trie.EmptyHash, [][]byte{})
	if err != nil {
		t.Fatal(err)
	}

	tr := trie.NewEmptyTrie()

	err = stateSrvc.Initialize(genesisHeader, tr)
	if err != nil {
		t.Fatal(err)
	}
	err = stateSrvc.Start()
	if err != nil {
		t.Fatal(err)
	}
	return stateSrvc
}

func TestChainGetHeader_Genesis(t *testing.T) {
	chain := newChainService(t)
	svc := NewChainModule(chain.Block)
	expected := &ChainBlockHeaderResponse{
		ParentHash:     "0x0000000000000000000000000000000000000000000000000000000000000000",
		Number:         big.NewInt(0),
		StateRoot:      "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		ExtrinsicsRoot: "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		Digest:         [][]byte{},
	}
	res := &ChainBlockHeaderResponse{}
	req := ChainHashRequest("0xc375f478c6887dbcc2d1a4dbcc25f330b3df419325ece49cddfe5a0555663b7e")
	err := svc.GetHeader(nil, &req, res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestChainGetHeader_Latest(t *testing.T) {
	chain := newChainService(t)
	svc := NewChainModule(chain.Block)
	expected := &ChainBlockHeaderResponse{
		ParentHash:     "0x0000000000000000000000000000000000000000000000000000000000000000",
		Number:         big.NewInt(0),
		StateRoot:      "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		ExtrinsicsRoot: "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		Digest:         [][]byte{},
	}
	res := &ChainBlockHeaderResponse{}
	req := ChainHashRequest("") // empty request should return latest hash
	err := svc.GetHeader(nil, &req, res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestChainGetHeader_NotFound(t *testing.T) {
	chain := newChainService(t)
	svc := NewChainModule(chain.Block)

	res := &ChainBlockHeaderResponse{}
	req := ChainHashRequest("0xea374832a2c3997280d2772c10e6e5b0b493ccd3d09c0ab14050320e34076c2c")
	err := svc.GetHeader(nil, &req, res)
	require.EqualError(t, err, "Key not found")
}

func TestChainGetHeader_Error(t *testing.T) {
	chain := newChainService(t)
	svc := NewChainModule(chain.Block)

	res := &ChainBlockHeaderResponse{}
	req := ChainHashRequest("zz")
	err := svc.GetHeader(nil, &req, res)
	require.EqualError(t, err, "could not byteify non 0x prefixed string")
}

func TestChainGetBlock_Genesis(t *testing.T) {
	chain := newChainService(t)
	svc := NewChainModule(chain.Block)
	header := &ChainBlockHeaderResponse{
		ParentHash:     "0x0000000000000000000000000000000000000000000000000000000000000000",
		Number:         big.NewInt(0),
		StateRoot:      "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		ExtrinsicsRoot: "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		Digest:         [][]byte{},
	}
	expected := &ChainBlockResponse{
		Block: ChainBlock{
			Header: *header,
			Body:   nil,
		},
	}

	res := &ChainBlockResponse{}
	req := ChainHashRequest("0xc375f478c6887dbcc2d1a4dbcc25f330b3df419325ece49cddfe5a0555663b7e")
	err := svc.GetBlock(nil, &req, res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestChainGetBlock_Latest(t *testing.T) {
	chain := newChainService(t)
	svc := NewChainModule(chain.Block)
	header := &ChainBlockHeaderResponse{
		ParentHash:     "0x0000000000000000000000000000000000000000000000000000000000000000",
		Number:         big.NewInt(0),
		StateRoot:      "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		ExtrinsicsRoot: "0x03170a2e7597b7b7e3d84c05391d139a62b157e78786d8c082f29dcf4c111314",
		Digest:         [][]byte{},
	}
	expected := &ChainBlockResponse{
		Block: ChainBlock{
			Header: *header,
			Body:   nil,
		},
	}

	res := &ChainBlockResponse{}
	req := ChainHashRequest("") // empty request should return latest block
	err := svc.GetBlock(nil, &req, res)
	require.Nil(t, err)

	require.Equal(t, expected, res)
}

func TestChainGetBlock_NoFound(t *testing.T) {
	chain := newChainService(t)
	svc := NewChainModule(chain.Block)

	res := &ChainBlockResponse{}
	req := ChainHashRequest("0xea374832a2c3997280d2772c10e6e5b0b493ccd3d09c0ab14050320e34076c2c")
	err := svc.GetBlock(nil, &req, res)
	require.EqualError(t, err, "Key not found")
}

func TestChainGetBlock_Error(t *testing.T) {
	chain := newChainService(t)
	svc := NewChainModule(chain.Block)

	res := &ChainBlockResponse{}
	req := ChainHashRequest("zz")
	err := svc.GetBlock(nil, &req, res)
	require.EqualError(t, err, "could not byteify non 0x prefixed string")
}
