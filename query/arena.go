/*
 * Copyright 2017-2020 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"encoding/binary"
	"math"

	"github.com/dgraph-io/ristretto/z"
)

type arena struct {
	buf       []byte
	offsetMap map[uint64]uint32
}

func newArena(size int) *arena {
	// TODO: return error from here.
	// x.AssertTruef(int64(size) < int64(math.MaxUint32), "size should be < math.MaxUint32 at init.")
	a := new(arena)

	// Append dummy byte to avoid reading bytes when offset is
	// storing default value 0 in fastJsonNode.
	a.buf = make([]byte, 0, size)
	a.buf = append(a.buf, []byte("a")...)
	a.offsetMap = make(map[uint64]uint32)
	return a
}

// put appends b in arena. It first checks if last buffer in arena has enough space for b.
// If not, it appends new buffer in arena's buffers.
// Note: for now this function can only put buffers such that:
// varint(len(b)) + len(b) < math.MaxUint32.
func (a *arena) put(b []byte) uint32 {
	// Check if we already have b.
	fp := z.MemHash(b)
	if co, ok := a.offsetMap[fp]; ok {
		return co
	}
	// First put length of buffer(varint encoded), then put actual buffer.
	sizeBuf := make([]byte, binary.MaxVarintLen64)
	w := binary.PutVarint(sizeBuf, int64(len(b)))
	offset := len(a.buf)
	if int64(len(a.buf)+w+len(b)) > int64(math.MaxUint32) {
		// Panic for now once we have filled the buffer.
		// TODO: fix this.
		panic("underlying buffer is full in arena")
	}

	a.buf = append(a.buf, sizeBuf[:w]...)
	a.buf = append(a.buf, b...)

	// Store offset in map.
	a.offsetMap[fp] = uint32(offset)
	return uint32(offset)
}

func (a *arena) get(offset uint32) []byte {
	// We have only dummy values at offset 0.
	if offset == 0 {
		return nil
	}

	// First read length, then read actual buffer.
	size, r := binary.Varint(a.buf[offset:])
	offset += uint32(r)
	return a.buf[offset : offset+uint32(size)]
}
