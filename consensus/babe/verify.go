package babe

import (
	"encoding/binary"
	"fmt"
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
