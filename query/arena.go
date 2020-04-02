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

	"github.com/dgraph-io/dgraph/x"
)

type arena struct {
	n   int
	buf []byte
}

func newArena(size int) *arena {
	return &arena{
		n:   0,
		buf: make([]byte, size),
	}
}

func (a *arena) put(b []byte) int {
	// First put length of buffer, then put actual buffer.
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(b)))
	a.buf = append(a.buf, l[:]...)
	a.buf = append(a.buf, b...)

	offset := a.n
	a.n += 4 + len(b)

	return offset
}

func (a *arena) get(offset int) []byte {
	x.AssertTrue(offset+3 < len(a.buf))

	// First read length, then read actual buffer.
	l := int(binary.BigEndian.Uint32(a.buf[offset : offset+4]))
	offset += 4
	var b []byte
	// TODO: Can we avoid allocating a new slice.
	b = append(b, a.buf[offset:offset+l]...)

	return b
}

func (a *arena) size() int {
	return a.n
}

func (a *arena) reset() {
	a.n = 0
	a.buf = a.buf[:0]
}

