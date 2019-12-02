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
	"io"
	"math"
	"net/http"
	"os"
	"reflect"
	"testing"

	wasm "github.com/wasmerio/go-ext-wasm/wasmer"
)

const pageSize = 65536

// struct to hold data for a round of tests
type testHolder struct {
	offset uint32
	tests  []testSet
}

// struct for data used in allocate tests
type allocateTest struct {
	size uint32
}

// struct for data used in free tests
type freeTest struct {
	ptr uint32
}

// struct to hold data used for expected allocator state
type allocatorState struct {
	bumper    uint32
	heads     [HeadsQty]uint32
	ptrOffset uint32
	totalSize uint32
}

// struct to hold set of test (allocateTest or freeTest), expected output (return result of test (if any))
//  state, expected state of the allocator after given test is run
type testSet struct {
	test   interface{}
	output interface{}
	state  allocatorState
}

// allocate 1 byte test
var allocate1ByteTest = []testSet{
	{test: &allocateTest{size: 1},
		output: uint32(8),
		state: allocatorState{bumper: 16,
			totalSize: 16}},
}

// allocate 1 byte test with allocator memory offset
var allocate1ByteTestWithOffset = []testSet{
	{test: &allocateTest{size: 1},
		output: uint32(24),
		state: allocatorState{bumper: 16,
			ptrOffset: 16,
			totalSize: 16}},
}

// allocate memory 3 times and confirm expected state of allocator
var allocatorShouldIncrementPointers = []testSet{
	{test: &allocateTest{size: 1},
		output: uint32(8),
		state: allocatorState{bumper: 16,
			totalSize: 16}},
	{test: &allocateTest{size: 9},
		output: uint32(8 + 16),
		state: allocatorState{bumper: 40,
			totalSize: 40}},
	{test: &allocateTest{size: 1},
		output: uint32(8 + 16 + 24),
		state: allocatorState{bumper: 56,
			totalSize: 56}},
}

// allocate memory twice and free the second allocation
var allocateFreeTest = []testSet{
	{test: &allocateTest{size: 1},
		output: uint32(8),
		state: allocatorState{bumper: 16,
			totalSize: 16}},
	{test: &allocateTest{size: 9},
		output: uint32(8 + 16),
		state: allocatorState{bumper: 40,
			totalSize: 40}},
	{test: &freeTest{ptr: 24}, // address of second allocation
		state: allocatorState{bumper: 40,
			heads:     [22]uint32{0, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			totalSize: 16}},
}

// allocate free and reallocate with memory offset
var allocateDeallocateReallocateWithOffset = []testSet{
	{test: &allocateTest{size: 1},
		output: uint32(24),
		state: allocatorState{bumper: 16,
			ptrOffset: 16,
			totalSize: 16}},
	{test: &allocateTest{size: 9},
		output: uint32(40),
		state: allocatorState{bumper: 40,
			ptrOffset: 16,
			totalSize: 40}},
	{test: &freeTest{ptr: 40}, // address of second allocation
		state: allocatorState{bumper: 40,
			heads:     [22]uint32{0, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ptrOffset: 16,
			totalSize: 16}},
	{test: &allocateTest{size: 9},
		output: uint32(40),
		state: allocatorState{bumper: 40,
			ptrOffset: 16,
			totalSize: 40}},
}

var allocateShouldBuildFreeList = []testSet{
	// allocate 8 bytes
	{test: &allocateTest{size: 8},
		output: uint32(8),
		state: allocatorState{bumper: 16,
			totalSize: 16}},
	// allocate 8 bytes
	{test: &allocateTest{size: 8},
		output: uint32(24),
		state: allocatorState{bumper: 32,
			totalSize: 32}},
	// allocate 8 bytes
	{test: &allocateTest{size: 8},
		output: uint32(40),
		state: allocatorState{bumper: 48,
			totalSize: 48}},
	// free first allocation
	{test: &freeTest{ptr: 8}, // address of first allocation
		state: allocatorState{bumper: 48,
			totalSize: 32}},
	// free second allocation
	{test: &freeTest{ptr: 24}, // address of second allocation
		state: allocatorState{bumper: 48,
			heads:     [22]uint32{16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			totalSize: 16}},
	// free third allocation
	{test: &freeTest{ptr: 40}, // address of third allocation
		state: allocatorState{bumper: 48,
			heads:     [22]uint32{32, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			totalSize: 0}},
	// allocate 8 bytes
	{test: &allocateTest{size: 8},
		output: uint32(40),
		state: allocatorState{bumper: 48,
			heads:     [22]uint32{16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			totalSize: 16}},
}

// allocate 9 byte test with allocator memory offset
var allocateCorrectlyWithOffset = []testSet{
	{test: &allocateTest{size: 9},
		output: uint32(16),
		state: allocatorState{bumper: 24,
			ptrOffset: 8,
			totalSize: 24}},
}

// allocate 42 bytes with offset, then free should leave total size 0
var heapShouldBeZeroAfterFreeWithOffset = []testSet{
	{test: &allocateTest{size: 42},
		output: uint32(24),
		state: allocatorState{bumper: 72,
			ptrOffset: 16,
			totalSize: 72}},

	{test: &freeTest{ptr: 24},
		state: allocatorState{bumper: 72,
			ptrOffset: 16,
			totalSize: 0}},
}

var heapShouldBeZeroAfterFreeWithOffsetFiveTimes = []testSet{
	// first alloc
	{test: &allocateTest{size: 42},
		output: uint32(32),
		state: allocatorState{bumper: 72,
			ptrOffset: 24,
			totalSize: 72}},
	// first free
	{test: &freeTest{ptr: 32},
		state: allocatorState{bumper: 72,
			ptrOffset: 24,
			totalSize: 0}},
	// second alloc
	{test: &allocateTest{size: 42},
		output: uint32(104),
		state: allocatorState{bumper: 144,
			ptrOffset: 24,
			totalSize: 72}},
	// second free
	{test: &freeTest{ptr: 104},
		state: allocatorState{bumper: 144,
			heads:     [22]uint32{0, 0, 0, 72, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ptrOffset: 24,
			totalSize: 0}},
	// third alloc
	{test: &allocateTest{size: 42},
		output: uint32(104),
		state: allocatorState{bumper: 144,
			ptrOffset: 24,
			totalSize: 72}},
	// third free
	{test: &freeTest{ptr: 104},
		state: allocatorState{bumper: 144,
			heads:     [22]uint32{0, 0, 0, 72, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ptrOffset: 24,
			totalSize: 0}},
	// forth alloc
	{test: &allocateTest{size: 42},
		output: uint32(104),
		state: allocatorState{bumper: 144,
			ptrOffset: 24,
			totalSize: 72}},
	// forth free
	{test: &freeTest{ptr: 104},
		state: allocatorState{bumper: 144,
			heads:     [22]uint32{0, 0, 0, 72, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ptrOffset: 24,
			totalSize: 0}},
	// fifth alloc
	{test: &allocateTest{size: 42},
		output: uint32(104),
		state: allocatorState{bumper: 144,
			ptrOffset: 24,
			totalSize: 72}},
	// fifth free
	{test: &freeTest{ptr: 104},
		state: allocatorState{bumper: 144,
			heads:     [22]uint32{0, 0, 0, 72, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			ptrOffset: 24,
			totalSize: 0}},
}

// all tests to be run
var allTests = []testHolder{
	{offset: 0, tests: allocate1ByteTest},
	{offset: 13, tests: allocate1ByteTestWithOffset},
	{offset: 0, tests: allocatorShouldIncrementPointers},
	{offset: 0, tests: allocateFreeTest},
	{offset: 13, tests: allocateDeallocateReallocateWithOffset},
	{offset: 0, tests: allocateShouldBuildFreeList},
	{offset: 1, tests: allocateCorrectlyWithOffset},
	{offset: 13, tests: heapShouldBeZeroAfterFreeWithOffset},
	{offset: 19, tests: heapShouldBeZeroAfterFreeWithOffsetFiveTimes},
}

const SimpleWasmFP = "simple.wasm"
const SimpleWasmURL = "https://github.com//wasmerio/go-ext-wasm/blob/master/wasmer/test/testdata/examples/simple.wasm?raw=true"

// utility function to create a wasm.Memory instance for testing
//   it checks if simple.wasm has been downloaded from git repo, if not if fetchs it for building the test wasm blob
func NewWasmMemory() (*wasm.Memory, error) {
	// check if file already downloaded
	if _, err := os.Stat(SimpleWasmFP); err != nil {
		if os.IsNotExist(err) {
			// file not found, so load if from git repo
			out, err := os.Create(SimpleWasmFP)
			if err != nil {
				return nil, err
			}
			defer out.Close()

			resp, err := http.Get(SimpleWasmURL)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			_, err = io.Copy(out, resp.Body)
			if err != nil {
				return nil, err
			}
		}
	}
	// Reads the WebAssembly simple.wasm mock wasm blob.
	//  note the wasm blob can be any valid wasm blob that will create a new wasm instance in this case we're using
	//  test blob from https://github.com/wasmerio/go-ext-wasm/tree/master/wasmer/test/testdata/examples/simple.wasm
	bytes, err := wasm.ReadBytes(SimpleWasmFP)
	if err != nil {
		return nil, err
	}
	instance, err := wasm.NewInstance(bytes)
	if err != nil {
		return nil, err
	}

	return &instance.Memory, nil
}

// iterates allTests and runs tests on them based on data contained in
//  test holder
func TestAllocator(t *testing.T) {
	for _, test := range allTests {
		mem, err := NewWasmMemory()
		if err != nil {
			t.Fatal(err)
		}
		allocator := NewAllocator(mem, test.offset)

		for _, theTest := range test.tests {
			switch v := theTest.test.(type) {
			case *allocateTest:
				result, err1 :=
					allocator.Allocate(v.size)
				if err1 != nil {
					t.Fatal(err1)
				}

				compareState(*allocator, theTest.state, result, theTest.output, t)

			case *freeTest:
				err := allocator.Deallocate(v.ptr)
				if err != nil {
					t.Fatal(err)
				}
				compareState(*allocator, theTest.state, nil, theTest.output, t)
			}
		}
	}
}

// compare test results to expected results and fail test if differences are found
func compareState(allocator FreeingBumpHeapAllocator, state allocatorState, result interface{}, output interface{}, t *testing.T) {
	if !reflect.DeepEqual(allocator.bumper, state.bumper) {
		t.Errorf("Fail: got %v expected %v", allocator.bumper, state.bumper)
	}
	if !reflect.DeepEqual(allocator.heads, state.heads) {
		t.Errorf("Fail: got %v expected %v", allocator.heads, state.heads)
	}
	if !reflect.DeepEqual(allocator.ptrOffset, state.ptrOffset) {
		t.Errorf("Fail: got %v expected %v", allocator.ptrOffset, state.ptrOffset)
	}
	if !reflect.DeepEqual(allocator.TotalSize, state.totalSize) {
		t.Errorf("Fail: got %v expected %v", allocator.TotalSize, state.totalSize)
	}
	if !reflect.DeepEqual(result, output) {
		t.Errorf("Fail: got %v expected %v", result, output)
	}
}

// test that allocator should no allocate memory if the allocate
//  request is larger than current size
func TestShouldNotAllocateIfTooLarge(t *testing.T) {
	// given
	mem, err := NewWasmMemory()
	if err != nil {
		t.Fatal(err)
	}
	currentSize := mem.Length()

	fbha := NewAllocator(mem, 0)

	// when
	_, err = fbha.Allocate(currentSize + 1)

	// then expect error since trying to over Allocate
	if err == nil {
		t.Error("Error, expected out of space error, but didn't get one.")
	}
	if err != nil && err.Error() != "allocator out of space" {
		t.Errorf("Error: got unexpected error: %v", err.Error())
	}
}

// test that the allocator should not allocate memory if
//  it's already full
func TestShouldNotAllocateIfFull(t *testing.T) {
	// given
	mem, err := NewWasmMemory()
	if err != nil {
		t.Fatal(err)
	}
	currentSize := mem.Length()
	fbha := NewAllocator(mem, 0)

	ptr1, err := fbha.Allocate((currentSize / 2) - 8)
	if err != nil {
		t.Fatal(err)
	}
	if ptr1 != 8 {
		t.Errorf("Expected value of 8")
	}

	// when
	_, err = fbha.Allocate(currentSize / 2)

	// then
	// there is no room after half currentSize including it's 8 byte prefix, so error
	if err == nil {
		t.Error("Error, expected out of space error, but didn't get one.")
	}
	if err != nil && err.Error() != "allocator out of space" {
		t.Errorf("Error: got unexpected error: %v", err.Error())
	}

}

// test to confirm that allocator can allocate the MaxPossibleAllocation
func TestShouldAllocateMaxPossibleAllocationSize(t *testing.T) {
	// given, grow heap memory so that we have at least MaxPossibleAllocation available
	mem, err := NewWasmMemory()
	if err != nil {
		t.Fatal(err)
	}
	pagesNeeded := (MaxPossibleAllocation / pageSize) - (mem.Length() / pageSize) + 1
	err = mem.Grow(pagesNeeded)
	if err != nil {
		t.Error(err)
	}
	fbha := NewAllocator(mem, 0)

	// when
	ptr1, err := fbha.Allocate(MaxPossibleAllocation)
	if err != nil {
		t.Error(err)
	}

	//then
	t.Log("ptr1", ptr1)
	if ptr1 != 8 {
		t.Errorf("Expected value of 8")
	}
}

// test that allocator should not allocate memory if request is too large
func TestShouldNotAllocateIfRequestSizeTooLarge(t *testing.T) {
	// given
	mem, err := NewWasmMemory()
	if err != nil {
		t.Fatal(err)
	}
	fbha := NewAllocator(mem, 0)

	// when
	_, err = fbha.Allocate(MaxPossibleAllocation + 1)

	// then
	if err != nil {
		if err.Error() != "size to large" {
			t.Error("Didn't get expected error")
		}
	} else {
		t.Error("Error: Didn't get error but expected one.")
	}
}

// test to write Uint32 to LE correctly
func TestShouldWriteU32CorrectlyIntoLe(t *testing.T) {
	// NOTE: we used the go's binary.LittleEndianPutUint32 function
	//  so this test isn't necessary, but is included for completeness

	//given
	heap := make([]byte, 5)

	// when
	binary.LittleEndian.PutUint32(heap, 1)

	//then
	if !reflect.DeepEqual(heap, []byte{1, 0, 0, 0, 0}) {
		t.Error("Error Write U32 to LE")
	}
}

// test to write MaxUint32 to LE correctly
func TestShouldWriteU32MaxCorrectlyIntoLe(t *testing.T) {
	// NOTE: we used the go's binary.LittleEndianPutUint32 function
	//  so this test isn't necessary, but is included for completeness

	//given
	heap := make([]byte, 5)

	// when
	binary.LittleEndian.PutUint32(heap, math.MaxUint32)

	//then
	if !reflect.DeepEqual(heap, []byte{255, 255, 255, 255, 0}) {
		t.Error("Error Write U32 MAX to LE")
	}
}

// test that getItemSizeFromIndex method gets expected item size from index
func TestShouldGetItemFromIndex(t *testing.T) {
	// given
	index := uint(0)

	// when
	itemSize := getItemSizeFromIndex(index)

	//then
	t.Log("[TestShouldGetItemFromIndex]", "item_size", itemSize)
	if itemSize != 8 {
		t.Error("item_size should be 8, got item_size:", itemSize)
	}
}

// that that getItemSizeFromIndex method gets expected item size from index
//  max index possition
func TestShouldGetMaxFromIndex(t *testing.T) {
	// given
	index := uint(21)

	// when
	itemSize := getItemSizeFromIndex(index)

	//then
	t.Log("[TestShouldGetMaxFromIndex]", "item_size", itemSize)
	if itemSize != MaxPossibleAllocation {
		t.Errorf("item_size should be %d, got item_size: %d", MaxPossibleAllocation, itemSize)
	}
}
