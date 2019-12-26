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

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

const testSize = 100
const benchmarkSize = 1e7

func newUidPack(data []uint64) *pb.UidPack {
	encoder := Encoder{BlockSize: 10}
	for _, uid := range data {
		encoder.Add(uid)
	}
	return encoder.Done()
}

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

func TestCopyUidPack(t *testing.T) {
	pack := newUidPack([]uint64{1, 2, 3, 4, 5})
	copy := CopyUidPack(pack)
	require.Equal(t, Decode(pack, 0), Decode(copy, 0))
}

func BenchmarkIterationNormal(b *testing.B) {
	b.StopTimer()
	encoder := Encoder{BlockSize: 100}
	for i := 0; i < benchmarkSize; i++ {
		encoder.Add(uint64(i))
	}
	packedUids := encoder.Done()

	decoder := Decoder{Pack: packedUids}
	decoder.Seek(0, SeekStart)
	unpackedUids := make([]uint64, 0)

	b.StartTimer()
	for ; decoder.Valid(); decoder.Next() {
		for _, uid := range decoder.Uids() {
			unpackedUids = append(unpackedUids, uid)
		}
	}
}

func BenchmarkIterationWithUtil(b *testing.B) {
	b.StopTimer()
	encoder := Encoder{BlockSize: 100}
	for i := 0; i < benchmarkSize; i++ {
		encoder.Add(uint64(i))
	}
	packedUids := encoder.Done()

	b.StartTimer()
	it := NewUidPackIterator(packedUids)
	unpackedUids := make([]uint64, 0)
	for ; it.Valid(); it.Next() {
		unpackedUids = append(unpackedUids, it.Get())
	}
}
