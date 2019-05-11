/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
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

package x

import (
	"encoding/binary"
)

// ProtoMessage is an interface to interact with protobuf messages.
type ProtoMessage interface {
	Size() int
	MarshalTo([]byte) (int, error)
}

// AppendProtoMsg appends the given protobuf message to the given byte slice.
func AppendProtoMsg(p []byte, msg ProtoMessage) ([]byte, error) {
	sz := msg.Size()
	p = ReserveCap(p, len(p)+sz)
	buf := p[len(p) : len(p)+sz]
	n, err := msg.MarshalTo(buf)
	AssertTrue(sz == n)
	return p[:len(p)+sz], err
}

// AppendUvarint appends the given uint64 to the given byte slice.
func AppendUvarint(p []byte, x uint64) []byte {
	p = ReserveCap(p, len(p)+binary.MaxVarintLen64)
	buf := p[len(p) : len(p)+binary.MaxVarintLen64]
	n := binary.PutUvarint(buf, x)
	return p[:len(p)+n]
}

// ReserveCap returns a new byte slice containing the contents of the given one with
// a capacity of atLeast. The original byte slice is returned if it meets the capacity
// requirements.
func ReserveCap(p []byte, atLeast int) []byte {
	if cap(p) >= atLeast {
		return p
	}
	newCap := cap(p) * 2
	if newCap < atLeast {
		newCap = atLeast
	}
	newP := make([]byte, len(p), newCap)
	copy(newP, p)
	return newP
}
