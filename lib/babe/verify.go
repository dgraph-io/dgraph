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
)

// EpochDescriptor contains the information needed to verify blocks within a BABE epoch
type EpochDescriptor struct {
	AuthorityData []*types.BABEAuthorityData
	Randomness    [RandomnessLength]byte
	Threshold     *big.Int
}

// VerificationManager assists the syncer in keeping track of what epoch is it currently syncing and verifying,
// as well as keeping track of the NextEpochDesciptor which is required to create a Verifier for an epoch.
type VerificationManager struct {
	epochDescriptors map[uint64]*EpochDescriptor
	blockState       BlockState
	// TODO: map of epochs to epoch length changes, for use in determining block epoch
	// TODO: BABE should signal to the verifier when the epoch changes and send it the current EpochDescriptor

	// current epoch information
	currentEpoch uint64
	verifier     *epochVerifier // TODO: may need to keep historical verifiers
}

// NewVerificationManager returns a new VerificationManager
func NewVerificationManager(blockState BlockState, currentEpoch uint64, descriptor *EpochDescriptor) (*VerificationManager, error) {
	if blockState == nil {
		return nil, ErrNilBlockState
	}

	verifier, err := newEpochVerifier(blockState, descriptor)
	if err != nil {
		return nil, err
	}

	desciptors := make(map[uint64]*EpochDescriptor)
	desciptors[currentEpoch] = descriptor

	return &VerificationManager{
		blockState:       blockState,
		epochDescriptors: desciptors,
		currentEpoch:     currentEpoch,
		verifier:         verifier,
	}, nil
}

// VerifyBlock verifies the given header with verifyAuthorshipRight.
func (v *VerificationManager) VerifyBlock(header *types.Header) (bool, error) {
	epoch, err := v.getBlockEpoch(header)
	if err != nil {
		return false, err
	}

	if epoch == v.currentEpoch {
		return v.verifier.verifyAuthorshipRight(header)
	}

	if v.epochDescriptors[epoch] == nil {
		// TODO: return an error here once updating of the verifier's epoch data is implemented
		return v.verifier.verifyAuthorshipRight(header)
	}

	verifier, err := newEpochVerifier(v.blockState, v.epochDescriptors[epoch])
	if err != nil {
		return false, err
	}

	return verifier.verifyAuthorshipRight(header)
}

// getBlockEpoch gets the epoch number using the provided block hash
func (v *VerificationManager) getBlockEpoch(header *types.Header) (epoch uint64, err error) {
	// get slot number to determine epoch number
	if len(header.Digest) == 0 {
		return 0, fmt.Errorf("chain head missing digest")
	}

	preDigestBytes := header.Digest[0]

	digestItem, err := types.DecodeDigestItem(preDigestBytes)
	if err != nil {
		return 0, err
	}

	preDigest, ok := digestItem.(*types.PreRuntimeDigest)
	if !ok {
		return 0, fmt.Errorf("first digest item is not pre-digest")
	}

	babeHeader := new(types.BabeHeader)
	err = babeHeader.Decode(preDigest.Data)
	if err != nil {
		return 0, fmt.Errorf("cannot decode babe header from pre-digest: %s", err)
	}

	slot := babeHeader.SlotNumber

	if slot != 0 {
		// epoch number = (slot - genesis slot) / epoch length
		epoch = (slot - 1) / 6 // TODO: use epoch length from babe or core config #762
	}

	return epoch, nil
}

// checkForConsensusDigest returns a consensus digest from the header, if it exists.
func checkForConsensusDigest(header *types.Header) (*types.ConsensusDigest, error) {
	// check if block header digest items exist
	if header.Digest == nil || len(header.Digest) == 0 {
		return nil, fmt.Errorf("header digest is not set")
	}

	// declare digest item
	var consensusDigest *types.ConsensusDigest

	// decode each digest item and check its type
	for _, digest := range header.Digest {
		item, err := types.DecodeDigestItem(digest)
		if err != nil {
			return nil, err
		}

		// check if digest item is consensus digest type
		if item.Type() == types.ConsensusDigestType {
			var ok bool
			consensusDigest, ok = item.(*types.ConsensusDigest)
			if ok {
				break
			}
		}
	}

	return consensusDigest, nil
}

// epochVerifier represents a BABE verifier for a specific epoch
type epochVerifier struct {
	blockState    BlockState
	authorityData []*types.BABEAuthorityData
	randomness    [RandomnessLength]byte
	threshold     *big.Int
}

// newEpochVerifier returns a Verifier for the epoch described by the given descriptor
func newEpochVerifier(blockState BlockState, descriptor *EpochDescriptor) (*epochVerifier, error) {
	if blockState == nil {
		return nil, ErrNilBlockState
	}

	return &epochVerifier{
		blockState:    blockState,
		authorityData: descriptor.AuthorityData,
		randomness:    descriptor.Randomness,
		threshold:     descriptor.Threshold,
	}, nil
}

// verifySlotWinner verifies the claim for a slot, given the BabeHeader for that slot.
func (b *epochVerifier) verifySlotWinner(slot uint64, header *types.BabeHeader) (bool, error) {
	if len(b.authorityData) <= int(header.BlockProducerIndex) {
		return false, fmt.Errorf("no authority data for index %d", header.BlockProducerIndex)
	}

	// check that vrf output is under threshold
	// if not, then return an error
	output := big.NewInt(0).SetBytes(header.VrfOutput[:])
	if output.Cmp(b.threshold) >= 0 {
		return false, fmt.Errorf("vrf output over threshold")
	}

	pub := b.authorityData[header.BlockProducerIndex].ID

	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	vrfInput := append(slotBytes, b.randomness[:]...)

	return pub.VrfVerify(vrfInput, header.VrfOutput[:], header.VrfProof[:])
}

// verifyAuthorshipRight verifies that the authority that produced a block was authorized to produce it.
func (b *epochVerifier) verifyAuthorshipRight(header *types.Header) (bool, error) {
	// header should have 2 digest items (possibly more in the future)
	// first item should be pre-digest, second should be seal
	if len(header.Digest) < 2 {
		return false, fmt.Errorf("block header is missing digest items")
	}

	// check for valid seal by verifying signature
	preDigestBytes := header.Digest[0]
	sealBytes := header.Digest[len(header.Digest)-1]

	digestItem, err := types.DecodeDigestItem(preDigestBytes)
	if err != nil {
		return false, err
	}

	preDigest, ok := digestItem.(*types.PreRuntimeDigest)
	if !ok {
		return false, fmt.Errorf("first digest item is not pre-digest")
	}

	digestItem, err = types.DecodeDigestItem(sealBytes)
	if err != nil {
		return false, err
	}

	seal, ok := digestItem.(*types.SealDigest)
	if !ok {
		return false, fmt.Errorf("last digest item is not seal")
	}

	babeHeader := new(types.BabeHeader)
	err = babeHeader.Decode(preDigest.Data)
	if err != nil {
		return false, fmt.Errorf("cannot decode babe header from pre-digest: %s", err)
	}

	if len(b.authorityData) <= int(babeHeader.BlockProducerIndex) {
		return false, fmt.Errorf("no authority data for index %d", babeHeader.BlockProducerIndex)
	}

	slot := babeHeader.SlotNumber

	authorPub := b.authorityData[babeHeader.BlockProducerIndex].ID
	// remove seal before verifying
	header.Digest = header.Digest[:len(header.Digest)-1]
	encHeader, err := header.Encode()
	if err != nil {
		return false, err
	}

	// verify that they are the slot winner
	ok, err = b.verifySlotWinner(slot, babeHeader)
	if err != nil {
		return false, err
	}

	if !ok {
		return false, ErrBadSlotClaim
	}

	// verify the seal is valid
	ok, err = authorPub.Verify(encHeader, seal.Data)
	if err != nil {
		return false, err
	}

	if !ok {
		return false, ErrBadSignature
	}

	// check if the producer has equivocated, ie. have they produced a conflicting block?
	hashes := b.blockState.GetAllBlocksAtDepth(header.ParentHash)

	for _, hash := range hashes {
		currentHeader, err := b.blockState.GetHeader(hash)
		if err != nil {
			continue
		}

		currentBlockProducerIndex, err := getBlockProducerIndex(currentHeader)
		if err != nil {
			continue
		}

		existingBlockProducerIndex := babeHeader.BlockProducerIndex

		if currentBlockProducerIndex == existingBlockProducerIndex && hash != header.Hash() {
			return false, ErrProducerEquivocated
		}
	}

	return true, nil
}

func getBlockProducerIndex(header *types.Header) (uint64, error) {
	if len(header.Digest) == 0 {
		return 0, fmt.Errorf("no digest provided")
	}

	preDigestBytes := header.Digest[0]

	digestItem, err := types.DecodeDigestItem(preDigestBytes)
	if err != nil {
		return 0, err
	}

	preDigest, ok := digestItem.(*types.PreRuntimeDigest)
	if !ok {
		return 0, err
	}

	babeHeader := new(types.BabeHeader)
	err = babeHeader.Decode(preDigest.Data)
	if err != nil {
		return 0, err
	}

	return babeHeader.BlockProducerIndex, nil
}
