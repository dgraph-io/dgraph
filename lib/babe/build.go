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

package babe

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/transaction"

	log "github.com/ChainSafe/log15"
)

// BuildBlock builds a block for the slot with the given parent.
// TODO: separate block builder logic into separate module. The only reason this is exported is so other packages
// can build blocks for testing, but it would be preferred to have the builder functionality separated.
func (b *Session) BuildBlock(parent *types.Header, slot Slot) (*types.Block, error) {
	return b.buildBlock(parent, slot)
}

// construct a block for this slot with the given parent
func (b *Session) buildBlock(parent *types.Header, slot Slot) (*types.Block, error) {
	log.Trace("[babe] build block", "parent", parent, "slot", slot)

	// create pre-digest
	preDigest, err := b.buildBlockPreDigest(slot)
	if err != nil {
		return nil, err
	}

	log.Trace("[babe] built pre-digest")

	// create new block header
	number := big.NewInt(0).Add(parent.Number, big.NewInt(1))
	header, err := types.NewHeader(parent.Hash(), number, common.Hash{}, common.Hash{}, [][]byte{})
	if err != nil {
		return nil, err
	}

	// initialize block header
	err = b.rt.InitializeBlock(header)
	if err != nil {
		return nil, err
	}

	log.Trace("[babe] initialized block")

	// add block inherents
	err = b.buildBlockInherents(slot)
	if err != nil {
		return nil, fmt.Errorf("cannot build inherents: %s", err)
	}

	log.Trace("[babe] built block inherents")

	// add block extrinsics
	included, err := b.buildBlockExtrinsics(slot)
	if err != nil {
		return nil, fmt.Errorf("cannot build extrinsics: %s", err)
	}

	log.Trace("[babe] built block extrinsics")

	// finalize block
	header, err = b.rt.FinalizeBlock()
	if err != nil {
		b.addToQueue(included)
		return nil, fmt.Errorf("cannot finalize block: %s", err)
	}

	log.Trace("[babe] finalized block")

	header.ParentHash = parent.Hash()
	header.Number.Add(parent.Number, big.NewInt(1))

	// add BABE header to digest
	header.Digest = append(header.Digest, preDigest.Encode())

	// create seal and add to digest
	seal, err := b.buildBlockSeal(header)
	if err != nil {
		return nil, err
	}

	header.Digest = append(header.Digest, seal.Encode())

	log.Trace("[babe] built block seal")

	body, err := extrinsicsToBody(included)
	if err != nil {
		return nil, err
	}

	block := &types.Block{
		Header: header,
		Body:   body,
	}

	return block, nil
}

// buildBlockSeal creates the seal for the block header.
// the seal consists of the ConsensusEngineID and a signature of the encoded block header.
func (b *Session) buildBlockSeal(header *types.Header) (*types.SealDigest, error) {
	encHeader, err := header.Encode()
	if err != nil {
		return nil, err
	}

	sig, err := b.keypair.Sign(encHeader)
	if err != nil {
		return nil, err
	}

	return &types.SealDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              sig,
	}, nil
}

// buildBlockPreDigest creates the pre-digest for the slot.
// the pre-digest consists of the ConsensusEngineID and the encoded BABE header for the slot.
func (b *Session) buildBlockPreDigest(slot Slot) (*types.PreRuntimeDigest, error) {
	babeHeader, err := b.buildBlockBabeHeader(slot)
	if err != nil {
		return nil, err
	}

	encBabeHeader := babeHeader.Encode()

	return &types.PreRuntimeDigest{
		ConsensusEngineID: types.BabeEngineID,
		Data:              encBabeHeader,
	}, nil
}

// buildBlockBabeHeader creates the BABE header for the slot.
// the BABE header includes the proof of authorship right for this slot.
func (b *Session) buildBlockBabeHeader(slot Slot) (*types.BabeHeader, error) {
	if b.slotToProof[slot.number] == nil {
		// if we don't have a proof already set, re-run lottery.
		// this can be removed when this is separated into a block builder module,
		// or moved to handleSlot.
		proof, err := b.runLottery(slot.number)
		if err != nil {
			return nil, err
		}

		if proof == nil {
			return nil, errors.New("not authorized to produce block")
		}

		b.slotToProof[slot.number] = proof
	}
	outAndProof := b.slotToProof[slot.number]
	return &types.BabeHeader{
		VrfOutput:          outAndProof.output,
		VrfProof:           outAndProof.proof,
		BlockProducerIndex: b.authorityIndex,
		SlotNumber:         slot.number,
	}, nil
}

// buildBlockExtrinsics applies extrinsics to the block. it returns an array of included extrinsics.
// for each extrinsic in queue, add it to the block, until the slot ends or the block is full.
// if any extrinsic fails, it returns an empty array and an error.
func (b *Session) buildBlockExtrinsics(slot Slot) ([]*transaction.ValidTransaction, error) {
	next := b.nextReadyExtrinsic()
	included := []*transaction.ValidTransaction{}

	for !hasSlotEnded(slot) && next != nil {
		log.Trace("[babe] build block", "applying extrinsic", next)
		ret, err := b.rt.ApplyExtrinsic(next)
		if err != nil {
			return nil, err
		}

		// if ret == 0x0001, there is a dispatch error; if ret == 0x01, there is an apply error
		if ret[0] == 1 || bytes.Equal(ret[:2], []byte{0, 1}) {
			errTxt, err := determineError(ret)
			if err != nil {
				return nil, err
			}
			// remove invalid extrinsic from queue
			b.transactionQueue.Pop()

			// re-add previously popped extrinsics back to queue
			b.addToQueue(included)

			return nil, errors.New("error applying extrinsic: " + errTxt)

		}

		log.Trace("[babe] build block applied extrinsic", "extrinsic", next)

		// keep track of included transactions; re-add them to queue later if block building fails
		t := b.transactionQueue.Pop()
		included = append(included, t)
		next = b.nextReadyExtrinsic()
	}

	return included, nil
}

// buildBlockInherents applies the inherents for a block
func (b *Session) buildBlockInherents(slot Slot) error {
	// Setup inherents: add timstap0 and babeslot
	idata := NewInherentsData()
	err := idata.SetInt64Inherent(Timstap0, uint64(time.Now().Unix()))
	if err != nil {
		return err
	}

	err = idata.SetInt64Inherent(Babeslot, slot.number)
	if err != nil {
		return err
	}

	ienc, err := idata.Encode()
	if err != nil {
		return err
	}

	// Call BlockBuilder_inherent_extrinsics
	_, err = b.rt.InherentExtrinsics(ienc)
	if err != nil {
		return err
	}

	return nil
}

func (b *Session) addToQueue(txs []*transaction.ValidTransaction) {
	for _, t := range txs {
		hash, err := b.transactionQueue.Push(t)
		if err != nil {
			log.Trace("[babe] Failed to add transaction to queue", "error", err)
		} else {
			log.Trace("[babe] Added transaction to queue", "hash", hash)
		}
	}
}

// nextReadyExtrinsic peeks from the transaction queue. it does not remove any transactions from the queue
func (b *Session) nextReadyExtrinsic() types.Extrinsic {
	transaction := b.transactionQueue.Peek()
	if transaction == nil {
		return nil
	}
	return transaction.Extrinsic
}

func hasSlotEnded(slot Slot) bool {
	return slot.start+slot.duration < uint64(time.Now().Unix())
}

func extrinsicsToBody(txs []*transaction.ValidTransaction) (*types.Body, error) {
	extrinsics := []types.Extrinsic{}

	for _, tx := range txs {
		extrinsics = append(extrinsics, tx.Extrinsic)
	}

	return types.NewBodyFromExtrinsics(extrinsics)
}

func determineError(res []byte) (string, error) {
	log.Error("[babe] build block apply extrinsic", "error", res)
	var errTxt strings.Builder
	var err error

	// when res[0] == 0x01 it is an apply error
	if res[0] == 1 {
		_, err = errTxt.WriteString("Apply error, type: ")
		if bytes.Equal(res[1:], []byte{0}) {
			_, err = errTxt.WriteString("NoPermission")
		}
		if bytes.Equal(res[1:], []byte{1}) {
			_, err = errTxt.WriteString("BadState")
		}
		if bytes.Equal(res[1:], []byte{2}) {
			_, err = errTxt.WriteString("Validity")
		}
		if bytes.Equal(res[1:], []byte{2, 0, 0}) {
			_, err = errTxt.WriteString("Call")
		}
		if bytes.Equal(res[1:], []byte{2, 0, 1}) {
			_, err = errTxt.WriteString("Payment")
		}
		if bytes.Equal(res[1:], []byte{2, 0, 2}) {
			_, err = errTxt.WriteString("Future")
		}
		if bytes.Equal(res[1:], []byte{2, 0, 3}) {
			_, err = errTxt.WriteString("Stale")
		}
		if bytes.Equal(res[1:], []byte{2, 0, 4}) {
			_, err = errTxt.WriteString("BadProof")
		}
		if bytes.Equal(res[1:], []byte{2, 0, 5}) {
			_, err = errTxt.WriteString("AncientBirthBlock")
		}
		if bytes.Equal(res[1:], []byte{2, 0, 6}) {
			_, err = errTxt.WriteString("ExhaustsResources")
		}
		if bytes.Equal(res[1:], []byte{2, 0, 7}) {
			_, err = errTxt.WriteString("Custom")
		}
		if bytes.Equal(res[1:], []byte{2, 1, 0}) {
			_, err = errTxt.WriteString("CannotLookup")
		}
		if bytes.Equal(res[1:], []byte{2, 1, 1}) {
			_, err = errTxt.WriteString("NoUnsignedValidator")
		}
		if bytes.Equal(res[1:], []byte{2, 1, 2}) {
			_, err = errTxt.WriteString("Custom")
		}
	}

	// when res[:2] == 0x0001 it's a dispatch error
	if bytes.Equal(res[:2], []byte{0, 1}) {
		mod := res[2:3]
		errID := res[3:4]
		_, err = errTxt.WriteString("Dispatch Error, module: " + string(mod) + " error: " + string(errID))
	}
	return errTxt.String(), err
}
