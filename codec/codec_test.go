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
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	humanize "github.com/dustin/go-humanize"
	"github.com/stretchr/testify/require"
)

func getUids(size int) []uint64 {
	var uids []uint64
	last := uint64(rand.Intn(100))
	uids = append(uids, last)
	for i := 1; i < size; i++ {
		last += uint64(rand.Intn(33))
		uids = append(uids, last)
	}
	return uids
}

func TestUidPack(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	// Some edge case tests.
	Encode([]uint64{}, 128)
	require.Equal(t, 0, ApproxLen(&pb.UidPack{}))
	require.Equal(t, 0, len(Decode(&pb.UidPack{}, 0)))

	for i := 0; i < 13; i++ {
		size := rand.Intn(10e6)
		if size < 0 {
			size = 1e6
		}
		t.Logf("Testing with size = %d", size)

		expected := getUids(size)
		pack := Encode(expected, 256)
		for _, block := range pack.Blocks {
			require.True(t, len(block.Deltas) <= 255)
		}
		require.Equal(t, len(expected), ExactLen(pack))
		actual := Decode(pack, 0)
		require.Equal(t, expected, actual)
	}
}

func TestSeek(t *testing.T) {
	N := 10001
	enc := Encoder{BlockSize: 10}
	for i := 0; i < N; i += 10 {
		enc.Add(uint64(i))
	}
	pack := enc.Done()
	dec := Decoder{Pack: pack}

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
}

func TestDecoder(t *testing.T) {
	N := 10001
	var expected []uint64
	enc := Encoder{BlockSize: 10}
	for i := 3; i < N; i += 3 {
		enc.Add(uint64(i))
		expected = append(expected, uint64(i))
	}
	pack := enc.Done()

	dec := Decoder{Pack: pack}
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

	uids := getUids(1e6)
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
			x.Check(err)
		}

		var out bytes.Buffer
		zw := gzip.NewWriter(&out)
		_, err := zw.Write(buf.Bytes())
		x.Check(err)

		data = out.Bytes()
	}
	b.Logf("Output size: %s. Compression: %.2f",
		humanize.Bytes(uint64(len(data))),
		float64(len(data))/float64(sz))
}

func benchmarkUidPackEncode(b *testing.B, blockSize int) {
	rand.Seed(time.Now().UnixNano())

	uids := getUids(1e6)
	sz := uint64(len(uids)) * 8
	b.Logf("Dataset Len=%d. Size: %s", len(uids), humanize.Bytes(sz))
	b.ResetTimer()

	var data []byte
	for i := 0; i < b.N; i++ {
		pack := Encode(uids, blockSize)
		out, err := pack.Marshal()
		x.Check(err)
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

	uids := getUids(1e6)
	sz := uint64(len(uids)) * 8
	b.Logf("Dataset Len=%d. Size: %s", len(uids), humanize.Bytes(sz))

	pack := Encode(uids, blockSize)
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
