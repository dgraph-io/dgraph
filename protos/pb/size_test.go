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

package pb

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestDeepSizeStruct(t *testing.T) {
	type threeBytes struct {
		b1 byte
		b2 byte
		b3 byte
	}

	var v threeBytes
	require.Equal(t, 3, int64(unsafe.Sizeof(v)))

	var va [3]threeBytes
	require.Equal(t, 9, int64(unsafe.Sizeof(va)))
}

func TestDeepSizeUidBlock(t *testing.T) {
	var v UidBlock
	require.Equal(t, 9*8, int(unsafe.Sizeof(v)))

	uids := make([]uint64, 1, 10)
	block := &UidBlock{Base: uids[0], NumUids: uint32(len(uids))}
	require.Equal(t, 72, block.DeepSize())
}
