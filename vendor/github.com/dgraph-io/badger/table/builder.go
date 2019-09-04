/*
 * Copyright 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package table

import (
	"bytes"
	"math"
	"unsafe"

	"github.com/dgryski/go-farm"
	"github.com/golang/protobuf/proto"

	"github.com/dgraph-io/badger/pb"
	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/ristretto/z"
)

func newBuffer(sz int) *bytes.Buffer {
	b := new(bytes.Buffer)
	b.Grow(sz)
	return b
}

type header struct {
	overlap uint16 // Overlap with base key.
	diff    uint16 // Length of the diff.
}

// Encode encodes the header.
func (h header) Encode() []byte {
	var b [4]byte
	*(*header)(unsafe.Pointer(&b[0])) = h
	return b[:]
}

// Decode decodes the header.
func (h *header) Decode(buf []byte) int {
	*h = *(*header)(unsafe.Pointer(&buf[0]))
	return h.Size()
}

const headerSize = 4

// Size returns size of the header. Currently it's just a constant.
func (h header) Size() int { return headerSize }

// Builder is used in building a table.
type Builder struct {
	// Typically tens or hundreds of meg. This is for one single file.
	buf *bytes.Buffer

	baseKey      []byte   // Base key for the current block.
	baseOffset   uint32   // Offset for the current block.
	entryOffsets []uint32 // Offsets of entries present in current block.

	tableIndex *pb.TableIndex
	keyHashes  []uint64

	opt *Options
}

// NewTableBuilder makes a new TableBuilder.
func NewTableBuilder(opts Options) *Builder {
	return &Builder{
		buf:        newBuffer(1 << 20),
		tableIndex: &pb.TableIndex{},
		keyHashes:  make([]uint64, 0, 1024), // Avoid some malloc calls.
		opt:        &opts,
	}
}

// Close closes the TableBuilder.
func (b *Builder) Close() {}

// Empty returns whether it's empty.
func (b *Builder) Empty() bool { return b.buf.Len() == 0 }

// keyDiff returns a suffix of newKey that is different from b.baseKey.
func (b *Builder) keyDiff(newKey []byte) []byte {
	var i int
	for i = 0; i < len(newKey) && i < len(b.baseKey); i++ {
		if newKey[i] != b.baseKey[i] {
			break
		}
	}
	return newKey[i:]
}

func (b *Builder) addHelper(key []byte, v y.ValueStruct) {
	b.keyHashes = append(b.keyHashes, farm.Fingerprint64(y.ParseKey(key)))

	// diffKey stores the difference of key with baseKey.
	var diffKey []byte
	if len(b.baseKey) == 0 {
		// Make a copy. Builder should not keep references. Otherwise, caller has to be very careful
		// and will have to make copies of keys every time they add to builder, which is even worse.
		b.baseKey = append(b.baseKey[:0], key...)
		diffKey = key
	} else {
		diffKey = b.keyDiff(key)
	}

	h := header{
		overlap: uint16(len(key) - len(diffKey)),
		diff:    uint16(len(diffKey)),
	}

	// store current entry's offset
	y.AssertTrue(uint32(b.buf.Len()) < math.MaxUint32)
	b.entryOffsets = append(b.entryOffsets, uint32(b.buf.Len())-b.baseOffset)

	// Layout: header, diffKey, value.
	b.buf.Write(h.Encode())
	b.buf.Write(diffKey) // We only need to store the key difference.

	v.EncodeTo(b.buf)
}

/*
Structure of Block.
+-------------------+---------------------+--------------------+--------------+------------------+
| Entry1            | Entry2              | Entry3             | Entry4       | Entry5           |
+-------------------+---------------------+--------------------+--------------+------------------+
| Entry6            | ...                 | ...                | ...          | EntryN           |
+-------------------+---------------------+--------------------+--------------+------------------+
| Block Meta(contains list of offsets used| Block Meta Size    | Block        | Checksum Size    |
| to perform binary search in the block)  | (4 Bytes)          | Checksum     | (4 Bytes)        |
+-----------------------------------------+--------------------+--------------+------------------+
*/
func (b *Builder) finishBlock() {
	b.buf.Write(y.U32SliceToBytes(b.entryOffsets))
	b.buf.Write(y.U32ToBytes(uint32(len(b.entryOffsets))))

	blockBuf := b.buf.Bytes()[b.baseOffset:] // Store checksum for current block.
	b.writeChecksum(blockBuf)

	// TODO(Ashish):Add padding: If we want to make block as multiple of OS pages, we can
	// implement padding. This might be useful while using direct I/O.

	// Add key to the block index
	bo := &pb.BlockOffset{
		Key:    y.Copy(b.baseKey),
		Offset: b.baseOffset,
		Len:    uint32(b.buf.Len()) - b.baseOffset,
	}
	b.tableIndex.Offsets = append(b.tableIndex.Offsets, bo)
}

func (b *Builder) shouldFinishBlock(key []byte, value y.ValueStruct) bool {
	// If there is no entry till now, we will return false.
	if len(b.entryOffsets) <= 0 {
		return false
	}

	// Integer overflow check for statements below.
	y.AssertTrue((uint32(len(b.entryOffsets))+1)*4+4+8+4 < math.MaxUint32)
	// We should include current entry also in size, that's why +1 to len(b.entryOffsets).
	entriesOffsetsSize := uint32((len(b.entryOffsets)+1)*4 +
		4 + // size of list
		8 + // Sum64 in checksum proto
		4) // checksum length
	estimatedSize := uint32(b.buf.Len()) - b.baseOffset + uint32(6 /*header size for entry*/) +
		uint32(len(key)) + uint32(value.EncodedSize()) + entriesOffsetsSize

	return estimatedSize > uint32(b.opt.BlockSize)
}

// Add adds a key-value pair to the block.
func (b *Builder) Add(key []byte, value y.ValueStruct) {
	if b.shouldFinishBlock(key, value) {
		b.finishBlock()
		// Start a new block. Initialize the block.
		b.baseKey = []byte{}
		y.AssertTrue(uint32(b.buf.Len()) < math.MaxUint32)
		b.baseOffset = uint32(b.buf.Len())
		b.entryOffsets = b.entryOffsets[:0]
	}
	b.addHelper(key, value)
}

// TODO: vvv this was the comment on ReachedCapacity.
// FinalSize returns the *rough* final size of the array, counting the header which is
// not yet written.
// TODO: Look into why there is a discrepancy. I suspect it is because of Write(empty, empty)
// at the end. The diff can vary.

// ReachedCapacity returns true if we... roughly (?) reached capacity?
func (b *Builder) ReachedCapacity(cap int64) bool {
	blocksSize := b.buf.Len() + // length of current buffer
		len(b.entryOffsets)*4 + // all entry offsets size
		4 + // count of all entry offsets
		8 + // checksum bytes
		4 // checksum length
	estimateSz := blocksSize +
		4 + // Index length
		5*(len(b.tableIndex.Offsets)) // approximate index size

	return int64(estimateSz) > cap
}

// Finish finishes the table by appending the index.
/*
The table structure looks like
+---------+------------+-----------+---------------+
| Block 1 | Block 2    | Block 3   | Block 4       |
+---------+------------+-----------+---------------+
| Block 5 | Block 6    | Block ... | Block N       |
+---------+------------+-----------+---------------+
| Index   | Index Size | Checksum  | Checksum Size |
+---------+------------+-----------+---------------+
*/
func (b *Builder) Finish() []byte {
	bf := z.NewBloomFilter(float64(len(b.keyHashes)), b.opt.BloomFalsePositive)
	for _, h := range b.keyHashes {
		bf.Add(h)
	}
	// Add bloom filter to the index.
	b.tableIndex.BloomFilter = bf.JSONMarshal()

	b.finishBlock() // This will never start a new block.

	index, err := proto.Marshal(b.tableIndex)
	y.Check(err)
	// Write index the file.
	n, err := b.buf.Write(index)
	y.Check(err)

	y.AssertTrue(uint32(n) < math.MaxUint32)
	// Write index size.
	_, err = b.buf.Write(y.U32ToBytes(uint32(n)))
	y.Check(err)

	b.writeChecksum(index)
	return b.buf.Bytes()
}

func (b *Builder) writeChecksum(data []byte) {
	// Build checksum for the index.
	checksum := pb.Checksum{
		// TODO: The checksum type should be configurable from the
		// options.
		// We chose to use CRC32 as the default option because
		// it performed better compared to xxHash64.
		// See the BenchmarkChecksum in table_test.go file
		// Size     =>   1024 B        2048 B
		// CRC32    => 63.7 ns/op     112 ns/op
		// xxHash64 => 87.5 ns/op     158 ns/op
		Sum:  y.CalculateChecksum(data, pb.Checksum_CRC32C),
		Algo: pb.Checksum_CRC32C,
	}

	// Write checksum to the file.
	chksum, err := proto.Marshal(&checksum)
	y.Check(err)
	n, err := b.buf.Write(chksum)
	y.Check(err)

	y.AssertTrue(uint32(n) < math.MaxUint32)
	// Write checksum size.
	_, err = b.buf.Write(y.U32ToBytes(uint32(n)))
	y.Check(err)
}
