package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"math/rand"
	"testing"
	"time"

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
		require.Equal(t, len(expected), NumUids(pack))
		actual := Decode(pack)
		require.Equal(t, expected, actual)
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
		_ = Decode(pack)
	}
}
