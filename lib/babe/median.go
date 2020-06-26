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
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"
)

// slotTail is the number of blocks needed for us to run the median algorithm. in the spec, it's arbitrarily set to 1200.
// TODO: will need to update this once finished simple slot time algo testing
var slotTail = uint64(12)

// returns the estimated current slot number, without median algorithm
func (b *Service) estimateCurrentSlot() (uint64, error) {
	// estimate slot of highest block we've received
	head := b.blockState.BestBlockHash()

	slot, err := b.blockState.GetSlotForBlock(head)
	if err != nil {
		return 0, fmt.Errorf("cannot get slot for head of chain: %s", err)
	}

	// find arrival time of chain head
	// note: this assumes that the block arrived within the slot it was produced, may be off
	arrivalTime, err := b.blockState.GetArrivalTime(head)
	if err != nil {
		return 0, fmt.Errorf("cannot get arrival time for head of chain: %s", err)
	}

	// use slot duration to count up
	for {
		at := time.Unix(int64(arrivalTime), 0)

		if time.Since(at) <= b.slotDuration() {
			return slot, nil
		}

		// increment slot, slot time
		arrivalTime += b.config.SlotDuration
		slot++
	}
}

// getCurrentSlot estimates the current slot, then uses the slotTime algorithm to determine the exact slot
func (b *Service) getCurrentSlot() (uint64, error) {
	estimate, err := b.estimateCurrentSlot()
	if err != nil {
		return 0, err
	}

	for {
		slotTime, err := b.slotTime(estimate, slotTail)
		if err != nil {
			return 0, err
		}

		st := time.Unix(int64(slotTime), 0)

		if time.Since(st) <= b.slotDuration() {
			return estimate, nil
		}

		estimate++
	}
}

// slotTime calculates the slot time in the form of seconds since the unix epoch
// for a given slot in seconds, returns 0 and an error if it can't be calculated
func (b *Service) slotTime(slot uint64, slotTail uint64) (uint64, error) {
	var at []uint64

	head := b.blockState.BestBlockHash()
	tail := new(big.Int).SetUint64(slotTail)

	deepestBlock, err := b.blockState.GetHeader(head)
	if err != nil {
		return 0, fmt.Errorf("cannot get deepest block: %s", err)
	}

	// check to make sure we have enough blocks before the deepest block to accurately calculate slot time
	if deepestBlock.Number.Cmp(tail) == -1 {
		return 0, fmt.Errorf("cannot calculate slot time: deepest block number %d less than or equal to slot tail %d", deepestBlock.Number, tail)
	}

	startNumber := tail.Sub(deepestBlock.Number, tail)

	start, err := b.blockState.GetBlockByNumber(startNumber)
	if err != nil {
		return 0, err
	}

	sd := uint64(b.slotDuration().Seconds())

	var currSlot uint64
	var so uint64
	var arrivalTime uint64

	subchain, err := b.blockState.SubChain(start.Header.Hash(), deepestBlock.Hash())
	if err != nil {
		return 0, err
	}

	for _, hash := range subchain {
		currSlot, err = b.blockState.GetSlotForBlock(hash)
		if err != nil {
			return 0, err
		}

		so, err = slotOffset(currSlot, slot)
		if err != nil {
			return 0, err
		}

		arrivalTime, err = b.blockState.GetArrivalTime(hash)
		if err != nil {
			return 0, err
		}

		st := arrivalTime + (so * sd)
		at = append(at, st)
	}

	st, err := median(at)
	if err != nil {
		return 0, err
	}
	return st, nil
}

// median calculates the median of a uint64 slice
// @TODO: Implement quickselect as an alternative to this.
func median(l []uint64) (uint64, error) {
	// sort the list
	sort.Slice(l, func(i, j int) bool { return l[i] < l[j] })

	m := len(l)
	med := uint64(0)
	if m == 0 {
		return 0, errors.New("arrival times list is empty! ")
	} else if m%2 == 0 {
		med = (l[(m/2)-1] + l[(m/2)+1]) / 2
	} else {
		med = l[m/2]
	}
	return med, nil
}

// slotOffset returns the number of slots between slot
func slotOffset(start uint64, end uint64) (uint64, error) {
	os := end - start
	if end < start {
		return 0, errors.New("cannot have negative Slot Offset")
	}
	return os, nil
}
