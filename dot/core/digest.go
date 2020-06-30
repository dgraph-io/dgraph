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
	"errors"
	"math/big"
	"time"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/scale"
)

type digestHandler struct {
	// interfaces
	blockState BlockState
	grandpa    FinalityGadget
	babe       BlockProducer

	// state variables
	stopped bool

	// BABE changes
	babeScheduledChange *babeChange //nolint
	babeForcedChange    *babeChange //nolint
	babePause           *pause      //nolint
	babeResume          *resume     //nolint

	// GRANDPA changes
	grandpaScheduledChange *grandpaChange
	grandpaForcedChange    *grandpaChange
	grandpaPause           *pause
	grandpaResume          *resume
	grandpaAuths           []*types.GrandpaAuthorityData // saved in case of pause
}

type babeChange struct { //nolint
	auths   []*types.BABEAuthorityData //nolint
	atBlock *big.Int                   //nolint
}

type grandpaChange struct {
	auths   []*types.GrandpaAuthorityData
	atBlock *big.Int
}

type pause struct {
	atBlock *big.Int
}

type resume struct {
	atBlock *big.Int
}

func newDigestHandler(blockState BlockState, babe BlockProducer, grandpa FinalityGadget) *digestHandler {
	return &digestHandler{
		blockState: blockState,
		grandpa:    grandpa,
		babe:       babe,
		stopped:    true,
	}
}

func (h *digestHandler) start() {
	go h.handleGrandpaChanges()
	h.stopped = false
}

func (h *digestHandler) stop() {
	h.stopped = true
}

func (h *digestHandler) handleGrandpaChanges() {
	for {
		if h.stopped {
			return
		}

		// TODO: update block state to register new channels for added blocks
		curr, err := h.blockState.BestBlockHeader()
		if err != nil {
			continue
		}

		pause := h.grandpaPause
		if pause != nil && curr.Number.Cmp(pause.atBlock) == 0 {
			// save authority data for Resume
			h.grandpaAuths = h.grandpa.Authorities()
			h.grandpa.UpdateAuthorities([]*types.GrandpaAuthorityData{})
			h.grandpaPause = nil
			continue
		}

		resume := h.grandpaResume
		if resume != nil && curr.Number.Cmp(resume.atBlock) == 0 {
			h.grandpa.UpdateAuthorities(h.grandpaAuths)
			h.grandpaResume = nil
			continue
		}

		fc := h.grandpaForcedChange
		if fc != nil && curr.Number.Cmp(fc.atBlock) == 0 {
			h.grandpa.UpdateAuthorities(fc.auths)
			h.grandpaForcedChange = nil
			continue
		}

		sc := h.grandpaScheduledChange
		if sc != nil && curr.Number.Cmp(sc.atBlock) == 0 {
			h.grandpa.UpdateAuthorities(sc.auths)
			h.grandpaScheduledChange = nil
		}

		time.Sleep(time.Millisecond * 100)
	}
}

// handleConsensusDigest is the function used by the syncer to handler a consensus digest
func (h *digestHandler) handleConsensusDigest(d *types.ConsensusDigest) error {
	t := d.DataType()

	switch t {
	case types.ScheduledChangeType:
		return h.handleScheduledChange(d)
	case types.ForcedChangeType:
		return h.handleForcedChange(d)
	case types.OnDisabledType:
		return h.handleOnDisabled(d)
	case types.PauseType:
		return h.handlePause(d)
	case types.ResumeType:
		return h.handleResume(d)
	default:
		return errors.New("invalid consensus digest data")
	}
}

func (h *digestHandler) handleScheduledChange(d *types.ConsensusDigest) error {
	curr, err := h.blockState.BestBlockHeader()
	if err != nil {
		return err
	}

	if d.ConsensusEngineID == types.BabeEngineID {
		// TODO
	} else {
		if h.grandpaScheduledChange != nil {
			return errors.New("already have scheduled change scheduled")
		}

		sc := &types.GrandpaScheduledChange{}
		dec, err := scale.Decode(d.Data[1:], sc)
		if err != nil {
			return err
		}
		sc = dec.(*types.GrandpaScheduledChange)

		c, err := newGrandpaChange(sc.Auths, sc.Delay, curr.Number)
		if err != nil {
			return err
		}

		h.grandpaScheduledChange = c
	}

	return nil
}

func (h *digestHandler) handleForcedChange(d *types.ConsensusDigest) error {
	curr, err := h.blockState.BestBlockHeader()
	if err != nil {
		return err
	}

	if d.ConsensusEngineID == types.BabeEngineID {
		// TODO
	} else {
		if h.grandpaForcedChange != nil {
			return errors.New("already have forced change scheduled")
		}

		fc := &types.GrandpaForcedChange{}
		dec, err := scale.Decode(d.Data[1:], fc)
		if err != nil {
			return err
		}
		fc = dec.(*types.GrandpaForcedChange)

		c, err := newGrandpaChange(fc.Auths, fc.Delay, curr.Number)
		if err != nil {
			return err
		}

		h.grandpaForcedChange = c
	}

	return nil
}

func (h *digestHandler) handleOnDisabled(d *types.ConsensusDigest) error {
	od := &types.OnDisabled{}
	dec, err := scale.Decode(d.Data[1:], od)
	if err != nil {
		return err
	}
	od = dec.(*types.OnDisabled)

	if d.ConsensusEngineID == types.BabeEngineID {
		// TODO
	} else {
		curr := h.grandpa.Authorities()
		next := []*types.GrandpaAuthorityData{}

		for _, auth := range curr {
			if auth.ID != od.ID {
				next = append(next, auth)
			}
		}

		h.grandpa.UpdateAuthorities(next)
	}

	return nil
}

func (h *digestHandler) handlePause(d *types.ConsensusDigest) error {
	curr, err := h.blockState.BestBlockHeader()
	if err != nil {
		return err
	}

	p := &types.Pause{}
	dec, err := scale.Decode(d.Data[1:], p)
	if err != nil {
		return err
	}
	p = dec.(*types.Pause)

	delay := big.NewInt(int64(p.Delay))

	if d.ConsensusEngineID == types.BabeEngineID {
		// TODO
	} else {
		h.grandpaPause = &pause{
			atBlock: big.NewInt(-1).Add(curr.Number, delay),
		}
	}

	return nil
}

func (h *digestHandler) handleResume(d *types.ConsensusDigest) error {
	curr, err := h.blockState.BestBlockHeader()
	if err != nil {
		return err
	}

	p := &types.Resume{}
	dec, err := scale.Decode(d.Data[1:], p)
	if err != nil {
		return err
	}
	p = dec.(*types.Resume)

	delay := big.NewInt(int64(p.Delay))

	if d.ConsensusEngineID == types.BabeEngineID {
		// TODO
	} else {
		h.grandpaResume = &resume{
			atBlock: big.NewInt(-1).Add(curr.Number, delay),
		}
	}

	return nil
}

func newGrandpaChange(raw []*types.GrandpaAuthorityDataRaw, delay uint32, currBlock *big.Int) (*grandpaChange, error) {
	auths, err := types.GrandpaAuthorityDataRawToAuthorityData(raw)
	if err != nil {
		return nil, err
	}

	d := big.NewInt(int64(delay))

	return &grandpaChange{
		auths:   auths,
		atBlock: big.NewInt(-1).Add(currBlock, d),
	}, nil
}
