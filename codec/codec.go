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
	"encoding/binary"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/dgraph-io/roaring/roaring64"
	"github.com/pkg/errors"
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

// // Encoder is used to convert a list of UIDs into a pb.UidPack object.
// type Encoder struct {
// 	roar  *roaring64.Bitmap
// 	uids  []uint64
// 	Alloc *z.Allocator
// 	buf   *bytes.Buffer
// }

// // Add takes an uid and adds it to the list of UIDs to be encoded.
// func (e *Encoder) Add(uid uint64) {
// 	if e.roar == nil {
// 		e.roar = roaring64.NewBitmap()
// 	}
// 	e.roar.Add(uid)
// }

// // Done returns the final output of the encoder. This UidPack MUST BE FREED via a call to FreePack.
// func (e *Encoder) Done(w io.Writer) int64 {
// 	n, err := e.roar.WriteTo(w)
// 	x.Check(err)
// 	return n
// }

func ApproxLen(pack *pb.UidPack) int {
	return 0
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

func ToList(rm *roaring64.Bitmap) *pb.List {
	return &pb.List{
		Uids: rm.ToArray(),
		// Bitmap: ToBytes(rm),
	}
}

func And(rm *roaring64.Bitmap, l *pb.List) {
	rl := FromList(l)
	rm.And(rl)
}

func MatrixToBitmap(matrix []*pb.List) *roaring64.Bitmap {
	res := roaring64.New()
	for _, l := range matrix {
		r := FromList(l)
		res.Or(r)
	}
	return res
}

func Intersect(matrix []*pb.List) *roaring64.Bitmap {
	out := roaring64.New()
	if len(matrix) == 0 {
		return out
	}
	out.Or(FromList(matrix[0]))
	for _, l := range matrix[1:] {
		r := FromList(l)
		out.And(r)
	}
	return out
}

func Merge(matrix []*pb.List) *roaring64.Bitmap {
	out := roaring64.New()
	if len(matrix) == 0 {
		return out
	}
	out.Or(FromList(matrix[0]))
	for _, l := range matrix[1:] {
		r := FromList(l)
		out.Or(r)
	}
	return out
}

func ToBytes(bm *roaring64.Bitmap) []byte {
	if bm.IsEmpty() {
		return nil
	}
	b, err := bm.ToBytes()
	x.Check(err)
	return b
}

func FromPostingList(r *roaring64.Bitmap, pl *pb.PostingList) error {
	if len(pl.Bitmap) == 0 {
		return nil
	}
	if err := r.UnmarshalBinary(pl.Bitmap); err != nil {
		return errors.Wrapf(err, "codec.FromPostingList")
	}
	return nil
}

func FromList(l *pb.List) *roaring64.Bitmap {
	iw := roaring64.New()
	if l == nil {
		return iw
	}
	if len(l.BitmapDoNotUse) > 0 {
		// Only one of Uids or Bitmap should be defined.
		x.Check(iw.UnmarshalBinary(l.BitmapDoNotUse))
	}
	if len(l.Uids) > 0 {
		iw.AddMany(l.Uids)
	}
	return iw
}

func FromBytes(buf []byte) *roaring64.Bitmap {
	r := roaring64.New()
	if buf == nil || len(buf) == 0 {
		return r
	}
	x.Check(r.UnmarshalBinary(buf))
	return r
}

func FromBackup(buf []byte) *roaring64.Bitmap {
	r := roaring64.New()
	var prev uint64
	for len(buf) > 0 {
		uid, n := binary.Uvarint(buf)
		if uid == 0 {
			break
		}
		buf = buf[n:]

		next := prev + uid
		r.Add(next)
		prev = next
	}
	return r
}

func ToUids(plist *pb.PostingList, start uint64) []uint64 {
	r := roaring64.New()
	x.Check(FromPostingList(r, plist))
	r.RemoveRange(0, start)
	return r.ToArray()
}

// RemoveRange would remove [from, to] from bm.
func RemoveRange(bm *roaring64.Bitmap, from, to uint64) {
	bm.RemoveRange(from, to)
	bm.Remove(to)
}

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

// DecodeToBuffer is the same as Decode but it returns a z.Buffer which is
// calloc'ed and can be SHOULD be freed up by calling buffer.Release().
func DecodeToBuffer(bm *roaring64.Bitmap) *z.Buffer {
	buf, err := z.NewBufferWith(256<<20, 32<<30, z.UseCalloc, "Codec.DecodeToBuffer")
	x.Check(err)
	buf.AutoMmapAfter(1 << 30)

	var last uint64
	tmp := make([]byte, 16)
	itr := bm.ManyIterator()
	uids := make([]uint64, 64)
	for {
		got := itr.NextMany(uids)
		if got == 0 {
			break
		}
		for _, u := range uids[:got] {
			n := binary.PutUvarint(tmp, u-last)
			x.Check2(buf.Write(tmp[:n]))
			last = u
		}
	}
	return buf
}
