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
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
)

// initiateEpoch sets the randomness for the given epoch, runs the lottery for the slots in the epoch,
// and stores updated EpochInfo in the database
func (b *Service) initiateEpoch(epoch, startSlot uint64) error {
	if epoch > 2 {
		var err error
		b.randomness, err = b.epochRandomness(epoch)
		if err != nil {
			return err
		}
	}

	if epoch > 1 {
		first, err := b.blockState.BestBlockNumber()
		if err != nil {
			return err
		}

		if first.Uint64() == 0 {
			first = big.NewInt(1) // first epoch starts at block 1, not block 0
		}

		// Duration may only change when the runtime is updated. This call happens in SetRuntime()
		// FirstBlock is used to calculate the randomness (blocks in an epoch used to calculate epoch randomness for 2 epochs ahead)
		// Randomness changes every epoch, as calculated by epochRandomness()
		info := &types.EpochInfo{
			Duration:   b.config.EpochLength,
			FirstBlock: first.Uint64(),
			Randomness: b.randomness,
		}

		err = b.epochState.SetEpochInfo(epoch, info)
		if err != nil {
			return err
		}
	}

	var err error
	for i := startSlot; i < startSlot+b.config.EpochLength; i++ {
		b.slotToProof[i], err = b.runLottery(i)
		if err != nil {
			return fmt.Errorf("error running slot lottery at slot %d: error %s", i, err)
		}
	}

	return nil
}

func (b *Service) epochRandomness(epoch uint64) ([types.RandomnessLength]byte, error) {
	if epoch < 2 {
		return b.randomness, nil
	}

	epochMinusTwo, err := b.epochState.GetEpochInfo(epoch - 2)
	if err != nil {
		return [types.RandomnessLength]byte{}, err
	}

	lastNum := epochMinusTwo.FirstBlock + epochMinusTwo.Duration - 1
	first, err := b.blockState.GetBlockByNumber(big.NewInt(int64(epochMinusTwo.FirstBlock)))
	if err != nil {
		return [types.RandomnessLength]byte{}, err
	}

	last, err := b.blockState.GetBlockByNumber(big.NewInt(int64(lastNum)))
	if err != nil {
		return [types.RandomnessLength]byte{}, err
	}

	sc, err := b.blockState.SubChain(first.Header.Hash(), last.Header.Hash())
	if err != nil {
		return [types.RandomnessLength]byte{}, err
	}

	epochBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(epochBytes, epoch)
	buf := append(b.randomness[:], epochBytes...)
	for _, hash := range sc {
		header, err := b.blockState.GetHeader(hash)
		if err != nil {
			return [types.RandomnessLength]byte{}, err
		}

		output, err := getVRFOutput(header)
		if err != nil {
			return [types.RandomnessLength]byte{}, err
		}

		buf = append(buf, output[:]...)
	}

	return common.Blake2bHash(buf)
}

// incrementEpoch increments the current epoch stored in the db and returns the new epoch number
func (b *Service) incrementEpoch() (uint64, error) {
	epoch, err := b.epochState.GetCurrentEpoch()
	if err != nil {
		return 0, err
	}

	next := epoch + 1
	err = b.epochState.SetCurrentEpoch(next)
	if err != nil {
		return 0, err
	}

	return next, nil
}

// runLottery runs the lottery for a specific slot number
// returns an encoded VrfOutput and VrfProof if validator is authorized to produce a block for that slot, nil otherwise
// output = return[0:32]; proof = return[32:96]
func (b *Service) runLottery(slot uint64) (*VrfOutputAndProof, error) {
	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	vrfInput := append(slotBytes, b.randomness[:]...)

	output, proof, err := b.vrfSign(vrfInput)
	if err != nil {
		return nil, err
	}

	outputInt := big.NewInt(0).SetBytes(output[:])
	if b.epochThreshold == nil {
		err = b.setEpochThreshold()
		if err != nil {
			return nil, err
		}
	}

	if outputInt.Cmp(b.epochThreshold) < 0 {
		outbytes := [sr25519.VrfOutputLength]byte{}
		copy(outbytes[:], output)
		proofbytes := [sr25519.VrfProofLength]byte{}
		copy(proofbytes[:], proof)
		b.logger.Trace("lottery", "won slot", slot)
		return &VrfOutputAndProof{
			output: outbytes,
			proof:  proofbytes,
		}, nil
	}

	return nil, nil
}

func getVRFOutput(header *types.Header) ([sr25519.VrfOutputLength]byte, error) {
	var bh *types.BabeHeader

	for _, d := range header.Digest {
		digest, err := types.DecodeDigestItem(d)
		if err != nil {
			continue
		}

		if digest.Type() == types.PreRuntimeDigestType {
			prd, ok := digest.(*types.PreRuntimeDigest)
			if !ok {
				continue
			}

			tbh := new(types.BabeHeader)
			err = tbh.Decode(prd.Data)
			if err != nil {
				continue
			}

			bh = tbh
			break
		}
	}

	if bh == nil {
		return [sr25519.VrfOutputLength]byte{}, fmt.Errorf("block %d: %w", header.Number, ErrNoBABEHeader)
	}

	return bh.VrfOutput, nil
}
