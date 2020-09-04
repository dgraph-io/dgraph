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

package grandpa

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ChainSafe/gossamer/dot/state"
	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/ed25519"
	"github.com/ChainSafe/gossamer/lib/keystore"

	log "github.com/ChainSafe/log15"
	"github.com/stretchr/testify/require"
)

var testTimeout = 20 * time.Second

func onSameChain(blockState BlockState, a, b common.Hash) bool {
	descendant, err := blockState.IsDescendantOf(a, b)
	if err != nil {
		return false
	}

	if !descendant {
		descendant, err = blockState.IsDescendantOf(b, a)
		if err != nil {
			return false
		}
	}

	return descendant
}

func setupGrandpa(t *testing.T, kp *ed25519.Keypair) (*Service, chan FinalityMessage, chan FinalityMessage, chan FinalityMessage) {
	st := newTestState(t)
	voters := newTestVoters()

	cfg := &Config{
		BlockState:    st.Block,
		DigestHandler: &mockDigestHandler{},
		Voters:        voters,
		Keypair:       kp,
		LogLvl:        log.LvlTrace,
		Authority:     true,
	}

	gs, err := NewService(cfg)
	require.NoError(t, err)
	return gs, gs.in, gs.out, gs.finalized
}

func TestGrandpa_BaseCase(t *testing.T) {
	// this is a base test case that asserts that all validators finalize the same block if they all see the
	// same pre-votes and pre-commits, even if their chains are different
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gss := make([]*Service, len(kr.Keys))
	prevotes := make(map[ed25519.PublicKeyBytes]*Vote)
	precommits := make(map[ed25519.PublicKeyBytes]*Vote)

	for i, gs := range gss {
		gs, _, _, _ = setupGrandpa(t, kr.Keys[i])
		gss[i] = gs
		state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 15)
		prevotes[gs.publicKeyBytes()], err = gs.determinePreVote()
		require.NoError(t, err)
	}

	for _, gs := range gss {
		gs.prevotes = prevotes
	}

	for _, gs := range gss {
		precommits[gs.publicKeyBytes()], err = gs.determinePreCommit()
		require.NoError(t, err)
		err = gs.finalize()
		require.NoError(t, err)
		has, err := gs.blockState.HasJustification(gs.head.Hash())
		require.NoError(t, err)
		require.True(t, has)
	}

	finalized := gss[0].head.Hash()
	for _, gs := range gss {
		require.Equal(t, finalized, gs.head.Hash())
	}
}

func TestGrandpa_DifferentChains(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// this asserts that all validators finalize the same block if they all see the
	// same pre-votes and pre-commits, even if their chains are different lengths
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gss := make([]*Service, len(kr.Keys))
	prevotes := make(map[ed25519.PublicKeyBytes]*Vote)
	precommits := make(map[ed25519.PublicKeyBytes]*Vote)

	for i, gs := range gss {
		gs, _, _, _ = setupGrandpa(t, kr.Keys[i])
		gss[i] = gs

		r := rand.Intn(3)
		state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 4+r)
		prevotes[gs.publicKeyBytes()], err = gs.determinePreVote()
		require.NoError(t, err)
	}

	// only want to add prevotes for a node that has a block that exists on its chain
	for _, gs := range gss {
		for k, pv := range prevotes {
			err = gs.validateVote(pv)
			if err == nil {
				gs.prevotes[k] = pv
			}
		}
	}

	for _, gs := range gss {
		precommits[gs.publicKeyBytes()], err = gs.determinePreCommit()
		require.NoError(t, err)
		err = gs.finalize()
		require.NoError(t, err)
	}

	t.Log(gss[0].blockState.BlocktreeAsString())
	finalized := gss[0].head

	for i, gs := range gss {
		// TODO: this can be changed to equal once attemptToFinalizeRound is implemented (needs check for >=2/3 precommits)
		headOk := onSameChain(gss[0].blockState, finalized.Hash(), gs.head.Hash())
		finalizedOK := onSameChain(gs.blockState, finalized.Hash(), gs.head.Hash())
		require.True(t, headOk || finalizedOK, "node %d did not match: %s", i, gs.blockState.BlocktreeAsString())
	}
}

func broadcastVotes(from <-chan FinalityMessage, to []chan FinalityMessage, done *bool) {
	for v := range from {
		for _, tc := range to {
			if *done {
				return
			}

			tc <- v
		}
	}
}

func cleanup(gs *Service, in, out chan FinalityMessage, done *bool) { //nolint
	*done = true
	close(in)
	gs.cancel()
}

func TestPlayGrandpaRound_BaseCase(t *testing.T) {
	// this asserts that all validators finalize the same block if they all see the
	// same pre-votes and pre-commits, even if their chains are different lengths
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gss := make([]*Service, len(kr.Keys))
	ins := make([]chan FinalityMessage, len(kr.Keys))
	outs := make([]chan FinalityMessage, len(kr.Keys))
	fins := make([]chan FinalityMessage, len(kr.Keys))
	done := false

	for i := range gss {
		gs, in, out, fin := setupGrandpa(t, kr.Keys[i])
		defer cleanup(gs, in, out, &done)

		gss[i] = gs
		ins[i] = in
		outs[i] = out
		fins[i] = fin

		state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 4)
	}

	for _, out := range outs {
		go broadcastVotes(out, ins, &done)
	}

	for _, gs := range gss {
		time.Sleep(time.Millisecond * 100)
		go gs.initiate()
	}

	wg := sync.WaitGroup{}
	wg.Add(len(kr.Keys))

	finalized := make([]*FinalizationMessage, len(kr.Keys))

	for i, fin := range fins {
		go func(i int, fin <-chan FinalityMessage) {
			select {
			case f := <-fin:

				// receive first message, which is finalized block from previous round
				if f.(*FinalizationMessage).Round == 0 {
					select {
					case f = <-fin:
					case <-time.After(testTimeout):
						t.Errorf("did not receive finalized block from %d", i)
					}
				}

				finalized[i] = f.(*FinalizationMessage)

			case <-time.After(testTimeout):
				t.Errorf("did not receive finalized block from %d", i)
			}
			wg.Done()
		}(i, fin)

	}

	wg.Wait()

	for _, fb := range finalized {
		require.NotNil(t, fb)
		require.GreaterOrEqual(t, len(fb.Justification), len(kr.Keys)/2)
		finalized[0].Justification = []*Justification{}
		fb.Justification = []*Justification{}
		require.Equal(t, finalized[0], fb)
	}
}

func TestPlayGrandpaRound_VaryingChain(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// this asserts that all validators finalize the same block if they all see the
	// same pre-votes and pre-commits, even if their chains are different lengths
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gss := make([]*Service, len(kr.Keys))
	ins := make([]chan FinalityMessage, len(kr.Keys))
	outs := make([]chan FinalityMessage, len(kr.Keys))
	fins := make([]chan FinalityMessage, len(kr.Keys))
	done := false

	// this represents the chains that will be slightly ahead of the others
	headers := []*types.Header{}
	diff := 8

	for i := range gss {
		gs, in, out, fin := setupGrandpa(t, kr.Keys[i])
		defer cleanup(gs, in, out, &done)

		gss[i] = gs
		ins[i] = in
		outs[i] = out
		fins[i] = fin

		r := 0
		r = rand.Intn(diff)
		chain, _ := state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 4+r)
		if r == diff-1 {
			headers = chain
		}
	}

	for _, out := range outs {
		go broadcastVotes(out, ins, &done)
	}

	for _, gs := range gss {
		time.Sleep(time.Millisecond * 100)
		go gs.initiate()
	}

	// mimic the chains syncing and catching up
	for _, gs := range gss {
		for _, h := range headers {
			time.Sleep(time.Millisecond * 10)
			block := &types.Block{
				Header: h,
				Body:   &types.Body{},
			}
			gs.blockState.(*state.BlockState).AddBlock(block)
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(len(kr.Keys))

	finalized := make([]*FinalizationMessage, len(kr.Keys))

	for i, fin := range fins {

		go func(i int, fin <-chan FinalityMessage) {
			select {
			case f := <-fin:

				// receive first message, which is finalized block from previous round
				if f.(*FinalizationMessage).Round == 0 {
					select {
					case f = <-fin:
					case <-time.After(testTimeout):
						t.Errorf("did not receive finalized block from %d", i)
					}
				}

				finalized[i] = f.(*FinalizationMessage)
			case <-time.After(testTimeout):
				t.Errorf("did not receive finalized block from %d", i)
			}
			wg.Done()
		}(i, fin)

	}

	wg.Wait()

	for _, fb := range finalized {
		require.NotNil(t, fb)
		require.GreaterOrEqual(t, len(fb.Justification), len(kr.Keys)/2)
		finalized[0].Justification = []*Justification{}
		fb.Justification = []*Justification{}
		require.Equal(t, finalized[0], fb)
	}
}

func TestPlayGrandpaRound_OneThirdEquivocating(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// this asserts that all validators finalize the same block even if 1/3 of voters equivocate
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gss := make([]*Service, len(kr.Keys))
	ins := make([]chan FinalityMessage, len(kr.Keys))
	outs := make([]chan FinalityMessage, len(kr.Keys))
	fins := make([]chan FinalityMessage, len(kr.Keys))

	done := false
	r := byte(rand.Intn(256))

	for i := range gss {
		gs, in, out, fin := setupGrandpa(t, kr.Keys[i])
		defer cleanup(gs, in, out, &done)

		gss[i] = gs
		ins[i] = in
		outs[i] = out
		fins[i] = fin

		// this creates a tree with 2 branches starting at depth 2
		branches := make(map[int]int)
		branches[2] = 1
		state.AddBlocksToStateWithFixedBranches(t, gs.blockState.(*state.BlockState), 4, branches, r)
	}

	// should have blocktree for all nodes
	leaves := gss[0].blockState.Leaves()

	for _, out := range outs {
		go broadcastVotes(out, ins, &done)
	}

	for _, gs := range gss {
		time.Sleep(time.Millisecond * 100)
		go gs.initiate()
	}

	// nodes 6, 7, 8 will equivocate
	for _, gs := range gss {
		vote, err := NewVoteFromHash(leaves[1], gs.blockState)
		require.NoError(t, err)

		vmsg, err := gs.createVoteMessage(vote, prevote, gs.keypair)
		require.NoError(t, err)

		for _, in := range ins {
			in <- vmsg
		}
	}

	wg := sync.WaitGroup{}
	wg.Add(len(kr.Keys))

	finalized := make([]*FinalizationMessage, len(kr.Keys))

	for i, fin := range fins {

		go func(i int, fin <-chan FinalityMessage) {
			select {
			case f := <-fin:

				// receive first message, which is finalized block from previous round
				if f.(*FinalizationMessage).Round == 0 {
					select {
					case f = <-fin:
					case <-time.After(testTimeout):
						t.Errorf("did not receive finalized block from %d", i)
					}
				}

				finalized[i] = f.(*FinalizationMessage)
			case <-time.After(testTimeout):
				t.Errorf("did not receive finalized block from %d", i)
			}
			wg.Done()
		}(i, fin)

	}

	wg.Wait()

	for _, fb := range finalized {
		require.NotNil(t, fb)
		require.GreaterOrEqual(t, len(fb.Justification), len(kr.Keys)/2)
		finalized[0].Justification = []*Justification{}
		fb.Justification = []*Justification{}
		require.Equal(t, finalized[0], fb)
	}
}

func TestPlayGrandpaRound_MultipleRounds(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	// this asserts that all validators finalize the same block in successive rounds
	kr, err := keystore.NewEd25519Keyring()
	require.NoError(t, err)

	gss := make([]*Service, len(kr.Keys))
	ins := make([]chan FinalityMessage, len(kr.Keys))
	outs := make([]chan FinalityMessage, len(kr.Keys))
	fins := make([]chan FinalityMessage, len(kr.Keys))
	done := false

	for i := range gss {
		gs, in, out, fin := setupGrandpa(t, kr.Keys[i])
		defer cleanup(gs, in, out, &done)

		gss[i] = gs
		ins[i] = in
		outs[i] = out
		fins[i] = fin

		state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 4)
	}

	for _, out := range outs {
		go broadcastVotes(out, ins, &done)
	}

	for _, gs := range gss {
		// start rounds at slightly different times to account for real-time node differences
		time.Sleep(time.Millisecond * 100)
		go gs.initiate()
	}

	rounds := 10

	for j := 0; j < rounds; j++ {

		wg := sync.WaitGroup{}
		wg.Add(len(kr.Keys))

		finalized := make([]*FinalizationMessage, len(kr.Keys))

		for i, fin := range fins {

			go func(i int, fin <-chan FinalityMessage) {
				select {
				case f := <-fin:

					// receive first message, which is finalized block from previous round
					if f.(*FinalizationMessage).Round == uint64(j) {
						select {
						case f = <-fin:
						case <-time.After(testTimeout):
							t.Errorf("did not receive finalized block from %d", i)
						}
					}

					finalized[i] = f.(*FinalizationMessage)
				case <-time.After(testTimeout):
					t.Errorf("did not receive finalized block from %d", i)
				}
				wg.Done()
			}(i, fin)

		}

		wg.Wait()

		head := gss[0].blockState.(*state.BlockState).BestBlockHash()
		for _, fb := range finalized {
			require.NotNil(t, fb)
			require.Equal(t, head, fb.Vote.hash)
			require.GreaterOrEqual(t, len(fb.Justification), len(kr.Keys)/2)
			finalized[0].Justification = []*Justification{}
			fb.Justification = []*Justification{}
			require.Equal(t, finalized[0], fb)
		}

		for _, gs := range gss {
			state.AddBlocksToState(t, gs.blockState.(*state.BlockState), 1)
		}

	}
}
