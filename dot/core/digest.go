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
	"context"
	"errors"
	"math/big"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/scale"
	log "github.com/ChainSafe/log15"
)

var maxUint64 = uint64(2^64) - 1

// DigestHandler is used to handle consensus messages and relevant authority updates to BABE and GRANDPA
type DigestHandler struct {
	ctx    context.Context
	cancel context.CancelFunc

	// interfaces
	blockState          BlockState
	grandpa             FinalityGadget
	babe                BlockProducer
	verifier            Verifier
	isFinalityAuthority bool
	isBlockProducer     bool

	// block notification channels
	imported    chan *types.Block
	importedID  byte
	finalized   chan *types.Header
	finalizedID byte

	// BABE changes
	babeScheduledChange *babeChange
	babeForcedChange    *babeChange
	babePause           *pause
	babeResume          *resume
	babeAuths           []*types.Authority // saved in case of pause

	// GRANDPA changes
	grandpaScheduledChange *grandpaChange
	grandpaForcedChange    *grandpaChange
	grandpaPause           *pause
	grandpaResume          *resume
	grandpaAuths           []*types.Authority // saved in case of pause
}

type babeChange struct {
	auths   []*types.Authority
	atBlock *big.Int
}

type grandpaChange struct {
	auths   []*types.Authority
	atBlock *big.Int
}

type pause struct {
	atBlock *big.Int
}

type resume struct {
	atBlock *big.Int
}

// NewDigestHandler returns a new DigestHandler
func NewDigestHandler(blockState BlockState, babe BlockProducer, grandpa FinalityGadget, verifier Verifier) (*DigestHandler, error) {
	imported := make(chan *types.Block, 16)
	finalized := make(chan *types.Header, 16)
	iid, err := blockState.RegisterImportedChannel(imported)
	if err != nil {
		return nil, err
	}

	fid, err := blockState.RegisterFinalizedChannel(finalized)
	if err != nil {
		return nil, err
	}

	isFinalityAuthority := grandpa != nil
	isBlockProducer := babe != nil

	ctx, cancel := context.WithCancel(context.Background())

	return &DigestHandler{
		ctx:                 ctx,
		cancel:              cancel,
		blockState:          blockState,
		grandpa:             grandpa,
		babe:                babe,
		verifier:            verifier,
		isFinalityAuthority: isFinalityAuthority,
		isBlockProducer:     isBlockProducer,
		imported:            imported,
		importedID:          iid,
		finalized:           finalized,
		finalizedID:         fid,
	}, nil
}

// Start starts the DigestHandler
func (h *DigestHandler) Start() {
	ctx, _ := context.WithCancel(h.ctx) //nolint
	go h.handleBlockImport(ctx)
	ctx, _ = context.WithCancel(h.ctx) //nolint
	go h.handleBlockFinalization(ctx)
}

// Stop stops the DigestHandler
func (h *DigestHandler) Stop() {
	h.cancel()
	h.blockState.UnregisterImportedChannel(h.importedID)
	h.blockState.UnregisterFinalizedChannel(h.finalizedID)
	close(h.imported)
	close(h.finalized)
}

// SetFinalityGadget sets the digest handler's grandpa instance
func (h *DigestHandler) SetFinalityGadget(grandpa FinalityGadget) {
	h.grandpa = grandpa
}

// NextGrandpaAuthorityChange returns the block number of the next upcoming grandpa authorities change.
// It returns 0 if no change is scheduled.
func (h *DigestHandler) NextGrandpaAuthorityChange() uint64 {
	next := maxUint64

	if h.grandpaScheduledChange != nil {
		next = h.grandpaScheduledChange.atBlock.Uint64()
	}

	if h.grandpaForcedChange != nil && h.grandpaForcedChange.atBlock.Uint64() < next {
		next = h.grandpaForcedChange.atBlock.Uint64()
	}

	if h.grandpaPause != nil && h.grandpaPause.atBlock.Uint64() < next {
		next = h.grandpaPause.atBlock.Uint64()
	}

	if h.grandpaResume != nil && h.grandpaResume.atBlock.Uint64() < next {
		next = h.grandpaResume.atBlock.Uint64()
	}

	return next
}

// HandleConsensusDigest is the function used by the syncer to handle a consensus digest
func (h *DigestHandler) HandleConsensusDigest(d *types.ConsensusDigest) error {
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

func (h *DigestHandler) handleBlockImport(ctx context.Context) {
	for {
		select {
		case block := <-h.imported:
			if block == nil || block.Header == nil {
				continue
			}

			if h.isFinalityAuthority {
				h.handleGrandpaChangesOnImport(block.Header.Number)
			}

			if h.isBlockProducer {
				h.handleBABEChangesOnImport(block.Header)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (h *DigestHandler) handleBlockFinalization(ctx context.Context) {
	for {
		select {
		case header := <-h.finalized:
			if header == nil {
				continue
			}

			if h.isFinalityAuthority {
				h.handleGrandpaChangesOnFinalization(header.Number)
			}

			if h.isBlockProducer {
				h.handleBABEChangesOnFinalization(header)
			}
		case <-ctx.Done():
			return
		}
	}

}

func (h *DigestHandler) handleBABEChangesOnImport(header *types.Header) {
	num := header.Number
	resume := h.babeResume
	if resume != nil && num.Cmp(resume.atBlock) == 0 {
		err := h.babe.SetAuthorities(h.babeAuths)
		if err != nil {
			log.Warn("error setting authorities", "error", err)
		}
		h.verifier.SetAuthorityChangeAtBlock(header, h.babeAuths)
		h.babeResume = nil
	}

	fc := h.babeForcedChange
	if fc != nil && num.Cmp(fc.atBlock) == 0 {
		err := h.babe.SetAuthorities(fc.auths)
		if err != nil {
			log.Warn("error setting authorities", "error", err)
		}
		h.verifier.SetAuthorityChangeAtBlock(header, fc.auths)
		h.babeForcedChange = nil
	}
}

func (h *DigestHandler) handleBABEChangesOnFinalization(header *types.Header) {
	num := header.Number
	pause := h.babePause
	if pause != nil && num.Cmp(pause.atBlock) == 0 {
		// save authority data for Resume
		h.babeAuths = h.babe.Authorities()
		err := h.babe.SetAuthorities([]*types.Authority{})
		if err != nil {
			log.Warn("error setting authorities", "error", err)
		}
		h.verifier.SetAuthorityChangeAtBlock(header, []*types.Authority{})
		h.babePause = nil
	}

	sc := h.babeScheduledChange
	if sc != nil && num.Cmp(sc.atBlock) == 0 {
		err := h.babe.SetAuthorities(sc.auths)
		if err != nil {
			log.Warn("error setting authorities", "error", err)
		}
		h.verifier.SetAuthorityChangeAtBlock(header, sc.auths)
		h.babeScheduledChange = nil
	}

	// if blocks get finalized before forced change takes place, disregard it
	h.babeForcedChange = nil
}

func (h *DigestHandler) handleGrandpaChangesOnImport(num *big.Int) {
	resume := h.grandpaResume
	if resume != nil && num.Cmp(resume.atBlock) == 0 {
		h.grandpa.UpdateAuthorities(h.grandpaAuths)
		h.grandpaResume = nil
	}

	fc := h.grandpaForcedChange
	if fc != nil && num.Cmp(fc.atBlock) == 0 {
		h.grandpa.UpdateAuthorities(fc.auths)
		h.grandpaForcedChange = nil
	}
}

func (h *DigestHandler) handleGrandpaChangesOnFinalization(num *big.Int) {
	pause := h.grandpaPause
	if pause != nil && num.Cmp(pause.atBlock) == 0 {
		// save authority data for Resume
		h.grandpaAuths = h.grandpa.Authorities()
		h.grandpa.UpdateAuthorities([]*types.Authority{})
		h.grandpaPause = nil
	}

	sc := h.grandpaScheduledChange
	if sc != nil && num.Cmp(sc.atBlock) == 0 {
		h.grandpa.UpdateAuthorities(sc.auths)
		h.grandpaScheduledChange = nil
	}

	// if blocks get finalized before forced change takes place, disregard it
	h.grandpaForcedChange = nil
}

func (h *DigestHandler) handleScheduledChange(d *types.ConsensusDigest) error {
	curr, err := h.blockState.BestBlockHeader()
	if err != nil {
		return err
	}

	if d.ConsensusEngineID == types.BabeEngineID {
		if h.babeScheduledChange != nil {
			return errors.New("already have scheduled change scheduled")
		}

		sc := &types.BABEScheduledChange{}
		dec, err := scale.Decode(d.Data[1:], sc)
		if err != nil {
			return err
		}
		sc = dec.(*types.BABEScheduledChange)

		c, err := newBABEChange(sc.Auths, sc.Delay, curr.Number)
		if err != nil {
			return err
		}

		h.babeScheduledChange = c
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

func (h *DigestHandler) handleForcedChange(d *types.ConsensusDigest) error {
	curr, err := h.blockState.BestBlockHeader()
	if err != nil {
		return err
	}

	if d.ConsensusEngineID == types.BabeEngineID {
		if h.babeForcedChange != nil {
			return errors.New("already have forced change scheduled")
		}

		fc := &types.BABEForcedChange{}
		dec, err := scale.Decode(d.Data[1:], fc)
		if err != nil {
			return err
		}
		fc = dec.(*types.BABEForcedChange)

		c, err := newBABEChange(fc.Auths, fc.Delay, curr.Number)
		if err != nil {
			return err
		}

		h.babeForcedChange = c
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

func (h *DigestHandler) handleOnDisabled(d *types.ConsensusDigest) error {
	od := &types.OnDisabled{}
	dec, err := scale.Decode(d.Data[1:], od)
	if err != nil {
		return err
	}
	od = dec.(*types.OnDisabled)

	if d.ConsensusEngineID == types.BabeEngineID {
		curr := h.babe.Authorities()
		next := []*types.Authority{}

		for i, auth := range curr {
			if uint64(i) != od.ID {
				next = append(next, auth)
			}
		}

		err := h.babe.SetAuthorities(next)
		if err != nil {
			return err
		}
	} else {
		curr := h.grandpa.Authorities()
		next := []*types.Authority{}

		for _, auth := range curr {
			if auth.Weight != od.ID {
				next = append(next, auth)
			}
		}

		h.grandpa.UpdateAuthorities(next)
	}

	return nil
}

func (h *DigestHandler) handlePause(d *types.ConsensusDigest) error {
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
		h.babePause = &pause{
			atBlock: big.NewInt(-1).Add(curr.Number, delay),
		}
	} else {
		h.grandpaPause = &pause{
			atBlock: big.NewInt(-1).Add(curr.Number, delay),
		}
	}

	return nil
}

func (h *DigestHandler) handleResume(d *types.ConsensusDigest) error {
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
		h.babeResume = &resume{
			atBlock: big.NewInt(-1).Add(curr.Number, delay),
		}
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

func newBABEChange(raw []*types.AuthorityRaw, delay uint32, currBlock *big.Int) (*babeChange, error) {
	auths, err := types.BABEAuthorityRawToAuthority(raw)
	if err != nil {
		return nil, err
	}

	d := big.NewInt(int64(delay))

	return &babeChange{
		auths:   auths,
		atBlock: big.NewInt(-1).Add(currBlock, d),
	}, nil
}
