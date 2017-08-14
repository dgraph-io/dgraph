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
	"math"
	"sort"

	"github.com/dgraph-io/dgraph/x"
)

const (
	// BlockSize is the number of integers per block. Each
	// block address must be aligned at 16-byte boundaries.
	BlockSize = 256
	intSize   = 64
	bitVarint = 0x80
)

var (
	maxBits func(*uint64, *uint64) uint8
	fpack   []func(*uint64, *byte, *uint64)
	funpack []func(*byte, *uint64, *uint64)
)

func init() {
	if BlockSize == 128 {
		fpack = fdpack128
		maxBits = maxBits128
		funpack = fdunpack128
	} else if BlockSize == 256 {
		fpack = fdpack256
		maxBits = maxBits256
		funpack = fdunpack256
	} else {
		x.Fatalf("Unknown block size")
	}
}

var fdpack128 = []func(in *uint64, out *byte, seed *uint64){
	dpack128_0, dpack128_1, dpack128_2, dpack128_3, dpack128_4, dpack128_5,
	dpack128_6, dpack128_7, dpack128_8, dpack128_9, dpack128_10, dpack128_11,
	dpack128_12, dpack128_13, dpack128_14, dpack128_15, dpack128_16, dpack128_17,
	dpack128_18, dpack128_19, dpack128_20, dpack128_21, dpack128_22, dpack128_23,
	dpack128_24, dpack128_25, dpack128_26, dpack128_27, dpack128_28, dpack128_29,
	dpack128_30, dpack128_31, dpack128_32, dpack128_33, dpack128_34, dpack128_35,
	dpack128_36, dpack128_37, dpack128_38, dpack128_39, dpack128_40, dpack128_41,
	dpack128_42, dpack128_43, dpack128_44, dpack128_45, dpack128_46, dpack128_47,
	dpack128_48, dpack128_49, dpack128_50, dpack128_51, dpack128_52, dpack128_53,
	dpack128_54, dpack128_55, dpack128_56, dpack128_57, dpack128_58, dpack128_59,
	dpack128_60, dpack128_61, dpack128_62, dpack128_63, dpack128_64,
}

var fdunpack128 = []func(in *byte, out *uint64, seed *uint64){
	dunpack128_0, dunpack128_1, dunpack128_2, dunpack128_3, dunpack128_4,
	dunpack128_5, dunpack128_6, dunpack128_7, dunpack128_8, dunpack128_9,
	dunpack128_10, dunpack128_11, dunpack128_12, dunpack128_13, dunpack128_14,
	dunpack128_15, dunpack128_16, dunpack128_17, dunpack128_18, dunpack128_19,
	dunpack128_20, dunpack128_21, dunpack128_22, dunpack128_23, dunpack128_24,
	dunpack128_25, dunpack128_26, dunpack128_27, dunpack128_28, dunpack128_29,
	dunpack128_30, dunpack128_31, dunpack128_32, dunpack128_33, dunpack128_34,
	dunpack128_35, dunpack128_36, dunpack128_37, dunpack128_38, dunpack128_39,
	dunpack128_40, dunpack128_41, dunpack128_42, dunpack128_43, dunpack128_44,
	dunpack128_45, dunpack128_46, dunpack128_47, dunpack128_48, dunpack128_49,
	dunpack128_50, dunpack128_51, dunpack128_52, dunpack128_53, dunpack128_54,
	dunpack128_55, dunpack128_56, dunpack128_57, dunpack128_58, dunpack128_59,
	dunpack128_60, dunpack128_61, dunpack128_62, dunpack128_63, dunpack128_64,
}

var fdpack256 = []func(in *uint64, out *byte, seed *uint64){
	dpack256_0, dpack256_1, dpack256_2, dpack256_3, dpack256_4, dpack256_5,
	dpack256_6, dpack256_7, dpack256_8, dpack256_9, dpack256_10, dpack256_11,
	dpack256_12, dpack256_13, dpack256_14, dpack256_15, dpack256_16, dpack256_17,
	dpack256_18, dpack256_19, dpack256_20, dpack256_21, dpack256_22, dpack256_23,
	dpack256_24, dpack256_25, dpack256_26, dpack256_27, dpack256_28, dpack256_29,
	dpack256_30, dpack256_31, dpack256_32, dpack256_33, dpack256_34, dpack256_35,
	dpack256_36, dpack256_37, dpack256_38, dpack256_39, dpack256_40, dpack256_41,
	dpack256_42, dpack256_43, dpack256_44, dpack256_45, dpack256_46, dpack256_47,
	dpack256_48, dpack256_49, dpack256_50, dpack256_51, dpack256_52, dpack256_53,
	dpack256_54, dpack256_55, dpack256_56, dpack256_57, dpack256_58, dpack256_59,
	dpack256_60, dpack256_61, dpack256_62, dpack256_63, dpack256_64,
}

var fdunpack256 = []func(in *byte, out *uint64, seed *uint64){
	dunpack256_0, dunpack256_1, dunpack256_2, dunpack256_3, dunpack256_4,
	dunpack256_5, dunpack256_6, dunpack256_7, dunpack256_8, dunpack256_9,
	dunpack256_10, dunpack256_11, dunpack256_12, dunpack256_13, dunpack256_14,
	dunpack256_15, dunpack256_16, dunpack256_17, dunpack256_18, dunpack256_19,
	dunpack256_20, dunpack256_21, dunpack256_22, dunpack256_23, dunpack256_24,
	dunpack256_25, dunpack256_26, dunpack256_27, dunpack256_28, dunpack256_29,
	dunpack256_30, dunpack256_31, dunpack256_32, dunpack256_33, dunpack256_34,
	dunpack256_35, dunpack256_36, dunpack256_37, dunpack256_38, dunpack256_39,
	dunpack256_40, dunpack256_41, dunpack256_42, dunpack256_43, dunpack256_44,
	dunpack256_45, dunpack256_46, dunpack256_47, dunpack256_48, dunpack256_49,
	dunpack256_50, dunpack256_51, dunpack256_52, dunpack256_53, dunpack256_54,
	dunpack256_55, dunpack256_56, dunpack256_57, dunpack256_58, dunpack256_59,
	dunpack256_60, dunpack256_61, dunpack256_62, dunpack256_63, dunpack256_64,
}

type BPackEncoder struct {
	data     x.BytesBuffer
	metadata x.BytesBuffer
	length   int
	// Used to store seed of last block
	lastSeed []uint64
	// Offset into data
	offset int
}

func (bp *BPackEncoder) PackAppend(in []uint64) {
	if len(in) == 0 {
		return
	}

	if len(bp.lastSeed) == 0 && len(in) >= 2 {
		bp.lastSeed = make([]uint64, 2)
		bp.lastSeed[0] = in[0]
		bp.lastSeed[1] = in[1]
	} else if len(bp.lastSeed) == 0 && len(in) == 1 {
		// We won't use seed value for varint, writing it in metadata
		// to have uniform length for metadata
		bp.lastSeed = make([]uint64, 2)
	}

	bp.length += len(in)
	b := bp.metadata.Slice(20)
	binary.BigEndian.PutUint64(b[0:8], bp.lastSeed[0])
	binary.BigEndian.PutUint64(b[8:16], bp.lastSeed[1])
	binary.BigEndian.PutUint32(b[16:20], uint32(bp.offset))

	// This should be the last block
	if len(in) < BlockSize {
		b = bp.data.Slice(1 + 10*len(in))
		b[0] = 0 | bitVarint
		off := 1
		for _, num := range in {
			off += binary.PutUvarint(b[off:], num)
		}
		bp.data.TruncateBy(len(b) - off)
		return
	}

	bs := maxBits(&in[0], &bp.lastSeed[0])
	nBytes := int(bs)*BlockSize/8 + 1
	b = bp.data.Slice(nBytes)
	b[0] = bs
	if bs > 0 {
		fpack[bs](&in[0], &b[1], &bp.lastSeed[0])
	}
	bp.offset += nBytes
}

func (bp *BPackEncoder) WriteTo(in []byte) {
	x.AssertTrue(bp.length > 0)
	binary.BigEndian.PutUint32(in[:4], uint32(bp.length))

	if bp.length <= BlockSize {
		// If number of integers are less all are stored as varint
		// and without metadata.
		bp.data.CopyTo(in[4:])
		return
	}
	offset := bp.metadata.CopyTo(in[4:])
	bp.data.CopyTo(in[4+offset:])
}

func (bp *BPackEncoder) Size() int {
	if bp.length == 0 {
		return 0
	}
	return 4 + bp.data.Length() + bp.metadata.Length()
}

func (bp *BPackEncoder) Length() int {
	return bp.length
}

type BPackIterator struct {
	data     []byte
	metadata []byte
	length   int

	in_offset int
	count     int
	valid     bool
	lastSeed  []uint64
	// Byte slice which would be reused for decompression
	buf []uint64
	// out is the slice ready to be read by the user, would
	// point to some offset in buf
	out []uint64
}

func numBlocks(len int) int {
	if len < BlockSize {
		return 0
	}
	if len%BlockSize == 0 {
		return len / BlockSize
	}
	return len/BlockSize + 1
}

func (pi *BPackIterator) Init(data []byte, afterUid uint64) {
	if len(data) == 0 {
		return
	}

	pi.length = int(binary.BigEndian.Uint32(data[0:4]))
	nBlocks := numBlocks(pi.length)
	pi.data = data[4+nBlocks*20:]
	pi.metadata = data[4 : 4+nBlocks*20]
	pi.out = make([]uint64, BlockSize, BlockSize)
	pi.buf = pi.out
	pi.lastSeed = make([]uint64, 2)
	pi.valid = true

	if afterUid > 0 {
		pi.search(afterUid, nBlocks)
		uidx := sort.Search(len(pi.out), func(idx int) bool {
			return afterUid < pi.out[idx]
		})
		pi.out = pi.out[uidx:]
		return
	}

	if len(pi.metadata) > 0 {
		pi.lastSeed[0] = binary.BigEndian.Uint64(pi.metadata[0:8])
		pi.lastSeed[1] = binary.BigEndian.Uint64(pi.metadata[8:16])
	}
	pi.Next()
	return
}

func (pi *BPackIterator) search(afterUid uint64, numBlocks int) {
	if len(pi.metadata) == 0 {
		pi.Next()
		return
	}
	// Search in metadata whose seed[1] > afterUid
	idx := sort.Search(numBlocks, func(idx int) bool {
		i := idx * 20
		return afterUid < binary.BigEndian.Uint64(pi.metadata[i+8:i+16])
	})
	// seed is stored for previous block, so search there. If not found
	// then search in last block.
	if idx >= numBlocks {
		idx = numBlocks - 1
	} else if idx > 0 {
		idx -= 1
	}

	pi.count = idx * BlockSize
	i := idx * 20
	pi.in_offset = int(binary.BigEndian.Uint32(pi.metadata[i+16 : i+20]))
	pi.lastSeed[0] = binary.BigEndian.Uint64(pi.metadata[i : i+8])
	pi.lastSeed[1] = binary.BigEndian.Uint64(pi.metadata[i+8 : i+16])
	pi.Next()
}

func (pi *BPackIterator) AfterUid(uid uint64) (found bool) {
	// Current uncompressed block doesn't have uid, search for appropriate
	// block, uncompress it and store it in pi.out
	if pi.out[len(pi.out)-1] < uid {
		nBlocks := numBlocks(pi.length)
		pi.search(uid-1, nBlocks)
	}
	// Search for uid in the current block
	uidx := sort.Search(len(pi.out), func(idx int) bool {
		return pi.out[idx] >= uid
	})
	if uidx < len(pi.out) && pi.out[uidx] == uid {
		found = true
		uidx++
	}
	// Expose slice whose startId > uid to the user
	if uidx < len(pi.out) {
		pi.out = pi.out[uidx:]
		return
	}
	pi.Next()
	return
}

func (pi *BPackIterator) Valid() bool {
	return pi.valid
}

func (pi *BPackIterator) Length() int {
	return pi.length
}

// Returns the startIndex
func (pi *BPackIterator) StartIdx() int {
	return pi.count - len(pi.out)
}

func (pi *BPackIterator) Uids() []uint64 {
	return pi.out
}

func (pi *BPackIterator) Next() {
	if pi.count >= pi.length {
		pi.valid = false
		pi.out = pi.buf[:0]
		return
	}

	sz := uint8(pi.data[pi.in_offset])
	pi.in_offset++
	if sz&bitVarint != 0 {
		//varint is the last block and has less than blockSize integers
		pi.out = pi.buf[:0]
		for pi.count < pi.length {
			i, n := binary.Uvarint(pi.data[pi.in_offset:])
			pi.out = append(pi.out, i)
			pi.in_offset += n
			pi.count++
		}
		return
	}
	pi.out = pi.buf[:BlockSize]
	funpack[sz](&pi.data[pi.in_offset], &pi.out[0], &pi.lastSeed[0])
	pi.in_offset += (int(sz) * BlockSize) / 8
	pi.count += BlockSize
}

func (pi *BPackIterator) SkipNext() {
	if pi.count >= pi.length {
		pi.valid = false
		pi.out = pi.buf[:0]
		return
	}

	// Find the bit size of the block
	sz := uint8(pi.data[pi.in_offset])
	// If it's varint block,(The last one)
	if sz&bitVarint != 0 {
		pi.in_offset = len(pi.data)
		pi.count = pi.length
		return
	}
	// Calculate size of the block based on bitsize
	pi.in_offset += (int(sz)*BlockSize)/8 + 1
	pi.count += BlockSize
	// Update seed
	i := (pi.count / BlockSize) * 20
	pi.lastSeed[0] = binary.BigEndian.Uint64(pi.metadata[i : i+8])
	pi.lastSeed[1] = binary.BigEndian.Uint64(pi.metadata[i+8 : i+16])
}

func (pi *BPackIterator) MaxIntInBlock() uint64 {
	nBlocks := numBlocks(pi.length)
	currBlock := pi.count / BlockSize
	// We find max value through seed value stored in next meta block, so
	// if it's a last block, we don't know the max so we return maxuint64
	if currBlock >= nBlocks-1 {
		return math.MaxUint64
	}
	// MaxInt in current block can be found by seed value of next block
	midx := (currBlock + 1) * 20
	return binary.BigEndian.Uint64(pi.metadata[midx+8 : midx+16])
}

func DeltaUnpack(in []byte, out []uint64) {
	var bi BPackIterator
	bi.Init(in, 0)
	offset := 0
	x.AssertTrue(len(out) == bi.Length())

	for bi.Valid() {
		uids := bi.Uids()
		// Benchmarks would be slower due to this copy
		copy(out[offset:], uids)
		offset += len(uids)
		bi.Next()
	}
}

func DeltaPack(in []uint64) []byte {
	var bp BPackEncoder
	offset := 0
	for offset+BlockSize <= len(in) {
		bp.PackAppend(in[offset : offset+BlockSize])
		offset += BlockSize
	}
	if offset < len(in) {
		bp.PackAppend(in[offset:])
	}
	x := make([]byte, bp.Size())
	bp.WriteTo(x)
	return x
}
