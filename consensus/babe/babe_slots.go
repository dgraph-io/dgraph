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
	"math/big"
	"sort"

	"github.com/ChainSafe/gossamer/core/blocktree"
)

// slotTime calculates the slot time in the form of miliseconds since the unix epoch
// for a given slot in miliseconds, returns 0 and an error if it can't be calculated
func (b *Session) slotTime(slot uint64, bt *blocktree.BlockTree, slotTail uint64) (uint64, error) {
	var at []uint64
	dl := bt.DeepestBlock()
	bn := new(big.Int).SetUint64(slotTail)
	nf := bn.Sub(dl.Header.Number, bn)
	// check to make sure we have enough blocks before the deepest block to accurately calculate slot time
	if dl.Header.Number.Cmp(bn) <= 0 {
		return 0, errors.New("Cannot calculate slot time, deepest leaf block number less than or equal to Slot Tail")
	}
	s := bt.GetBlockFromBlockNumber(nf)
	err := b.configurationFromRuntime()
	sd := b.config.SlotDuration
	if err != nil {
		return 0, err
	}
	for _, block := range bt.SubBlockchain(s.Header.Number, dl.Header.Number) {
		so, offsetErr := slotOffset(bt.ComputeSlotForBlock(block, sd), slot)
		if offsetErr != nil {
			return 0, err
		}
		st := block.GetBlockArrivalTime() + (so * sd)
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
		return 0, errors.New("Arrival times list is empty!")
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
		return 0, errors.New("Cannot have negative Slot Offset!")
	}
	return os, nil
}
