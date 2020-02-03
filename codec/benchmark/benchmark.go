// The MIT License (MIT)

// Copyright (c) 2015-2016 robskie <mrobskie@gmail.com>

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/dgraph-io/dgraph/codec"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
)

// So, with SIMD instructions before, we were getting great performance, but at the cost of
// complexity and bugs.
// $ go build . && ./benchmark
// BenchmarkDeltaPack64     	  551 mis
// BenchmarkDeltaUnPack64   	 2330 mis
//
// With the new simple codec, the code is simple and easy to work with, but we're getting quite poor
// performance. So, sometime later, it would be good to come back in here, and try and improve the
// performance of decoding.
// $ go build . && ./benchmark
// BenchmarkDeltaPack64     	   30 mis
// BenchmarkDeltaUnPack64   	  116 mis
// TODO: Improve performance here, using SIMD instructions.

const (
	// chunkByteSize is the number
	// of bytes per chunk of data.
	chunkByteSize = 262144
)

func read(filename string) []int {
	f, err := os.Open(filename)
	if err != nil {
		x.Panic(err)
	}
	defer f.Close()

	fgzip, err := gzip.NewReader(f)
	if err != nil {
		x.Panic(err)
	}
	defer fgzip.Close()

	buf := make([]byte, 4)
	_, err = fgzip.Read(buf)
	if err != nil && err != io.EOF {
		x.Panic(err)
	}
	ndata := binary.LittleEndian.Uint32(buf)

	data := make([]int, ndata)
	for i := range data {
		_, err = fgzip.Read(buf)
		if err != nil && err != io.EOF {
			x.Panic(err)
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

func benchmarkPack(trials int, chunks *chunks) int {
	times := make([]int, trials)
	for i := range times {
		start := time.Now()
		for _, c := range chunks.data {
			codec.Encode(c)
			// bp128.DeltaPack(c)
		}
		times[i] = int(time.Since(start).Nanoseconds())
	}

	sort.Ints(times)
	tmedian := times[len(times)/2]
	speed := (float64(chunks.length) / float64(tmedian)) * 1e3

	return int(speed)
}

func benchmarkUnpack(trials int, chunks *chunks) int {
	packed := make([]*pb.UidPack, len(chunks.data))
	for i, c := range chunks.data {
		packed[i] = codec.Encode(c)
	}

	times := make([]int, trials)
	for i := range times {
		start := time.Now()
		for _, p := range packed {
			dec := codec.NewDecoder(p)
			for uids := dec.Seek(0, 0); len(uids) > 0; uids = dec.Next() {
			}
		}
		times[i] = int(time.Since(start).Nanoseconds())
	}

	// Check if both input and output are equal
	for i, c := range chunks.data {
		out := codec.Decode(packed[i], 0)

		for j := 0; j < len(c); j++ {
			if c[j] != out[j] {
				x.Fatalf("Something wrong %+v \n%+v\n %+v\n", len(c), len(out), j)
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
	data := read("clustered1M.bin.gz")
	if !sort.IsSorted(sort.IntSlice(data)) {
		x.Panic(errors.New("test data must be sorted"))
	}

	chunks64 := chunkify64(data)
	mis := 0
	const ntrials = 100

	mis = benchmarkPack(ntrials, chunks64)
	fmtBenchmark("BenchmarkDeltaPack64", mis)

	mis = benchmarkUnpack(ntrials, chunks64)
	fmtBenchmark("BenchmarkDeltaUnPack64", mis)
}
