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
	"math"
	"sync"

	"github.com/dgraph-io/ristretto/z"
)

var (
	errArenaFull     = errors.New("arena is full")
	errInvalidOffset = errors.New("arena get performed with invalid get")

	maxArenaSize = math.MaxUint32
)

// arena can used to store []byte. It has one underlying large buffer([]byte). All of []byte to be
// stored in arena are appended to this underlying buffer. For futher optimizations, arena also
// keeps mapping from memhash([]byte) => offset in buffer. This ensure single []byte is put into
// arena only once.
// For now, max size for underlying buffer is limit to math.MaxUint32.
type arena struct {
	buf         []byte
	offsetMap   map[uint64]uint32
	sizeBufPool *sync.Pool
}

// newArena returns arena with initial capacity size.
func newArena(size int) *arena {
	// Start offset from 1, to avoid reading bytes when offset is storing default value(0) in
	// fastJsonNode. Hence append dummy byte.
	buf := make([]byte, 0, size)
	return &arena{
		buf:       append(buf, []byte("a")...),
		offsetMap: make(map[uint64]uint32),
		sizeBufPool: &sync.Pool{
			New: func() interface{} {
				return make([]byte, binary.MaxVarintLen64)
			},
		},
	}
}

// put stores b in arena and returns offset for it. Return offset is always > 0(if no error).
// Note: for now this function can only put buffers such that:
// len(current arena buf) + varint(len(b)) + len(b) <= math.MaxUint32.
func (a *arena) put(b []byte) (uint32, error) {
	// Check if we already have b.
	fp := z.MemHash(b)
	if co, ok := a.offsetMap[fp]; ok {
		return co, nil
	}
	// First put length of buffer(varint encoded), then put actual buffer.
	sizeBuf := (a.sizeBufPool.Get()).([]byte)
	w := binary.PutVarint(sizeBuf, int64(len(b)))
	offset := len(a.buf)
	if int64(len(a.buf)+w+len(b)) > int64(math.MaxUint32) {
		return 0, errArenaFull
	}

	a.buf = append(a.buf, sizeBuf[:w]...)
	a.buf = append(a.buf, b...)

	a.sizeBufPool.Put(sizeBuf)
	a.offsetMap[fp] = uint32(offset) // Store offset in map.
	return uint32(offset), nil
}

func (a *arena) get(offset uint32) []byte {
	// We have only dummy values at offset 0.
	if offset == 0 {
		return nil
	}

	if int64(offset) >= int64(len(a.buf)) {
		// TODO: also check validity of offset.
	}

	// First read length, then read actual buffer.
	size, r := binary.Varint(a.buf[offset:])
	offset += uint32(r)
	return a.buf[offset : offset+uint32(size)]
}
