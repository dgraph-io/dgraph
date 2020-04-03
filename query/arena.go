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
	x.AssertTrue(size > 0)
	return &arena{
		n:   1,
		buf: make([]byte, 0, size),
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
	if offset == 0 {
		return nil
	}
	eoffset := offset - 1
	x.AssertTrue(eoffset+3 < len(a.buf))

	// First read length, then read actual buffer.
	l := int(binary.BigEndian.Uint32(a.buf[eoffset : eoffset+4]))
	eoffset += 4
	return a.buf[eoffset : eoffset+l]
}

func (a *arena) size() int {
	return a.n - 1
}

func (a *arena) reset() {
	a.n = 1
	a.buf = a.buf[:0]
}
