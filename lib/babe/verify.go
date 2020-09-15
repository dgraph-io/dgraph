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
	"sync"

	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"

	"github.com/ChainSafe/gossamer/dot/types"
	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/runtime"
)

// VerificationManager assists the syncer in keeping track of what epoch is it currently syncing and verifying,
// as well as keeping track of the NextEpochDesciptor which is required to create a Verifier for an epoch.
type VerificationManager struct {
	lock        sync.Mutex
	descriptors map[common.Hash]*Descriptor
	branchNums  []int64                 // descending slice of branch numbers, for quicker access
	branches    map[int64][]common.Hash // a map of block numbers -> block hashes, needed to check what chain a block is on when verifying ie. what descriptor to use
	blockState  BlockState
	// TODO: BABE should signal to the verifier when the epoch changes and send it the current Descriptor
	// TODO: when a block is finalized, update branches/descriptors to delete any block data w/ number < finalized block number

	// current epoch information
	verifier *verifier // current chain head verifier TODO: remove, or keep historical verifiers
}

// NewVerificationManagerFromRuntime returns a new VerificationManager
func NewVerificationManagerFromRuntime(blockState BlockState, rt *runtime.Runtime) (*VerificationManager, error) {
	descriptor, err := descriptorFromRuntime(rt)
	if err != nil {
		return nil, err
	}

	return NewVerificationManager(blockState, descriptor)
}

// NewVerificationManager returns a new NewVerificationManager
func NewVerificationManager(blockState BlockState, descriptor *Descriptor) (*VerificationManager, error) {
	if blockState == nil {
		return nil, ErrNilBlockState
	}

	descriptors := make(map[common.Hash]*Descriptor)
	branches := make(map[int64][]common.Hash)

	// TODO: save VerificationManager in database when node shuts down, reload upon startup
	descriptors[blockState.GenesisHash()] = descriptor
	branches[0] = []common.Hash{blockState.GenesisHash()}

	verifier, err := newVerifier(blockState, descriptor)
	if err != nil {
		return nil, err
	}

	return &VerificationManager{
		blockState:  blockState,
		descriptors: descriptors,
		branchNums:  []int64{0},
		branches:    branches,
		verifier:    verifier,
	}, nil
}

// SetRuntimeChangeAtBlock sets a runtime change at the given block
// Blocks that are descendants of this block will be verified using the given runtime
func (v *VerificationManager) SetRuntimeChangeAtBlock(header *types.Header, rt *runtime.Runtime) error {
	descriptor, err := descriptorFromRuntime(rt)
	if err != nil {
		return err
	}

	v.setDescriptorChangeAtBlock(header, descriptor)
	return nil
}

// SetAuthorityChangeAtBlock sets an authority change at the given block and all descendants of that block
func (v *VerificationManager) SetAuthorityChangeAtBlock(header *types.Header, authorities []*types.Authority) {
	v.lock.Lock()

	num := header.Number.Int64()
	desc := &Descriptor{
		AuthorityData: authorities,
	}

	// find branch that header is on, set randomness and threshold to latest known on that chain
	for _, bn := range v.branchNums {
		if num >= bn {
			for _, hash := range v.branches[bn] {
				// found most recent ancestor descriptor
				if is, _ := v.blockState.IsDescendantOf(hash, header.Hash()); is {
					desc.Randomness = v.descriptors[hash].Randomness
					desc.Threshold = v.descriptors[hash].Threshold
					break
				}
			}
		}
	}

	v.lock.Unlock()
	v.setDescriptorChangeAtBlock(header, desc)
}

func (v *VerificationManager) setDescriptorChangeAtBlock(header *types.Header, descriptor *Descriptor) {
	v.lock.Lock()
	defer v.lock.Unlock()

	num := header.Number.Int64()
	if v.branches[num] == nil {
		v.branches[num] = []common.Hash{}
	}

	for i, bn := range v.branchNums {
		// number already stored, don't need to add to branch number slice
		if bn == num {
			break
		}

		if num < bn || i == len(v.branchNums)-1 {
			pre := make([]int64, len(v.branchNums[:i]))
			copy(pre, v.branchNums[:i])
			post := make([]int64, len(v.branchNums[i:]))
			copy(post, v.branchNums[i:])
			v.branchNums = append(append(pre, num), post...)
			break
		}
	}

	v.branches[num] = append(v.branches[num], header.Hash())
	v.descriptors[header.Hash()] = descriptor
}

// VerifyBlock verifies the given header with verifyAuthorshipRight.
func (v *VerificationManager) VerifyBlock(header *types.Header) (bool, error) {
	var (
		desc     *Descriptor
		verifier *verifier
		err      error
	)

	// iterate through descending list of blocktree branches
	for _, n := range v.branchNums {

		if header.Number.Int64() > n {
			// this is a func so that locking can be deferred to whenever it exits
			func() {
				v.lock.Lock()
				defer v.lock.Unlock()

				// get block hashes at most recent branch
				brs := v.branches[n]

				// check which branch this block is a descendant of
				for _, hash := range brs {
					// can only compare ParentHash, since current block isn't in blockState yet
					if is, _ := v.blockState.IsDescendantOf(hash, header.ParentHash); is {
						desc = v.descriptors[hash]
						break
					}
				}
			}()

			if desc != nil {
				break
			}
		}
	}

	if desc == nil {
		// we didn't find any data for the chain that this block is on, try to verify it with the current verifier anyways
		verifier = v.verifier
	} else {
		verifier, err = newVerifier(v.blockState, desc)
		if err != nil {
			return false, err
		}
	}

	return verifier.verifyAuthorshipRight(header)
}

func descriptorFromRuntime(rt *runtime.Runtime) (*Descriptor, error) {
	cfg, err := rt.BabeConfiguration()
	if err != nil {
		return nil, err
	}

	auths, err := types.BABEAuthorityRawToAuthority(cfg.GenesisAuthorities)
	if err != nil {
		return nil, err
	}

	threshold, err := CalculateThreshold(cfg.C1, cfg.C2, len(auths))
	if err != nil {
		return nil, err
	}

	return &Descriptor{
		AuthorityData: auths,
		Randomness:    cfg.Randomness,
		Threshold:     threshold,
	}, nil
}

// verifier is a BABE verifier for a specific authority set, randomness, and threshold
type verifier struct {
	blockState    BlockState
	authorityData []*types.Authority
	randomness    [types.RandomnessLength]byte
	threshold     *big.Int
}

// newVerifier returns a Verifier for the epoch described by the given descriptor
func newVerifier(blockState BlockState, descriptor *Descriptor) (*verifier, error) {
	if blockState == nil {
		return nil, ErrNilBlockState
	}

	return &verifier{
		blockState:    blockState,
		authorityData: descriptor.AuthorityData,
		randomness:    descriptor.Randomness,
		threshold:     descriptor.Threshold,
	}, nil
}

// verifySlotWinner verifies the claim for a slot, given the BabeHeader for that slot.
func (b *verifier) verifySlotWinner(slot uint64, header *types.BabeHeader) (bool, error) {
	if len(b.authorityData) <= int(header.BlockProducerIndex) {
		return false, fmt.Errorf("no authority data for index %d", header.BlockProducerIndex)
	}

	// check that vrf output is under threshold
	// if not, then return an error
	output := big.NewInt(0).SetBytes(header.VrfOutput[:])
	if output.Cmp(b.threshold) >= 0 {
		return false, fmt.Errorf("vrf output over threshold")
	}

	pub := b.authorityData[header.BlockProducerIndex].Key

	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	vrfInput := append(slotBytes, b.randomness[:]...)

	sr25519PK, err := sr25519.NewPublicKey(pub.Encode())
	if err != nil {
		return false, err
	}

	return sr25519PK.VrfVerify(vrfInput, header.VrfOutput[:], header.VrfProof[:])
}

// verifyAuthorshipRight verifies that the authority that produced a block was authorized to produce it.
func (b *verifier) verifyAuthorshipRight(header *types.Header) (bool, error) {
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

	authorPub := b.authorityData[babeHeader.BlockProducerIndex].Key
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
