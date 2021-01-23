/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package codec

import (
	"bytes"
	"io"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

type seekPos int

const (
	// SeekStart is used with Seek() to search relative to the Uid, returning it in the results.
	SeekStart seekPos = iota
	// SeekCurrent to Seek() a Uid using it as offset, not as part of the results.
	SeekCurrent
)

var (
	bitMask uint64 = 0xffffffff00000000
)

// Encoder is used to convert a list of UIDs into a pb.UidPack object.
type Encoder struct {
	roar  *roaring64.Bitmap
	uids  []uint64
	Alloc *z.Allocator
	buf   *bytes.Buffer
}

// Add takes an uid and adds it to the list of UIDs to be encoded.
func (e *Encoder) Add(uid uint64) {
	if e.roar == nil {
		e.roar = roaring64.NewBitmap()
	}
	e.roar.Add(uid)
}

// Done returns the final output of the encoder. This UidPack MUST BE FREED via a call to FreePack.
func (e *Encoder) Done(w io.Writer) int64 {
	n, err := e.roar.WriteTo(w)
	x.Check(err)
	return n
}

// Encode takes in a list of uids and a block size. It would pack these uids into blocks of the
// given size, with the last block having fewer uids. Within each block, it stores the first uid as
// base. For each next uid, a delta = uids[i] - uids[i-1] is stored. Protobuf uses Varint encoding,
// as mentioned here: https://developers.google.com/protocol-buffers/docs/encoding . This ensures
// that the deltas being considerably smaller than the original uids are nicely packed in fewer
// bytes. Our benchmarks on artificial data show compressed size to be 13% of the original. This
// mechanism is a LOT simpler to understand and if needed, debug.
func Encode(uids []uint64) []byte {
	r := roaring64.New()
	r.AddMany(uids)
	b, err := r.ToBytes()
	x.Check(err)
	return b
}

// // EncodeFromBuffer is the same as Encode but it accepts a byte slice instead of a uint64 slice.
// func EncodeFromBuffer(buf []byte, blockSize int) *pb.UidPack {
// 	enc := Encoder{BlockSize: blockSize}
// 	var prev uint64
// 	for len(buf) > 0 {
// 		uid, n := binary.Uvarint(buf)
// 		buf = buf[n:]

// 		next := prev + uid
// 		enc.Add(next)
// 		prev = next
// 	}
// 	return enc.Done()
// }

// // Decode decodes the UidPack back into the list of uids. This is a stop-gap function, Decode would
// // need to do more specific things than just return the list back.
// func Decode(pack *pb.UidPack, seek uint64) []uint64 {
// 	out := make([]uint64, 0, ApproxLen(pack))
// 	dec := Decoder{Pack: pack}

// 	for uids := dec.Seek(seek, SeekStart); len(uids) > 0; uids = dec.Next() {
// 		out = append(out, uids...)
// 	}
// 	return out
// }

// // DecodeToBuffer is the same as Decode but it returns a z.Buffer which is
// // calloc'ed and can be SHOULD be freed up by calling buffer.Release().
// func DecodeToBuffer(pack *pb.UidPack, seek uint64) *z.Buffer {
// 	buf, err := z.NewBufferWith(256<<20, 32<<30, z.UseCalloc)
// 	x.Check(err)
// 	buf.AutoMmapAfter(1 << 30)

// 	var last uint64
// 	tmp := make([]byte, 16)
// 	dec := Decoder{Pack: pack}
// 	for uids := dec.Seek(seek, SeekStart); len(uids) > 0; uids = dec.Next() {
// 		for _, u := range uids {
// 			n := binary.PutUvarint(tmp, u-last)
// 			x.Check2(buf.Write(tmp[:n]))
// 			last = u
// 		}
// 	}
// 	return buf
// }
