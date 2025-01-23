/*
 * Copyright 2016-2025 Hypermode Inc. and Contributors
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
 *
 * Co-authored by: jairad26@gmail.com, sunil@hypermode.com, bill@hypdermode.com
 */

package index

import (
	"encoding/binary"
	"math"
	"reflect"
	"unsafe"

	"github.com/golang/glog"
	c "github.com/hypermodeinc/dgraph/v24/tok/constraints"
)

// BytesAsFloatArray[T c.Float](encoded) converts encoded into a []T,
// where T is either float32 or float64, depending on the value of floatBits.
// Let floatBytes = floatBits/8. If len(encoded) % floatBytes is
// not 0, it will ignore any trailing bytes, and simply convert floatBytes
// bytes at a time to generate the entries.
// The result is appended to the given retVal slice. If retVal is nil
// then a new slice is created and appended to.
func BytesAsFloatArray[T c.Float](encoded *[]byte, retVal *[]T, floatBits int) {
	floatBytes := floatBits / 8

	if len(*encoded) == 0 {
		*retVal = []T{}
		return
	}

	// Ensure the byte slice length is a multiple of 8 (size of float64)
	if len(*encoded)%floatBytes != 0 {
		glog.Errorf("Invalid byte slice length %d %v", len(*encoded), *encoded)
		return
	}

	if retVal == nil {
		*retVal = make([]T, len(*encoded)/floatBytes)
	}
	*retVal = (*retVal)[:0]
	header := (*reflect.SliceHeader)(unsafe.Pointer(retVal))
	header.Data = uintptr(unsafe.Pointer(&(*encoded)[0]))
	header.Len = len(*encoded) / floatBytes
	header.Cap = len(*encoded) / floatBytes
}

func BytesToFloat[T c.Float](encoded []byte, floatBits int) T {
	if floatBits == 32 {
		bits := binary.LittleEndian.Uint32(encoded)
		return T(math.Float32frombits(bits))
	} else if floatBits == 64 {
		bits := binary.LittleEndian.Uint64(encoded)
		return T(math.Float64frombits(bits))
	}
	panic("Invalid floatBits")
}
