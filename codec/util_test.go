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

package codec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const testSize = 100

func TestUidPackIterator(t *testing.T) {
	uids := make([]uint64, 0)
	encoder := Encoder{BlockSize: 10}
	for i := 0; i < testSize; i++ {
		uids = append(uids, uint64(i))
		encoder.Add(uint64(i))
	}
	packedUids := encoder.Done()
	require.Equal(t, testSize, len(uids))
	require.Equal(t, testSize, ExactLen(packedUids))

	it := NewUidPackIterator(packedUids)
	unpackedUids := make([]uint64, 0)
	for ; it.Valid(); it.Next() {
		unpackedUids = append(unpackedUids, it.Get())
	}
	require.Equal(t, testSize, len(unpackedUids))
	require.Equal(t, uids, unpackedUids)
}
