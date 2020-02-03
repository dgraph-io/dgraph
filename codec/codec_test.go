/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
	"github.com/stretchr/testify/require"
)

func getUniqueUids(size int) []uint64 {
	uids := make([]uint64, 0, size)
	//var uids []uint64
	uidMap := make(map[uint64]struct{}, size)
	for len(uidMap) < size {
		last := rand.Uint64()
		if _, found := uidMap[last]; !found {
			uids = append(uids, last)
		}
		uidMap[last] = struct{}{}
	}
	return uids
}

func sortedSample(ints []uint64, size int) []uint64 {
	shuffle := append([]uint64(nil), ints...)
	rand.Shuffle(size, func(i, j int) { shuffle[i], shuffle[j] = shuffle[j], shuffle[i] })
	sortedShuffle := shuffle[:size]
	sort.Slice(sortedShuffle, func(i, j int) bool { return sortedShuffle[i] < sortedShuffle[j] })
	return sortedShuffle
}

func TestUidPack(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	// Some edge case tests.
	Encode([]uint64{})
	require.Equal(t, 0, ApproxLen(&pb.UidPack{}))
	require.Equal(t, 0, len(Decode(&pb.UidPack{}, 0)))

	randomUids := getUniqueUids(1 << 21)
	for i := uint(0); i < 22; i += 3 {
		size := rand.Intn(1 << i)
		t.Logf("Testing with size = %d", size)

		expected := sortedSample(randomUids, size)
		pack := Encode(expected)
		require.Equal(t, len(expected), ExactLen(pack))
		actual := Decode(pack, 0)
		require.Equal(t, expected, actual)
	}
}

func BenchmarkUidPack2(b *testing.B) { // FIXME: Name
	rand.Seed(time.Now().UnixNano())
	var actual []uint64

	randomUids := getUniqueUids(1 << 21)

	size := rand.Intn(1 << 21)
	b.Logf("Testing with size = %d", size)

	expected := sortedSample(randomUids, size)
	for n := 0; n < b.N; n++ {
		pack := Encode(expected)
		actual = Decode(pack, 0)
		require.NotEmpty(b, actual)
	}
}

func TestSeek(t *testing.T) {
	N := 10001
	enc := Encoder{}
	for i := 0; i < N; i += 10 {
		enc.Add(uint64(i))
	}
	pack := enc.Done()
	dec := NewDecoder(pack)

	tests := []struct {
		in, out uint64
		whence  seekPos
		empty   bool
	}{
		{in: 0, out: 0, whence: SeekStart},
		{in: 0, out: 0, whence: SeekCurrent},
		{in: 100, out: 100, whence: SeekStart},
		{in: 100, out: 110, whence: SeekCurrent},
		{in: 1000, out: 1000, whence: SeekStart},
		{in: 1000, out: 1010, whence: SeekCurrent},
		{in: 1999, out: 2000, whence: SeekStart},
		{in: 1999, out: 2000, whence: SeekCurrent},
		{in: 1101, out: 1110, whence: SeekStart},
		{in: 1101, out: 1110, whence: SeekCurrent},
		{in: 10000, out: 10000, whence: SeekStart},
		{in: 9999, out: 10000, whence: SeekCurrent},
		{in: uint64(N), empty: true, whence: SeekStart},
		{in: uint64(N), empty: true, whence: SeekCurrent},
		{in: math.MaxUint64, empty: true, whence: SeekStart},
		{in: math.MaxUint64, empty: true, whence: SeekCurrent},
	}

	for _, tc := range tests {
		uids := dec.Seek(tc.in, tc.whence)
		if tc.empty {
			require.Empty(t, uids)
		} else {
			require.Equal(t, tc.out, uids[0])
		}
	}

	/*
		dec.blockIdx = 0
		for i := 100; i < 10000; i += 100 {
			uids := dec.LinearSeek(uint64(i))
			require.Contains(t, uids, uint64(i))
		}
	*/
}

/*
func TestLinearSeek(t *testing.T) {
	N := 10001
	enc := Encoder{}
	for i := 0; i < N; i += 10 {
		enc.Add(uint64(i))
	}
	pack := enc.Done()
	dec := NewDecoder(pack)

	for i := 0; i < 2*N; i += 10 {
		uids := dec.LinearSeek(uint64(i))

		if i < N {
			require.Contains(t, uids, uint64(i))
		} else {
			require.NotContains(t, uids, uint64(i))
		}
	}

	//blockIdx points to last block.
	for i := 0; i < 9990; i += 10 {
		uids := dec.LinearSeek(uint64(i))

		require.NotContains(t, uids, uint64(i))
	}
}
*/

func TestDecoder(t *testing.T) {
	N := 10001
	var expected []uint64
	enc := Encoder{}
	for i := 3; i < N; i += 3 {
		enc.Add(uint64(i))
		expected = append(expected, uint64(i))
	}
	pack := enc.Done()

	dec := NewDecoder(pack)
	for i := 3; i < N; i += 3 {
		uids := dec.Seek(uint64(i), SeekStart)
		require.Equal(t, uint64(i), uids[0])

		uids = dec.Seek(uint64(i-1), SeekStart)
		require.Equal(t, uint64(i), uids[0])

		uids = dec.Seek(uint64(i-2), SeekStart)
		require.Equal(t, uint64(i), uids[0])

		start := i/3 - 1
		actual := Decode(pack, uint64(i))
		require.Equal(t, expected[start:], actual)

	}
}

func BenchmarkGzip(b *testing.B) {
	rand.Seed(time.Now().UnixNano())

	uids := getUniqueUids(1e6)
	b.ResetTimer()
	sz := uint64(len(uids)) * 8

	b.Logf("Dataset Len=%d. Size: %s", len(uids), humanize.Bytes(sz))
	var data []byte
	for i := 0; i < b.N; i++ {
		tmp := make([]byte, binary.MaxVarintLen64)
		var buf bytes.Buffer
		for _, uid := range uids {
			n := binary.PutUvarint(tmp, uid)
			_, err := buf.Write(tmp[:n])
			if err != nil {
				b.Fatalf("Error while writing to buffer: %s", err.Error())
			}
		}

		var out bytes.Buffer
		zw := gzip.NewWriter(&out)
		_, err := zw.Write(buf.Bytes())
		if err != nil {
			b.Fatalf("Error while writing to gzip writer: %s", err.Error())
		}

		data = out.Bytes()
	}
	b.Logf("Output size: %s. Compression: %.2f",
		humanize.Bytes(uint64(len(data))),
		float64(len(data))/float64(sz))
}

func benchmarkUidPackEncode(b *testing.B, blockSize int) {
	rand.Seed(time.Now().UnixNano())

	uids := getUniqueUids(1e6)
	sz := uint64(len(uids)) * 8
	b.Logf("Dataset Len=%d. Size: %s", len(uids), humanize.Bytes(sz))
	b.ResetTimer()

	var data []byte
	for i := 0; i < b.N; i++ {
		pack := Encode(uids)
		out, err := pack.Marshal()
		if err != nil {
			b.Fatalf("Error marshaling uid pack: %s", err.Error())
		}
		data = out
	}
	b.Logf("Output size: %s. Compression: %.2f",
		humanize.Bytes(uint64(len(data))),
		float64(len(data))/float64(sz))
}

func BenchmarkUidPack(b *testing.B) {
	b.Run("encode/128", func(b *testing.B) {
		benchmarkUidPackEncode(b, 128)
	})
	b.Run("encode/256", func(b *testing.B) {
		benchmarkUidPackEncode(b, 256)
	})
	b.Run("decode/128", func(b *testing.B) {
		benchmarkUidPackDecode(b, 128)
	})
	b.Run("decode/256", func(b *testing.B) {
		benchmarkUidPackDecode(b, 256)
	})
}

func benchmarkUidPackDecode(b *testing.B, blockSize int) {
	rand.Seed(time.Now().UnixNano())

	uids := getUniqueUids(1e6)
	sz := uint64(len(uids)) * 8
	b.Logf("Dataset Len=%d. Size: %s", len(uids), humanize.Bytes(sz))

	pack := Encode(uids)
	data, err := pack.Marshal()
	x.Check(err)
	b.Logf("Output size: %s. Compression: %.2f",
		humanize.Bytes(uint64(len(data))),
		float64(len(data))/float64(sz))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Decode(pack, 0)
	}
}

func TestEncoding(t *testing.T) {
	bigInts := make([]uint64, 5)
	bigInts[0] = 0xf000000000000000
	bigInts[1] = 0xf00f000000000000
	bigInts[2] = 0x00f00f0000000000
	bigInts[3] = 0x000f0f0000000000
	bigInts[4] = 0x0f0f0f0f00000000

	rand.Seed(time.Now().UnixNano())
	var lengths = []int{0, 1, 2, 3, 5, 13, 18, 100, 99, 98}

	for tc := 0; tc < len(lengths); tc++ {
		ints := make([]uint64, lengths[tc])

		for i := 0; i < 50 && i < lengths[tc]; i++ {
			ints[i] = uint64(rand.Uint32())
		}

		for i := 50; i < lengths[tc]; i++ {
			ints[i] = uint64(rand.Uint32()) + bigInts[rand.Intn(5)]
		}

		sort.Slice(ints, func(i, j int) bool { return ints[i] < ints[j] })

		encodedInts := Encode(ints)
		decodedInts := Decode(encodedInts, 0)

		require.Equal(t, ints, decodedInts)
	}
}

func newUidPack(data []uint64) *pb.UidPack {
	encoder := Encoder{}
	for _, uid := range data {
		encoder.Add(uid)
	}
	return encoder.Done()
}

func TestCopyUidPack(t *testing.T) {
	pack := newUidPack([]uint64{1, 2, 3, 4, 5})
	copy := CopyUidPack(pack)
	require.Equal(t, Decode(pack, 0), Decode(copy, 0))
}

func TestNumMsb(t *testing.T) {
	// Roaring bitmaps support 32 bit integers, so most significant bits used as the base of a
	// block must cover at least the first 32 bits of a 64 bit UID.
	require.GreaterOrEqual(t, NumMsb, uint8(32))
	require.LessOrEqual(t, NumMsb, uint8(64))
}
