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

package core

import (
	"math/big"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/network"
	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/runtime"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/lib/utils"
	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

// testMessageTimeout is the wait time for messages to be exchanged
var testMessageTimeout = time.Second

// testGenesisHeader is a test block header
var testGenesisHeader = &types.Header{
	Number:    big.NewInt(0),
	StateRoot: trie.EmptyHash,
}

var firstEpochInfo = &types.EpochInfo{
	Duration:   200,
	FirstBlock: 0,
}

type mockVerifier struct{}

func (v *mockVerifier) SetRuntimeChangeAtBlock(header *types.Header, rt *runtime.Runtime) error {
	return nil
}

func (v *mockVerifier) SetAuthorityChangeAtBlock(header *types.Header, auths []*types.Authority) {

}

// mockBlockProducer implements the BlockProducer interface
type mockBlockProducer struct {
	auths []*types.Authority
}

// Start mocks starting
func (bp *mockBlockProducer) Start() error {
	return nil
}

// Stop mocks stopping
func (bp *mockBlockProducer) Stop() error {
	return nil
}

func (bp *mockBlockProducer) Authorities() []*types.Authority {
	return bp.auths
}

func (bp *mockBlockProducer) SetAuthorities(a []*types.Authority) error {
	bp.auths = a
	return nil
}

// GetBlockChannel returns a new channel
func (bp *mockBlockProducer) GetBlockChannel() <-chan types.Block {
	return make(chan types.Block)
}

// SetRuntime mocks setting runtime
func (bp *mockBlockProducer) SetRuntime(rt *runtime.Runtime) error {
	return nil
}

type mockNetwork struct {
	Message network.Message
}

func (n *mockNetwork) SendMessage(m network.Message) {
	n.Message = m
}

// mockFinalityGadget implements the FinalityGadget interface
type mockFinalityGadget struct {
	in        chan FinalityMessage
	out       chan FinalityMessage
	finalized chan FinalityMessage
	auths     []*types.Authority
}

// Start mocks starting
func (fg *mockFinalityGadget) Start() error {
	return nil
}

// Stop mocks stopping
func (fg *mockFinalityGadget) Stop() error {
	return nil
}

// GetVoteOutChannel returns the out channel
func (fg *mockFinalityGadget) GetVoteOutChannel() <-chan FinalityMessage {
	return fg.out
}

// GetVoteInChannel returns the in channel
func (fg *mockFinalityGadget) GetVoteInChannel() chan<- FinalityMessage {
	return fg.in
}

// GetFinalizedChannel returns the finalized channel
func (fg *mockFinalityGadget) GetFinalizedChannel() <-chan FinalityMessage {
	return fg.finalized
}

// DecodeMessage returns a mockFinalityMessage
func (fg *mockFinalityGadget) DecodeMessage(*network.ConsensusMessage) (FinalityMessage, error) {
	return &mockFinalityMessage{}, nil
}

func (fg *mockFinalityGadget) UpdateAuthorities(ad []*types.Authority) {
	fg.auths = ad
}

func (fg *mockFinalityGadget) Authorities() []*types.Authority {
	return fg.auths
}

var testConsensusMessage = &network.ConsensusMessage{
	ConsensusEngineID: types.GrandpaEngineID,
	Data:              []byte("nootwashere"),
}

type mockFinalityMessage struct{}

// ToConsensusMessage returns a testConsensusMessage
func (fm *mockFinalityMessage) ToConsensusMessage() (*network.ConsensusMessage, error) {
	return testConsensusMessage, nil
}

func (fm *mockFinalityMessage) Type() byte {
	return 0
}

type mockConsensusMessageHandler struct{}

func (h *mockConsensusMessageHandler) HandleMessage(msg *network.ConsensusMessage) (*network.ConsensusMessage, error) {
	return nil, nil
}

// NewTestService creates a new test core service
func NewTestService(t *testing.T, cfg *Config) *Service {
	if cfg == nil {
		cfg = &Config{
			IsBlockProducer: false,
		}
	}

	if cfg.Runtime == nil {
		cfg.Runtime = runtime.NewTestRuntime(t, runtime.NODE_RUNTIME)
	}

	if cfg.Keystore == nil {
		cfg.Keystore = keystore.NewGlobalKeystore()
		kp, err := sr25519.GenerateKeypair()
		if err != nil {
			t.Fatal(err)
		}
		cfg.Keystore.Acco.Insert(kp)
	}

	if cfg.NewBlocks == nil {
		cfg.NewBlocks = make(chan types.Block)
	}

	if cfg.Verifier == nil {
		cfg.Verifier = new(mockVerifier)
	}

	cfg.LogLvl = 3

	stateSrvc := state.NewService("", log.LvlInfo)
	stateSrvc.UseMemDB()

	genesisData := new(genesis.Data)

	tt := trie.NewEmptyTrie()
	err := stateSrvc.Initialize(genesisData, testGenesisHeader, tt, firstEpochInfo)
	require.Nil(t, err)

	err = stateSrvc.Start()
	require.Nil(t, err)

	if cfg.BlockState == nil {
		cfg.BlockState = stateSrvc.Block
	}

	if cfg.StorageState == nil {
		cfg.StorageState = stateSrvc.Storage
	}

	if cfg.ConsensusMessageHandler == nil {
		cfg.ConsensusMessageHandler = &mockConsensusMessageHandler{}
	}

	if cfg.Network == nil {
		basePath := utils.NewTestBasePath(t, "node")

		// removes all data directories created within test directory
		defer utils.RemoveTestDir(t)

		config := &network.Config{
			BasePath:    basePath,
			Port:        7001,
			RandSeed:    1,
			NoBootstrap: true,
			NoMDNS:      true,
			BlockState:  stateSrvc.Block,
		}
		cfg.Network = createTestNetworkService(t, config)
		require.NoError(t, err)
	}

	s, err := NewService(cfg)
	require.Nil(t, err)

	return s
}

// helper method to create and start a new network service
func createTestNetworkService(t *testing.T, cfg *network.Config) (srvc *network.Service) {
	if cfg.NetworkState == nil {
		cfg.NetworkState = &network.MockNetworkState{}
	}

	if cfg.LogLvl == 0 {
		cfg.LogLvl = 3
	}

	if cfg.Syncer == nil {
		cfg.Syncer = newMockSyncer()
	}

	srvc, err := network.NewService(cfg)
	require.NoError(t, err)

	err = srvc.Start()
	require.NoError(t, err)

	t.Cleanup(func() {
		utils.RemoveTestDir(t)
		err := srvc.Stop()
		require.NoError(t, err)
	})
	return srvc
}

type mockSyncer struct {
	highestSeen *big.Int
}

func newMockSyncer() *mockSyncer {
	return &mockSyncer{
		highestSeen: big.NewInt(0),
	}
}
func (s *mockSyncer) CreateBlockResponse(msg *network.BlockRequestMessage) (*network.BlockResponseMessage, error) {
	return nil, nil
}

func (s *mockSyncer) HandleBlockAnnounce(msg *network.BlockAnnounceMessage) *network.BlockRequestMessage {
	if msg.Number.Cmp(s.highestSeen) > 0 {
		s.highestSeen = msg.Number
	}

	return &network.BlockRequestMessage{
		ID: 99,
	}
}

func (s *mockSyncer) HandleBlockResponse(msg *network.BlockResponseMessage) *network.BlockRequestMessage {
	return nil
}

func (s *mockSyncer) HandleSeenBlocks(num *big.Int) *network.BlockRequestMessage {
	if num.Cmp(s.highestSeen) > 0 {
		s.highestSeen = num
	}
	return nil
}
