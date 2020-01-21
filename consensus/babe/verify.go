package babe

import (
	"encoding/binary"
	"fmt"

	"github.com/ChainSafe/gossamer/core/types"
)

func (b *Session) verifySlotWinner(slot uint64, header *BabeHeader) (bool, error) {
	if len(b.authorityData) <= int(header.BlockProducerIndex) {
		return false, fmt.Errorf("no authority data for index %d", header.BlockProducerIndex)
	}

	pub := b.authorityData[header.BlockProducerIndex].id

	slotBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(slotBytes, slot)
	vrfInput := append(slotBytes, b.config.Randomness)

	return pub.VrfVerify(vrfInput, header.VrfOutput[:], header.VrfProof[:])
}

func (b *Session) verifyAuthorshipRight(slot uint64, header *types.Header) (bool, error) {
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

	babeHeader := new(BabeHeader)
	err = babeHeader.Decode(preDigest.Data)
	if err != nil {
		return false, fmt.Errorf("cannot decode babe header from pre-digest: %s", err)
	}

	if len(b.authorityData) <= int(babeHeader.BlockProducerIndex) {
		return false, fmt.Errorf("no authority data for index %d", babeHeader.BlockProducerIndex)
	}

	authorPub := b.authorityData[babeHeader.BlockProducerIndex].id
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
		return false, fmt.Errorf("could not verify slot claim")
	}

	// verify the seal is valid
	ok, err = authorPub.Verify(encHeader, seal.Data)
	if err != nil {
		return false, err
	}

	if !ok {
		return false, fmt.Errorf("could not verify signature")
	}

	// TODO: check if the producer has equivocated, ie. have they produced a conflicting block?
	return true, nil
}
