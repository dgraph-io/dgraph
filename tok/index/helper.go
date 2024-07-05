/*
 * Copyright 2016-2023 Dgraph Labs, Inc. and Contributors
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

package index

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

	c "github.com/dgraph-io/dgraph/tok/constraints"
)

// BytesAsFloatArray[T c.Float](encoded) converts encoded into a []T,
// where T is either float32 or float64, depending on the value of floatBits.
// Let floatBytes = floatBits/8. If len(encoded) % floatBytes is
// not 0, it will ignore any trailing bytes, and simply convert floatBytes
// bytes at a time to generate the entries.
// The result is appended to the given retVal slice. If retVal is nil
// then a new slice is created and appended to.
func BytesAsFloatArray[T c.Float](encoded []byte, retVal *[]T, floatBits int) {
	// Unfortunately, this is not as simple as casting the result,
	// and it is also not possible to directly use the
	// golang "unsafe" library to directly do the conversion.
	// The machine where this operation gets run might prefer
	// BigEndian/LittleEndian, but the machine that sent it may have
	// preferred the other, and there is no way to tell!
	//
	// The solution below, unfortunately, requires another memory
	// allocation.
	// TODO Potential optimization: If we detect that current machine is
	// using LittleEndian format, there might be a way of making this
	// work with the golang "unsafe" library.
	floatBytes := floatBits / 8

	// Ensure the byte slice length is a multiple of 8 (size of float64)
	if len(encoded)%floatBytes != 0 {
		fmt.Println("Invalid byte slice length")
		return
	}

	// Create a slice header
	*retVal = *(*[]T)(unsafe.Pointer(&encoded))
	//floatBytes := floatBits / 8

	//*retVal = (*retVal)[:0]
	//resultLen := len(encoded) / floatBytes
	//if resultLen == 0 {
	//	return
	//}
	//for i := 0; i < resultLen; i++ {
	//	// Assume LittleEndian for encoding since this is
	//	// the assumption elsewhere when reading from client.
	//	// See dgraph-io/dgo/protos/api.pb.go
	//	// See also dgraph-io/dgraph/types/conversion.go
	//	// This also seems to be the preference from many examples
	//	// I have found via Google search. It's unclear why this
	//	// should be a preference.
	//	if retVal == nil {
	//		retVal = &[]T{}
	//	}
	//	*retVal = append(*retVal, BytesToFloat[T](encoded, floatBits))

	//	encoded = encoded[(floatBytes):]
	//}
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
