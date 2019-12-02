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

package runtime

import (
	"encoding/binary"
	"errors"
	"math/bits"

	log "github.com/ChainSafe/log15"
	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
)

// This module implements a freeing-bump allocator
// see more details at https://github.com/paritytech/substrate/issues/1615

// The pointers need to be aligned to 8 bytes
const alignment uint32 = 8
const HeadsQty = 22
const MaxPossibleAllocation = 16777216 // 2^24 bytes

type FreeingBumpHeapAllocator struct {
	bumper      uint32
	heads       [HeadsQty]uint32
	heap        *wasm.Memory
	maxHeapSize uint32
	ptrOffset   uint32
	TotalSize   uint32
}

// Creates a new allocation heap which follows a freeing-bump strategy.
// The maximum size which can be allocated at once is 16 MiB.
//
// # Arguments
//
// * `mem` - A wasm.Memory to the available memory which is
//   used as the heap.
//
// * `ptrOffset` - The pointers returned by `Allocate()` start from this
//   offset on. The pointer offset needs to be aligned to a multiple of 8,
//   hence a padding might be added to align `ptrOffset` properly.
//
// * returns a pointer to an initilized FreeingBumpHeapAllocator
func NewAllocator(mem *wasm.Memory, ptrOffset uint32) *FreeingBumpHeapAllocator {
	fbha := new(FreeingBumpHeapAllocator)
	currentSize := mem.Length()
	// we don't include offset memory in the heap
	heapSize := currentSize - ptrOffset

	padding := ptrOffset % alignment
	if padding != 0 {
		ptrOffset += alignment - padding
	}

	fbha.bumper = 0
	fbha.heap = mem
	fbha.maxHeapSize = heapSize
	fbha.ptrOffset = ptrOffset
	fbha.TotalSize = 0

	return fbha
}

// Allocate determines if there is space available in WASM heap to grow the heap by 'size'.  If there is space
//   available it grows the heap to fit give 'size'.  The heap grows is chunks of Powers of 2, so the growth becomes
//   the next highest power of 2 of the requested size.
func (fbha *FreeingBumpHeapAllocator) Allocate(size uint32) (uint32, error) {
	// test for space allocation
	if size > MaxPossibleAllocation {
		err := errors.New("size to large")
		return 0, err
	}
	itemSize := nextPowerOf2GT8(size)

	if (itemSize + 8 + fbha.TotalSize) > fbha.maxHeapSize {
		err := errors.New("allocator out of space")
		return 0, err
	}

	// get pointer based on list_index
	listIndex := bits.TrailingZeros32(itemSize) - 3

	var ptr uint32
	if fbha.heads[listIndex] != 0 {
		// Something from the free list
		item := fbha.heads[listIndex]
		fourBytes := fbha.getHeap4bytes(item)
		fbha.heads[listIndex] = binary.LittleEndian.Uint32(fourBytes)
		ptr = item + 8
	} else {
		// Nothing te be freed. Bump.
		ptr = fbha.bump(itemSize+8) + 8
	}

	// write "header" for allocated memory to heap
	for i := uint32(1); i <= 8; i++ {
		fbha.setHeap(ptr-i, 255)
	}
	fbha.setHeap(ptr-8, uint8(listIndex))
	fbha.TotalSize = fbha.TotalSize + itemSize + 8
	log.Trace("[Allocate]", "heap_size after allocation", fbha.TotalSize)
	return fbha.ptrOffset + ptr, nil
}

// Deallocate deallocates the memory located at pointer address
func (fbha *FreeingBumpHeapAllocator) Deallocate(pointer uint32) error {
	ptr := pointer - fbha.ptrOffset
	if ptr < 8 {
		return errors.New("invalid pointer for deallocation")
	}
	log.Trace("[Deallocate]", "ptr", ptr)
	listIndex := fbha.getHeapByte(ptr - 8)

	// update heads array, and heap "header"
	tail := fbha.heads[listIndex]
	fbha.heads[listIndex] = ptr - 8

	bTail := make([]byte, 4)
	binary.LittleEndian.PutUint32(bTail, tail)
	fbha.setHeap4bytes(ptr-8, bTail)

	// update heap total size
	itemSize := getItemSizeFromIndex(uint(listIndex))
	fbha.TotalSize = fbha.TotalSize - uint32(itemSize+8)
	log.Trace("[Deallocate]", "heap total_size after Deallocate", fbha.TotalSize)

	return nil
}

func (fbha *FreeingBumpHeapAllocator) bump(qty uint32) uint32 {
	res := fbha.bumper
	fbha.bumper += qty
	return res
}

func (fbha *FreeingBumpHeapAllocator) setHeap(ptr uint32, value uint8) {
	fbha.heap.Data()[fbha.ptrOffset+ptr] = value
}

func (fbha *FreeingBumpHeapAllocator) setHeap4bytes(ptr uint32, value []byte) {
	copy(fbha.heap.Data()[fbha.ptrOffset+ptr:fbha.ptrOffset+ptr+4], value)
}
func (fbha *FreeingBumpHeapAllocator) getHeap4bytes(ptr uint32) []byte {
	return fbha.heap.Data()[fbha.ptrOffset+ptr : fbha.ptrOffset+ptr+4]
}

func (fbha *FreeingBumpHeapAllocator) getHeapByte(ptr uint32) byte {
	return fbha.heap.Data()[fbha.ptrOffset+ptr]
}

func getItemSizeFromIndex(index uint) uint {
	// we shift 1 by three places since the first possible item size is 8
	return 1 << 3 << index
}

func nextPowerOf2GT8(v uint32) uint32 {
	if v < 8 {
		return 8
	}
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return v
}
