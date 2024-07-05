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

func pointerFloatConversion1[T c.Float](encoded []byte, retVal *[]T, floatBits int) {
	floatBytes := floatBits / 8

	// Ensure the byte slice length is a multiple of 8 (size of float64)
	if len(encoded)%floatBytes != 0 {
		fmt.Println("Invalid byte slice length")
		return
	}

	// Create a slice header
	*retVal = *(*[]T)(unsafe.Pointer(&encoded))
}

func pointerFloatConversion[T c.Float](encoded []byte, floatBits int) *[]T {
	floatBytes := floatBits / 8

	// Ensure the byte slice length is a multiple of 8 (size of float64)
	if len(encoded)%floatBytes != 0 {
		fmt.Println("Invalid byte slice length")
		return nil
	}

	// Create a slice header
	header := unsafe.Pointer(&encoded)
	return (*[]T)(header)
}

func BenchmarkFloatConverstion(b *testing.B) {
	num := 1400
	data := make([]byte, 64*num)
	rand.Read(data)

	b.Run(fmt.Sprintf("pointerFloat:size=%d", len(data)),
		func(b *testing.B) {

			temp := make([]float64, num)
			for k := 0; k < b.N; k++ {
				pointerFloatConversion1[float64](data, &temp, 64)
			}

		})
	b.Run(fmt.Sprintf("normalFloat:size=%d", len(data)),
		func(b *testing.B) {
			temp := make([]float64, num)
			temp1 := make([]float64, num)
			for k := 0; k < b.N; k++ {
				pointerFloatConversion1[float64](data, &temp1, 64)
				BytesAsFloatArray[float64](data, &temp, 64)

				for i := 0; i < num; i++ {
					if temp[i] != temp1[i] {
						fmt.Println("diff", temp, temp1)
					}
				}
			}

		})
}
