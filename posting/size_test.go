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
	"fmt"
	"testing"
	"unsafe"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/assert"
)

type A struct {
	a int64
	b int64
	m map[int64]int64
	c int64
}

func TestSizeOfList(t *testing.T) {
	assert.Equal(t, 11*8, int(unsafe.Sizeof(List{})))

	// var a A
	// fmt.Println(*(*int)(unsafe.Pointer(uintptr(unsafe.Pointer(&a)) + 16)))
	// a.m = make(map[int64]int64)
	// fmt.Println(*(*uint8)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(&a)) + 16)) + 0)))
	// for i := int64(0); i < 14; i++ {
	// 	a.m[i] = i
	// }
	// fmt.Println(*(*uint8)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(&a)) + 16)) + 0)))

	l := &List{}
	l.mutationMap = make(map[uint64]*pb.PostingList)
	for i := 0; i < 100; i++ {
		l.mutationMap[uint64(i)] = nil
	}

	numBuckets := 1 << (*(*uint8)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(uintptr(unsafe.Pointer(l)) + 64)) + 9)))
	fmt.Println(numBuckets)
}
