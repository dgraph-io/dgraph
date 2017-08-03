package main

import (
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// chunkByteSize is the number
	// of bytes per chunk of data.
	chunkByteSize = 262144
)

func read(filename string) []int {
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	fgzip, err := gzip.NewReader(f)
	if err != nil {
		panic(err)
	}
	defer fgzip.Close()

	buf := make([]byte, 4)
	_, err = fgzip.Read(buf)
	if err != nil && err != io.EOF {
		panic(err)
	}
	ndata := binary.LittleEndian.Uint32(buf)

	data := make([]int, ndata)
	for i := range data {
		_, err = fgzip.Read(buf)
		if err != nil && err != io.EOF {
			panic(err)
		}

		data[i] = int(binary.LittleEndian.Uint32(buf))
	}

	return data
}

type chunks struct {
	intSize int

	data   [][]uint64
	length int
}

func chunkify64(data []int) *chunks {
	const chunkLen = chunkByteSize / 8

	nchunks := len(data) / chunkLen
	cdata := make([][]uint64, nchunks)

	n := 0
	for i := range cdata {
		chunk := make([]uint64, chunkLen)
		for j := range chunk {
			chunk[j] = uint64(data[n])
			n++
		}
		cdata[i] = chunk
	}

	return &chunks{64, cdata, n}
}

func benchmarkPack(trials int,
	chunks *chunks,
	fpack func([]uint64) *bp128.PackedInts) int {

	times := make([]int, trials)
	for i := range times {
		start := time.Now()
		for _, c := range chunks.data {
			fpack(c)
		}
		times[i] = int(time.Since(start).Nanoseconds())
	}

	sort.Ints(times)
	tmedian := times[len(times)/2]
	speed := (float64(chunks.length) / float64(tmedian)) * 1e3

	return int(speed)
}

func benchmarkUnpack(trials int,
	chunks *chunks,
	fpack func([]uint64) *bp128.PackedInts,
	funpack func(*bp128.PackedInts, []uint64)) int {

	packed := make([]*bp128.PackedInts, len(chunks.data))
	for i, c := range chunks.data {
		packed[i] = fpack(c)
	}

	out := make([]uint64, chunkByteSize/8)

	times := make([]int, trials)
	for i := range times {
		start := time.Now()
		for _, p := range packed {
			funpack(p, out)
		}
		times[i] = int(time.Since(start).Nanoseconds())
	}

	// Check if both input and output are equal
	for i, c := range chunks.data {
		funpack(packed[i], out)

		for j := 0; j < len(c); j++ {
			if c[j] != out[j] {
				x.Fatalf("Something wrong %+v %+v %+v\n", len(c), len(out), j)
			}
		}
	}

	sort.Ints(times)
	tmedian := times[len(times)/2]
	speed := (float64(chunks.length) / float64(tmedian)) * 1e3

	return int(speed)
}

func fmtBenchmark(name string, speed int) {
	const maxlen = 25
	fmt.Printf("%-*s\t%5d mis\n", maxlen, name, speed)
}

func main() {
	data := read("../data/clustered1M.bin.gz")
	if !sort.IsSorted(sort.IntSlice(data)) {
		panic("test data must be sorted")
	}

	chunks64 := chunkify64(data)
	data = nil

	mis := 0
	const ntrials = 1000

	mis = benchmarkPack(ntrials, chunks64, bp128.DeltaPack)
	fmtBenchmark("BenchmarkDeltaPack64", mis)

	mis = benchmarkUnpack(ntrials, chunks64, bp128.DeltaPack, bp128.DeltaUnpack)
	fmtBenchmark("BenchmarkDeltaUnPack64", mis)
}
