/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package posting

import (
	"testing"
	"unsafe"

	"github.com/pkg/profile"
)

func memMapUsage(m map[uint64]int64) int {
	var size int

	// map has maptype and hmap
	// maptype is defined at compile time and is hardcoded in the compiled code.
	// Hence, it doesn't consume any extra memory.
	// Ref: https://dave.cheney.net/2018/05/29/how-the-go-runtime-implements-maps-efficiently-without-generics
	// Now, let's look at hmap stuct.
	// size of hmap struct
	// Ref: https://golang.org/src/runtime/map.go?#L114
	size += 6 * 8

	// A map bucket is 16 + 8*sizeof(key) + 8*sizeof(value) bytes.
	// size of each bucket is 16 + 1 + 1 = 18
	// TODO: This doesn't seem accurate
	if m != nil {
		hh := (*mapHeader)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(&m))))
		size += 16 * ((1 << hh.B) + int(hh.noverflow))
		size += 2 * 8 * hh.count
	}

	// memory consumed by keys and values
	size += len(m) * 16

	return size
}

func TestSizeOfList(t *testing.T) {
	defer profile.Start(profile.MemProfile, profile.ProfilePath(".")).Stop()

	m := make(map[uint64]int64)
	for i := 0; i < 100; i++ {
		m[uint64(i)] = int64(i * 7)
	}
}
