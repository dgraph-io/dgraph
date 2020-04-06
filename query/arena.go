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
)

type arena struct {
	buf []byte
}

func newArena(size int) *arena {
	a := new(arena)

	// Append dummy byte to avoid reading bytes when offset is
	// storing default value 0 in fastJsonNode.
	a.buf = make([]byte, 0, size)
	a.buf = append(a.buf, []byte("a")...)

	return a
}

func (a *arena) put(b []byte) uint32 {
	offset := uint32(len(a.buf)) // TODO: careful here.

	// First put length of buffer, then put actual buffer. Also put length using varint encoding.
	sizeBuf := make([]byte, binary.MaxVarintLen64)
	w := binary.PutVarint(sizeBuf, int64(len(b)))
	a.buf = append(a.buf, sizeBuf[:w]...)
	a.buf = append(a.buf, b...)

	return offset
}

func (a *arena) get(offset uint32) []byte {
	if offset == 0 {
		return nil
	}

	// First read length, then read actual buffer.
	size, r := binary.Varint(a.buf[int(offset):])
	offset += uint32(r)
	// TODO: typecasting int64 to uint32 might not be safe.
	return a.buf[offset : offset+uint32(size)]
}

func (a *arena) size() int {
	return len(a.buf) - 1 // -1 for dummy byte.
}

func (a *arena) reset() {
	a.buf = a.buf[:1]
}
