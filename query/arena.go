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

	"github.com/dgraph-io/dgraph/x"
)

const (
	minSize = 1 * 1024 // 1 KB
)

type arena struct {
	bufs [][]byte
}

func newArena(size int) *arena {
	x.AssertTruef(int64(size) < int64(math.MaxUint32), "size should be < math.MaxUint32 at init.")
	a := new(arena)

	// Append dummy byte to avoid reading bytes when offset is
	// storing default value 0 in fastJsonNode.
	buf := make([]byte, 0, size)
	buf = append(buf, []byte("a")...)
	a.bufs = append(a.bufs, buf)
	return a
}

// put appends b in arena. It first checks if last buffer in arena has enough space for b.
// If not, it appends new buffer in arena's buffers.
// Note: for now this function can only put buffers such that:
// varint(len(b)) + len(b) < math.MaxUint32.
func (a *arena) put(b []byte) (uint8, uint32) {
	// First put length of buffer(varint encoded), then put actual buffer.
	sizeBuf := make([]byte, binary.MaxVarintLen64)
	w := binary.PutVarint(sizeBuf, int64(len(b)))

	// If last buffer doesn't have enough space, get a new one.
	last := len(a.bufs) - 1
	offset := len(a.bufs[last])

	if int64(offset+w+len(b)) > int64(math.MaxUint32) {
		buf := make([]byte, 0, minSize)
		buf = append(buf, []byte("a")...)
		a.bufs = append(a.bufs, buf)
		last = len(a.bufs) - 1
		offset = 1
		x.AssertTruef(last < int(math.MaxUint8),
			"Number of bufs in arena should be < math.MaxUint8")
		x.AssertTruef(int64(offset+w+len(b)) < int64(math.MaxUint32),
			"varint(len(buf))+len(buf): %d is too large for arena", w+len(b))
	}

	a.bufs[last] = append(a.bufs[last], sizeBuf[:w]...)
	a.bufs[last] = append(a.bufs[last], b...)

	return uint8(last), uint32(offset)
}

func (a *arena) get(idx uint8, offset uint32) []byte {
	// We have only dummy values at offset 0 in all arena bufs.
	if offset == 0 {
		return nil
	}

	// First read length, then read actual buffer.
	size, r := binary.Varint(a.bufs[idx][offset:])
	offset += uint32(r)
	return a.bufs[idx][offset : offset+uint32(size)]
}
