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
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/dgraph-io/ristretto/z"
)

var (
	errInvalidOffset = errors.New("arena get performed with invalid offset")

	arenaPool = sync.Pool{
		New: func() interface{} {
			a := newArena(1 << 10)
			return a
		},
	}
)

// arena can used to store []byte. It has one underlying large buffer([]byte). All of []byte to be
// stored in arena are appended to this underlying buffer. For futher optimizations, arena also
// keeps mapping from memhash([]byte) => offset in map. This ensures same []byte is put into
// arena only once.
// For now, max size for underlying buffer is limited to math.MaxUint32.
type arena struct {
	buf       []byte
	offsetMap map[uint64]uint32
}

// newArena returns arena with initial capacity size.
func newArena(size int) *arena {
	// Start offset from 1, to avoid reading bytes when offset is storing default value(0) in
	// fastJsonNode. Hence append dummy byte.
	buf := make([]byte, 0, size)
	return &arena{
		buf:       append(buf, []byte("a")...),
		offsetMap: make(map[uint64]uint32),
	}
}

// put stores b in arena and returns offset for it. Returned offset is always > 0(if no error).
// Note: for now this function can only put buffers such that:
// len(current arena buf) + varint(len(b)) + len(b) <= math.MaxUint32.
func (a *arena) put(b []byte) (uint32, error) {
	// Check if we already have b.
	fp := z.MemHash(b)
	if co, ok := a.offsetMap[fp]; ok {
		return co, nil
	}
	// First put length of buffer(varint encoded), then put actual buffer.
	var sizeBuf [binary.MaxVarintLen64]byte
	w := binary.PutVarint(sizeBuf[:], int64(len(b)))
	offset := len(a.buf)
	if uint64(len(a.buf)+w+len(b)) > math.MaxUint32 {
		msg := fmt.Sprintf("errNotEnoughSpaceArena, curSize: %d, maxSize: %d, bufSize: %d",
			len(a.buf), maxEncodedSize, w+len(b))
		return 0, errors.New(msg)
	}

	a.buf = append(a.buf, sizeBuf[:w]...)
	a.buf = append(a.buf, b...)

	a.offsetMap[fp] = uint32(offset) // Store offset in map.
	return uint32(offset), nil
}

func (a *arena) get(offset uint32) ([]byte, error) {
	// We have only dummy value at offset 0.
	if offset == 0 {
		return nil, nil
	}

	if int64(offset) >= int64(len(a.buf)) {
		return nil, errInvalidOffset
	}

	// First read length, then read actual buffer.
	size, r := binary.Varint(a.buf[offset:])
	offset += uint32(r)
	return a.buf[offset : offset+uint32(size)], nil
}

func (a *arena) reset() {
	a.buf = a.buf[:1]

	for k := range a.offsetMap {
		delete(a.offsetMap, k)
	}
}
