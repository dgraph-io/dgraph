// Copyright 2019 ChainSafe Systems (ON) Corp.
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
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/genesis"
	"github.com/ChainSafe/gossamer/lib/keystore"
	"github.com/ChainSafe/gossamer/lib/trie"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

func newTestDigestHandler(t *testing.T, withBABE, withGrandpa bool) *DigestHandler { //nolint
	stateSrvc := state.NewService("", log.LvlInfo)
	stateSrvc.UseMemDB()

	genesisData := new(genesis.Data)

	err := stateSrvc.Initialize(genesisData, testGenesisHeader, trie.NewEmptyTrie(), firstEpochInfo)
	require.NoError(t, err)

	err = stateSrvc.Start()
	require.NoError(t, err)

	var bp BlockProducer
	if withBABE {
		bp = &mockBlockProducer{}
	}

	var fg FinalityGadget
	if withGrandpa {
		fg = &mockFinalityGadget{}
	}

	time.Sleep(time.Second)
	dh, err := NewDigestHandler(stateSrvc.Block, bp, fg, &mockVerifier{})
	require.NoError(t, err)
	return dh
}

func TestDigestHandler_BABEScheduledChange(t *testing.T) {
	handler := newTestDigestHandler(t, true, false)
	handler.Start()
	defer handler.Stop()

	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)

	sc := &types.BABEScheduledChange{
		Auths: []*types.AuthorityRaw{
			{Key: kr.Alice().Public().(*sr25519.PublicKey).AsBytes(), Weight: 0},
		},
		Delay: 3,
	}

	data, err := sc.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	headers := addTestBlocksToState(t, 2, handler.blockState)
	for _, h := range headers {
		handler.blockState.SetFinalizedHash(h.Hash(), 0, 0)
	}

	auths := handler.babe.Authorities()
	require.Nil(t, auths)

	// authorities should change on start of block 3 from start
	headers = addTestBlocksToState(t, 1, handler.blockState)
	for _, h := range headers {
		handler.blockState.SetFinalizedHash(h.Hash(), 0, 0)
	}

	time.Sleep(time.Millisecond * 100)
	auths = handler.babe.Authorities()
	require.Equal(t, 1, len(auths))
}

func TestDigestHandler_BABEForcedChange(t *testing.T) {
	handler := newTestDigestHandler(t, true, false)
	handler.Start()
	defer handler.Stop()

	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)

	fc := &types.BABEForcedChange{

		Auths: []*types.AuthorityRaw{
			{Key: kr.Alice().Public().(*sr25519.PublicKey).AsBytes(), Weight: 0},
		},
		Delay: 3,
	}

	data, err := fc.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	addTestBlocksToState(t, 2, handler.blockState)
	auths := handler.babe.Authorities()
	require.Nil(t, auths)

	// authorities should change on start of block 3 from start
	addTestBlocksToState(t, 1, handler.blockState)
	time.Sleep(time.Millisecond * 100)
	auths = handler.babe.Authorities()
	require.Equal(t, 1, len(auths))
}

func TestDigestHandler_BABEOnDisabled(t *testing.T) {
	handler := newTestDigestHandler(t, true, false)
	handler.Start()
	defer handler.Stop()

	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)

	handler.babe.SetAuthorities([]*types.Authority{
		{Key: kr.Alice().Public().(*sr25519.PublicKey), Weight: 0},
	})

	// try with ID that doesn't exist
	od := &types.OnDisabled{
		ID: 1,
	}

	data, err := od.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	auths := handler.babe.Authorities()
	require.Equal(t, 1, len(auths))

	// try with ID that does exist
	od = &types.OnDisabled{
		ID: 0,
	}

	data, err = od.Encode()
	require.NoError(t, err)

	d = &types.ConsensusDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	auths = handler.babe.Authorities()
	require.Equal(t, 0, len(auths))
}

func TestDigestHandler_BABEPauseAndResume(t *testing.T) {
	handler := newTestDigestHandler(t, true, false)
	handler.Start()
	defer handler.Stop()

	kr, err := keystore.NewSr25519Keyring()
	require.NoError(t, err)

	handler.babe.SetAuthorities([]*types.Authority{
		{Key: kr.Alice().Public().(*sr25519.PublicKey), Weight: 0},
	})

	p := &types.Pause{
		Delay: 3,
	}

	data, err := p.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	headers := addTestBlocksToState(t, 3, handler.blockState)
	for _, h := range headers {
		handler.blockState.SetFinalizedHash(h.Hash(), 0, 0)
	}

	time.Sleep(time.Millisecond * 100)
	auths := handler.babe.Authorities()
	require.Equal(t, 0, len(auths))

	r := &types.Resume{
		Delay: 3,
	}

	data, err = r.Encode()
	require.NoError(t, err)

	d = &types.ConsensusDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	addTestBlocksToState(t, 3, handler.blockState)
	time.Sleep(time.Millisecond * 110)
	auths = handler.babe.Authorities()
	require.Equal(t, 1, len(auths))
}

func TestDigestHandler_GrandpaScheduledChange(t *testing.T) {
	handler := newTestDigestHandler(t, false, true)
	handler.Start()
	defer handler.Stop()
	require.True(t, handler.isFinalityAuthority)

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	sc := &types.GrandpaScheduledChange{
		Auths: []*types.GrandpaAuthorityDataRaw{
			{Key: kr.Alice().Public().(*ed25519.PublicKey).AsBytes(), ID: 0},
		},
		Delay: 3,
	}

	data, err := sc.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	headers := addTestBlocksToState(t, 2, handler.blockState)
	for _, h := range headers {
		handler.blockState.SetFinalizedHash(h.Hash(), 0, 0)
	}

	auths := handler.grandpa.Authorities()
	require.Nil(t, auths)

	// authorities should change on start of block 3 from start
	headers = addTestBlocksToState(t, 1, handler.blockState)
	for _, h := range headers {
		handler.blockState.SetFinalizedHash(h.Hash(), 0, 0)
	}

	time.Sleep(time.Millisecond * 100)
	auths = handler.grandpa.Authorities()
	require.Equal(t, 1, len(auths))
}

func TestDigestHandler_GrandpaForcedChange(t *testing.T) {
	handler := newTestDigestHandler(t, false, true)
	handler.Start()
	defer handler.Stop()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	fc := &types.GrandpaForcedChange{
		Auths: []*types.GrandpaAuthorityDataRaw{
			{Key: kr.Alice().Public().(*ed25519.PublicKey).AsBytes(), ID: 0},
		},
		Delay: 3,
	}

	data, err := fc.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	addTestBlocksToState(t, 2, handler.blockState)
	auths := handler.grandpa.Authorities()
	require.Nil(t, auths)

	// authorities should change on start of block 3 from start
	addTestBlocksToState(t, 1, handler.blockState)
	time.Sleep(time.Millisecond * 100)
	auths = handler.grandpa.Authorities()
	require.Equal(t, 1, len(auths))
}

func TestDigestHandler_GrandpaOnDisabled(t *testing.T) {
	handler := newTestDigestHandler(t, false, true)
	handler.Start()
	defer handler.Stop()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	handler.grandpa.UpdateAuthorities([]*types.Authority{
		{Key: kr.Alice().Public().(*ed25519.PublicKey), Weight: 0},
	})

	// try with ID that doesn't exist
	od := &types.OnDisabled{
		ID: 1,
	}

	data, err := od.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	auths := handler.grandpa.Authorities()
	require.Equal(t, 1, len(auths))

	// try with ID that does exist
	od = &types.OnDisabled{
		ID: 0,
	}

	data, err = od.Encode()
	require.NoError(t, err)

	d = &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	auths = handler.grandpa.Authorities()
	require.Equal(t, 0, len(auths))
}

func TestDigestHandler_GrandpaPauseAndResume(t *testing.T) {
	handler := newTestDigestHandler(t, false, true)
	handler.Start()
	defer handler.Stop()

	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	handler.grandpa.UpdateAuthorities([]*types.Authority{
		{Key: kr.Alice().Public().(*ed25519.PublicKey), Weight: 0},
	})

	p := &types.Pause{
		Delay: 3,
	}

	data, err := p.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	headers := addTestBlocksToState(t, 3, handler.blockState)
	for _, h := range headers {
		handler.blockState.SetFinalizedHash(h.Hash(), 0, 0)
	}

	time.Sleep(time.Millisecond * 100)
	auths := handler.grandpa.Authorities()
	require.Equal(t, 0, len(auths))

	r := &types.Resume{
		Delay: 3,
	}

	data, err = r.Encode()
	require.NoError(t, err)

	d = &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	addTestBlocksToState(t, 3, handler.blockState)
	time.Sleep(time.Millisecond * 110)
	auths = handler.grandpa.Authorities()
	require.Equal(t, 1, len(auths))
}

func TestNextGrandpaAuthorityChange_OneChange(t *testing.T) {
	handler := newTestDigestHandler(t, false, true)
	handler.Start()
	defer handler.Stop()

	block := uint32(3)
	sc := &types.GrandpaScheduledChange{
		Auths: []*types.GrandpaAuthorityDataRaw{},
		Delay: block,
	}

	data, err := sc.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	next := handler.NextGrandpaAuthorityChange()
	require.Equal(t, uint64(block), next)
}

func TestNextGrandpaAuthorityChange_MultipleChanges(t *testing.T) {
	handler := newTestDigestHandler(t, false, true)
	handler.Start()
	defer handler.Stop()

	later := uint32(5)
	sc := &types.GrandpaScheduledChange{
		Auths: []*types.GrandpaAuthorityDataRaw{},
		Delay: later,
	}

	data, err := sc.Encode()
	require.NoError(t, err)

	d := &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	earlier := uint32(3)
	fc := &types.GrandpaForcedChange{
		Auths: []*types.GrandpaAuthorityDataRaw{},
		Delay: earlier,
	}

	data, err = fc.Encode()
	require.NoError(t, err)

	d = &types.ConsensusDigest{
		ConsensusEngineID: types.GrandpaEngineID,
		Data:              data,
	}

	err = handler.HandleConsensusDigest(d)
	require.NoError(t, err)

	next := handler.NextGrandpaAuthorityChange()
	require.Equal(t, uint64(earlier), next)
}
