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
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"
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

var fpack32 = []func(in uintptr, out *byte, inOffset int, seed *byte){
	pack32_0, pack32_1, pack32_2, pack32_3, pack32_4, pack32_5, pack32_6,
	pack32_7, pack32_8, pack32_9, pack32_10, pack32_11, pack32_12, pack32_13,
	pack32_14, pack32_15, pack32_16, pack32_17, pack32_18, pack32_19, pack32_20,
	pack32_21, pack32_22, pack32_23, pack32_24, pack32_25, pack32_26, pack32_27,
	pack32_28, pack32_29, pack32_30, pack32_31, pack32_32,
}

var fdpack32 = []func(in uintptr, out *byte, inOffset int, seed *byte){
	dpack32_0, dpack32_1, dpack32_2, dpack32_3, dpack32_4, dpack32_5,
	dpack32_6, dpack32_7, dpack32_8, dpack32_9, dpack32_10, dpack32_11,
	dpack32_12, dpack32_13, dpack32_14, dpack32_15, dpack32_16, dpack32_17,
	dpack32_18, dpack32_19, dpack32_20, dpack32_21, dpack32_22, dpack32_23,
	dpack32_24, dpack32_25, dpack32_26, dpack32_27, dpack32_28, dpack32_29,
	dpack32_30, dpack32_31, dpack32_32,
}

var funpack32 = []func(in *byte, out uintptr, outOffset int, seed *byte){
	unpack32_0, unpack32_1, unpack32_2, unpack32_3, unpack32_4, unpack32_5,
	unpack32_6, unpack32_7, unpack32_8, unpack32_9, unpack32_10, unpack32_11,
	unpack32_12, unpack32_13, unpack32_14, unpack32_15, unpack32_16, unpack32_17,
	unpack32_18, unpack32_19, unpack32_20, unpack32_21, unpack32_22, unpack32_23,
	unpack32_24, unpack32_25, unpack32_26, unpack32_27, unpack32_28, unpack32_29,
	unpack32_30, unpack32_31, unpack32_32,
}

var fdunpack32 = []func(in *byte, out uintptr, outOffset int, seed *byte){
	dunpack32_0, dunpack32_1, dunpack32_2, dunpack32_3, dunpack32_4,
	dunpack32_5, dunpack32_6, dunpack32_7, dunpack32_8, dunpack32_9,
	dunpack32_10, dunpack32_11, dunpack32_12, dunpack32_13, dunpack32_14,
	dunpack32_15, dunpack32_16, dunpack32_17, dunpack32_18, dunpack32_19,
	dunpack32_20, dunpack32_21, dunpack32_22, dunpack32_23, dunpack32_24,
	dunpack32_25, dunpack32_26, dunpack32_27, dunpack32_28, dunpack32_29,
	dunpack32_30, dunpack32_31, dunpack32_32,
}

var fpack64 = []func(in uintptr, out *byte, inOffset int, seed *byte){
	pack64_0, pack64_1, pack64_2, pack64_3, pack64_4, pack64_5, pack64_6,
	pack64_7, pack64_8, pack64_9, pack64_10, pack64_11, pack64_12, pack64_13,
	pack64_14, pack64_15, pack64_16, pack64_17, pack64_18, pack64_19, pack64_20,
	pack64_21, pack64_22, pack64_23, pack64_24, pack64_25, pack64_26, pack64_27,
	pack64_28, pack64_29, pack64_30, pack64_31, pack64_32, pack64_33, pack64_34,
	pack64_35, pack64_36, pack64_37, pack64_38, pack64_39, pack64_40, pack64_41,
	pack64_42, pack64_43, pack64_44, pack64_45, pack64_46, pack64_47, pack64_48,
	pack64_49, pack64_50, pack64_51, pack64_52, pack64_53, pack64_54, pack64_55,
	pack64_56, pack64_57, pack64_58, pack64_59, pack64_60, pack64_61, pack64_62,
	pack64_63, pack64_64,
}

var fdpack64 = []func(in uintptr, out *byte, inOffset int, seed *byte){
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

var funpack64 = []func(in *byte, out uintptr, outOffset int, seed *byte){
	unpack64_0, unpack64_1, unpack64_2, unpack64_3, unpack64_4, unpack64_5,
	unpack64_6, unpack64_7, unpack64_8, unpack64_9, unpack64_10, unpack64_11,
	unpack64_12, unpack64_13, unpack64_14, unpack64_15, unpack64_16, unpack64_17,
	unpack64_18, unpack64_19, unpack64_20, unpack64_21, unpack64_22, unpack64_23,
	unpack64_24, unpack64_25, unpack64_26, unpack64_27, unpack64_28, unpack64_29,
	unpack64_30, unpack64_31, unpack64_32, unpack64_33, unpack64_34, unpack64_35,
	unpack64_36, unpack64_37, unpack64_38, unpack64_39, unpack64_40, unpack64_41,
	unpack64_42, unpack64_43, unpack64_44, unpack64_45, unpack64_46, unpack64_47,
	unpack64_48, unpack64_49, unpack64_50, unpack64_51, unpack64_52, unpack64_53,
	unpack64_54, unpack64_55, unpack64_56, unpack64_57, unpack64_58, unpack64_59,
	unpack64_60, unpack64_61, unpack64_62, unpack64_63, unpack64_64,
}

var fdunpack64 = []func(in *byte, out uintptr, outOffset int, seed *byte){
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
	delta bool
	kind  reflect.Kind

	length int
	bytes  []byte
	seed   []byte
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

// GobEncode allows gob encoding of packed integers.
func (p *PackedInts) GobEncode() ([]byte, error) {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)

	err := checkErr(
		enc.Encode(p.delta),
		enc.Encode(p.kind),
		enc.Encode(p.length),
		enc.Encode(len(p.bytes)),
		enc.Encode(len(p.seed)),

		enc.Encode(p.bytes),
		enc.Encode(p.seed),
	)

	if err != nil {
		err = fmt.Errorf("bp128: encode failed (%v)", err)
	}

	return buf.Bytes(), err
}

// GobDecode allows gob decoding of packed integers.
func (p *PackedInts) GobDecode(data []byte) error {
	buf := bytes.NewReader(data)
	dec := gob.NewDecoder(buf)

	nbytes, nseed := 0, 0
	err := checkErr(
		dec.Decode(&p.delta),
		dec.Decode(&p.kind),
		dec.Decode(&p.length),
		dec.Decode(&nbytes),
		dec.Decode(&nseed),
	)
	if err != nil {
		return fmt.Errorf("bp128: decode failed (%v)", err)
	}

	p.bytes = makeAlignedBytes(nbytes)
	p.seed = makeAlignedBytes(nseed)
	err = checkErr(
		dec.Decode(&p.bytes),
		dec.Decode(&p.seed),
	)
	if err != nil {
		return fmt.Errorf("bp128: decode failed (%v)", err)
	}

	return nil
}

// Pack compresses a given integer slice. It accepts []int,
// []uint, []int64, []uint64, []int32, and []uint32 slices.
// If in is not aligned, it will be copied to a new aligned
// slice before packing. To prevent this, use MakeAlignedSlice
// and put the values in the created slice before calling Pack.
func Pack(in interface{}) *PackedInts {
	return pack(in, false)
}

// DeltaPack compresses a given integer slice
// using differential coding. Aside from the input
// requirements of PackInts, the input slice should
// also be in ascending order.
func DeltaPack(in interface{}) *PackedInts {
	return pack(in, true)
}

// Unpack decompresses the given packed integers. The out
// parameter should be a pointer to an integer slice that
// has the same type as the one used when packing. If out
// is not aligned or has insufficient length to store the
// unpacked integers, out will be extended to an aligned slice
// before unpacking. To prevent this, create an aligned slice by
// calling MakeAlignedSlice and used in.Len() as the length parameter.
func Unpack(in *PackedInts, out interface{}) {
	unpack(in, out)
}

func pack(in interface{}, isDelta bool) *PackedInts {
	vin := reflect.ValueOf(in)
	inAddr := unsafe.Pointer(vin.Pointer())
	if vin.Kind() != reflect.Slice {
		panic("bp128: input is not an integer slice")
	}

	intSize := 0
	var maxBits func(uintptr, int, *byte) uint8
	var fpack []func(uintptr, *byte, int, *byte)

	switch in.(type) {
	case []int, []uint, []int64, []uint64:
		intSize = 64
		fpack = fpack64
		maxBits = maxBits128_64

		if isDelta {
			fpack = fdpack64
			maxBits = dmaxBits128_64
		}

	case []int32, []uint32:
		intSize = 32
		fpack = fpack32
		maxBits = maxBits128_32

		if isDelta {
			fpack = fdpack32
			maxBits = dmaxBits128_32
		}

	default:
		panic("bp128: unsupported integer slice type")
	}

	seed := makeAlignedBytes(16)
	iseed := makeAlignedBytes(16)
	if !isDelta {
		iseed = nil
	}

	nslice := min(vin.Len(), blockSize/intSize)
	copy(seed, convertToBytes(intSize, vin.Slice(0, nslice)))
	copy(iseed, seed)

	if !isAligned(intSize, uintptr(inAddr), 0) {
		vin = alignSlice(intSize, vin)
		inAddr = unsafe.Pointer(vin.Pointer())
	}

	// Determine the number of bytes to allocate.
	// For each cluster, allocate 16 bytes to store
	// the bit sizes of each block.
	length := vin.Len()
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
	out := makeAlignedBytes(nbytes + 1)

	// Compress input data. Process the clusters first.
	cin = 0
	cout := 0
	length = vin.Len()
	for cin+clusterSize <= length {
		bs := bitSizes[:numBlocks]
		bitSizes = bitSizes[numBlocks:]

		copy(out[cout:], bs)
		cout += numBlocks

		for _, sz := range bs {
			fpack[sz](uintptr(inAddr), &out[cout], cin, &seed[0])

			cin += blockSize
			cout += (int(sz) * blockSize) / 8
		}
	}

	// Process the remaining blocks
	if cin+blockSize <= length {
		copy(out[cout:], bitSizes)
		cout += numBlocks

		for _, sz := range bitSizes {
			fpack[sz](uintptr(inAddr), &out[cout], cin, &seed[0])

			cin += blockSize
			cout += (int(sz) * blockSize) / 8
		}
	}

	// Process the remaining inputs
	if cin < length {
		vin = vin.Slice(cin, vin.Len())
		copy(out[cout:], convertToBytes(intSize, vin))
	}

	return &PackedInts{
		isDelta,
		vin.Type().Elem().Kind(),
		length,
		out,
		iseed,
	}
}

func unpack(in *PackedInts, out interface{}) {
	length := in.length
	inBytes := in.bytes
	isDelta := in.delta

	seed := []byte{0}
	if isDelta {
		seed = makeAlignedBytes(16)
		copy(seed, in.seed)
	}

	intSize := 0
	var funpack []func(*byte, uintptr, int, *byte)
	switch out.(type) {
	case *[]int, *[]uint, *[]int64, *[]uint64:
		intSize = 64
		funpack = funpack64
		if isDelta {
			funpack = fdunpack64
		}

	case *[]int32, *[]uint32:
		intSize = 32
		funpack = funpack32
		if isDelta {
			funpack = fdunpack32
		}

	case *[]int8, *[]uint8, *[]int16, *[]uint16:
		panic("bp128: unsupported integer slice type")
	default:
		panic("bp128: output is not a pointer to integer slice")
	}

	vout := reflect.ValueOf(out).Elem()
	outAddr := unsafe.Pointer(vout.Pointer())
	if vout.Type().Elem().Kind() != in.kind {
		panic("bp128: mismatched input-output type")
	}

	if vout.Len() < length {
		if vout.Cap() >= length {
			vout = vout.Slice(0, length)
		} else {
			MakeAlignedSlice(length, out)

			vout = reflect.ValueOf(out).Elem()
			outAddr = unsafe.Pointer(vout.Pointer())
		}
	}

	if !isAligned(intSize, uintptr(outAddr), 0) {
		vout = alignSlice(intSize, vout)
		outAddr = unsafe.Pointer(vout.Pointer())
	}

	// Process the clusters first
	cin := 0
	cout := 0
	for cout+clusterSize <= length {
		bitSizes := inBytes[cin : cin+numBlocks]
		cin += numBlocks

		for _, sz := range bitSizes {
			funpack[sz](&inBytes[cin], uintptr(outAddr), cout, &seed[0])

			cout += blockSize
			cin += (int(sz) * blockSize) / 8
		}
	}

	// Process the remaining blocks
	if cout+blockSize <= length {
		bitSizes := inBytes[cin : cin+((length-cout)/blockSize)]
		cin += numBlocks

		for _, sz := range bitSizes {
			funpack[sz](&inBytes[cin], uintptr(outAddr), cout, &seed[0])

			cout += blockSize
			cin += (int(sz) * blockSize) / 8
		}
	}

	// Process the remaining inputs
	if cout < length {
		vout = vout.Slice(0, cout)
		vout = appendBytes(intSize, vout, inBytes[cin:])
	}

	// Set output
	reflect.ValueOf(out).Elem().Set(vout.Slice(0, length))
}
