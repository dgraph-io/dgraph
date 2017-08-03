// Package bp128 implements SIMD-BP128 integer encoding and decoding.
// It requires an x86_64/AMD64 CPU that supports SSE2 instructions.
//
// For more details on SIMD-BP128 algorithm see "Decoding billions of
// integers per second through vectorization" by Daniel Lemire, Leonid
// Boytsov, and Nathan Kurz at http://arxiv.org/pdf/1209.2137
//
// For the original C++ implementation visit
// https://github.com/lemire/SIMDCompressionAndIntersection.
package bp128

import (
	"encoding/binary"
	"unsafe"
)

const (
	// addrAlignment is the memory address alignment of
	// input and output data. This is required by some
	// SIMD instructions.
	addrAlignment = 16

	// blockSize is the number of integers per block. Each
	// block address must be aligned at 16-byte boundaries.
	blockSize = 128
	numBlocks = addrAlignment

	// clusterSize represents the number of integers
	// per cluster. Each cluster is composed of 16 blocks.
	clusterSize = blockSize * numBlocks
)

var fdpack64 = []func(in *uint64, out *byte, seed *uint64){
	dpack64_0, dpack64_1, dpack64_2, dpack64_3, dpack64_4, dpack64_5,
	dpack64_6, dpack64_7, dpack64_8, dpack64_9, dpack64_10, dpack64_11,
	dpack64_12, dpack64_13, dpack64_14, dpack64_15, dpack64_16, dpack64_17,
	dpack64_18, dpack64_19, dpack64_20, dpack64_21, dpack64_22, dpack64_23,
	dpack64_24, dpack64_25, dpack64_26, dpack64_27, dpack64_28, dpack64_29,
	dpack64_30, dpack64_31, dpack64_32, dpack64_33, dpack64_34, dpack64_35,
	dpack64_36, dpack64_37, dpack64_38, dpack64_39, dpack64_40, dpack64_41,
	dpack64_42, dpack64_43, dpack64_44, dpack64_45, dpack64_46, dpack64_47,
	dpack64_48, dpack64_49, dpack64_50, dpack64_51, dpack64_52, dpack64_53,
	dpack64_54, dpack64_55, dpack64_56, dpack64_57, dpack64_58, dpack64_59,
	dpack64_60, dpack64_61, dpack64_62, dpack64_63, dpack64_64,
}

var fdunpack64 = []func(in *byte, out *uint64, seed *uint64){
	dunpack64_0, dunpack64_1, dunpack64_2, dunpack64_3, dunpack64_4,
	dunpack64_5, dunpack64_6, dunpack64_7, dunpack64_8, dunpack64_9,
	dunpack64_10, dunpack64_11, dunpack64_12, dunpack64_13, dunpack64_14,
	dunpack64_15, dunpack64_16, dunpack64_17, dunpack64_18, dunpack64_19,
	dunpack64_20, dunpack64_21, dunpack64_22, dunpack64_23, dunpack64_24,
	dunpack64_25, dunpack64_26, dunpack64_27, dunpack64_28, dunpack64_29,
	dunpack64_30, dunpack64_31, dunpack64_32, dunpack64_33, dunpack64_34,
	dunpack64_35, dunpack64_36, dunpack64_37, dunpack64_38, dunpack64_39,
	dunpack64_40, dunpack64_41, dunpack64_42, dunpack64_43, dunpack64_44,
	dunpack64_45, dunpack64_46, dunpack64_47, dunpack64_48, dunpack64_49,
	dunpack64_50, dunpack64_51, dunpack64_52, dunpack64_53, dunpack64_54,
	dunpack64_55, dunpack64_56, dunpack64_57, dunpack64_58, dunpack64_59,
	dunpack64_60, dunpack64_61, dunpack64_62, dunpack64_63, dunpack64_64,
}

// PackedInts represents compressed integers.
type PackedInts struct {
	length int
	bytes  []byte
	seed   []uint64
}

// Len returns the number of packed integers.
func (p *PackedInts) Len() int {
	return p.length
}

// Size returns the compressed size in bytes.
func (p *PackedInts) Size() int {
	return len(p.bytes) + len(p.seed)
}

func checkErr(err ...error) error {
	for _, e := range err {
		if e != nil {
			return e
		}
	}

	return nil
}

// DeltaPack compresses a given integer slice
// using differential coding. Aside from the input
// requirements of PackInts, the input slice should
// also be in ascending order.
func DeltaPack(in []uint64) *PackedInts {
	inAddr := unsafe.Pointer(&in[0])

	intSize := 64
	fpack := fdpack64
	maxBits := dmaxBits128_64

	seed := make([]uint64, 2)
	iseed := make([]uint64, 2)

	copy(seed, in[0:2])
	copy(iseed, seed)

	// Determine the number of bytes to allocate.
	// For each cluster, allocate 16 bytes to store
	// the bit sizes of each block.
	length := len(in)
	nclusters := length / clusterSize
	nbytes := nclusters * numBlocks
	if length-(nclusters*clusterSize) >= blockSize {
		// Allocate another 16 bytes for overflow
		// greater than or equal to blockSize.
		nbytes += numBlocks
	}

	// Allocate bytes to store the packed integers.
	cin := 0
	bitSizes := make([]uint8, 0, (length/blockSize)+1)
	for ; length >= blockSize; length -= blockSize {
		bs := maxBits(uintptr(inAddr), cin*blockSize, &seed[0])
		bitSizes = append(bitSizes, bs)

		nbytes += (int(bs) * blockSize) / 8
		cin++
	}

	if length > 0 {
		nbytes += (length * intSize) / 8
	}

	// Create output data and reinitialize seed
	copy(seed, iseed)
	out := make([]byte, nbytes+1)

	// Compress input data. Process the clusters first.
	cin = 0
	cout := 0
	length = len(in)
	for cin+clusterSize <= length {
		bs := bitSizes[:numBlocks]
		bitSizes = bitSizes[numBlocks:]

		copy(out[cout:], bs)
		cout += numBlocks

		for _, sz := range bs {
			fpack[sz](&in[cin], &out[cout], &seed[0])

			cin += blockSize
			cout += (int(sz) * blockSize) / 8
		}
	}

	// Process the remaining blocks
	if cin+blockSize <= length {
		copy(out[cout:], bitSizes)
		cout += numBlocks

		for _, sz := range bitSizes {
			fpack[sz](&in[cin], &out[cout], &seed[0])

			cin += blockSize
			cout += (int(sz) * blockSize) / 8
		}
	}

	// Process the remaining inputs
	if cin < length {
		input := in[cin]
		binary.BigEndian.PutUint64(out[cout:cout+8], input)
		cin += 1
		cout += 8
	}

	return &PackedInts{
		length,
		out,
		iseed,
	}
}

func DeltaUnpack(in *PackedInts, out []uint64) {
	length := in.length
	inBytes := in.bytes

	seed := make([]uint64, 2)
	copy(seed, in.seed)

	funpack := fdunpack64

	// Process the clusters first
	cin := 0
	cout := 0
	for cout+clusterSize <= length {
		bitSizes := inBytes[cin : cin+numBlocks]
		cin += numBlocks

		for _, sz := range bitSizes {
			funpack[sz](&inBytes[cin], &out[cout], &seed[0])

			cout += blockSize
			cin += (int(sz) * blockSize) / 8
		}
	}

	// Process the remaining blocks
	if cout+blockSize <= length {
		bitSizes := inBytes[cin : cin+((length-cout)/blockSize)]
		cin += numBlocks

		for _, sz := range bitSizes {
			funpack[sz](&inBytes[cin], &out[cout], &seed[0])

			cout += blockSize
			cin += (int(sz) * blockSize) / 8
		}
	}

	// Process the remaining inputs
	if cout < length {
		val := binary.BigEndian.Uint64(inBytes[cin : cin+8])
		out[cout] = val
		cout++
		cin += 8
	}
}
