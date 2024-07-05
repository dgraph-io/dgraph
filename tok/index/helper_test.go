/*
 * Copyright 2016-2024 Dgraph Labs, Inc. and Contributors
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
	"crypto/rand"
	"fmt"
	"testing"
	"unsafe"

	c "github.com/dgraph-io/dgraph/tok/constraints"
)

func pointerFloatConversion[T c.Float](encoded []byte, retVal *[]T, floatBits int) {
	floatBytes := floatBits / 8

	// Ensure the byte slice length is a multiple of 8 (size of float64)
	if len(encoded)%floatBytes != 0 {
		fmt.Println("Invalid byte slice length")
		return
	}

	// Create a slice header
	*retVal = *(*[]T)(unsafe.Pointer(&encoded))
}

func littleEndianBytesAsFloatArray[T c.Float](encoded []byte, retVal *[]T, floatBits int) {
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

	*retVal = (*retVal)[:0]
	resultLen := len(encoded) / floatBytes
	if resultLen == 0 {
		return
	}
	for i := 0; i < resultLen; i++ {
		// Assume LittleEndian for encoding since this is
		// the assumption elsewhere when reading from client.
		// See dgraph-io/dgo/protos/api.pb.go
		// See also dgraph-io/dgraph/types/conversion.go
		// This also seems to be the preference from many examples
		// I have found via Google search. It's unclear why this
		// should be a preference.
		if retVal == nil {
			retVal = &[]T{}
		}
		*retVal = append(*retVal, BytesToFloat[T](encoded, floatBits))

		encoded = encoded[(floatBytes):]
	}
}

func BenchmarkFloatConverstion(b *testing.B) {
	num := 1500
	data := make([]byte, 64*num)
	_, err := rand.Read(data)
	if err != nil {
		b.Skip()
	}

	b.Run(fmt.Sprintf("pointerFloat:size=%d", len(data)),
		func(b *testing.B) {

			temp := make([]float64, num)
			for k := 0; k < b.N; k++ {
				pointerFloatConversion[float64](data, &temp, 64)
			}

		})

	b.Run(fmt.Sprintf("littleEndianFloat:size=%d", len(data)),
		func(b *testing.B) {
			temp := make([]float64, num)
			for k := 0; k < b.N; k++ {
				littleEndianBytesAsFloatArray[float64](data, &temp, 64)
			}

		})
}
