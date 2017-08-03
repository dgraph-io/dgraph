package main

import (
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/robskie/bp128"
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
	if err != nil {
		panic(err)
	}
	ndata := binary.LittleEndian.Uint32(buf)

	data := make([]int, ndata)
	for i := range data {
		_, err = fgzip.Read(buf)
		if err != nil {
			panic(err)
		}

		data[i] = int(binary.LittleEndian.Uint32(buf))
	}

	return data
}

type chunks struct {
	intSize int

	data   []interface{}
	length int
}

func chunkify32(data []int) *chunks {
	const chunkLen = chunkByteSize / 4

	nchunks := len(data) / chunkLen
	cdata := make([]interface{}, nchunks)

	n := 0
	for i := range cdata {
		chunk := []uint32{}
		bp128.MakeAlignedSlice(chunkLen, &chunk)
		for j := range chunk {
			chunk[j] = uint32(data[n])
			n++
		}
		cdata[i] = chunk
	}

	return &chunks{32, cdata, n}
}

func chunkify64(data []int) *chunks {
	const chunkLen = chunkByteSize / 8

	nchunks := len(data) / chunkLen
	cdata := make([]interface{}, nchunks)

	n := 0
	for i := range cdata {
		chunk := []uint64{}
		bp128.MakeAlignedSlice(chunkLen, &chunk)
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
	fpack func(interface{}) *bp128.PackedInts) int {

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
	fpack func(interface{}) *bp128.PackedInts,
	funpack func(*bp128.PackedInts, interface{})) int {

	packed := make([]*bp128.PackedInts, len(chunks.data))
	for i, c := range chunks.data {
		packed[i] = fpack(c)
	}

	var out interface{}
	if chunks.intSize == 32 {
		out = &[]uint32{}
		bp128.MakeAlignedSlice(chunkByteSize/4, out)
	} else if chunks.intSize == 64 {
		out = &[]uint64{}
		bp128.MakeAlignedSlice(chunkByteSize/8, out)
	}

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

		vc := reflect.ValueOf(c)
		vo := reflect.ValueOf(out).Elem()
		for j := 0; j < vc.Len(); j++ {
			if vc.Index(j).Uint() != vo.Index(j).Uint() {
				panic("whoops! something went wrong")
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

	chunks32 := chunkify32(data)
	chunks64 := chunkify64(data)
	data = nil

	mis := 0
	const ntrials = 1000

	mis = benchmarkPack(ntrials, chunks32, bp128.Pack)
	fmtBenchmark("BenchmarkPack32", mis)

	mis = benchmarkUnpack(ntrials, chunks32, bp128.Pack, bp128.Unpack)
	fmtBenchmark("BenchmarkUnPack32", mis)

	mis = benchmarkPack(ntrials, chunks32, bp128.DeltaPack)
	fmtBenchmark("BenchmarkDeltaPack32", mis)

	mis = benchmarkUnpack(ntrials, chunks32, bp128.DeltaPack, bp128.Unpack)
	fmtBenchmark("BenchmarkDeltaUnPack32", mis)

	mis = benchmarkPack(ntrials, chunks64, bp128.Pack)
	fmtBenchmark("BenchmarkPack64", mis)

	mis = benchmarkUnpack(ntrials, chunks64, bp128.Pack, bp128.Unpack)
	fmtBenchmark("BenchmarkUnPack64", mis)

	mis = benchmarkPack(ntrials, chunks64, bp128.DeltaPack)
	fmtBenchmark("BenchmarkDeltaPack64", mis)

	mis = benchmarkUnpack(ntrials, chunks64, bp128.DeltaPack, bp128.Unpack)
	fmtBenchmark("BenchmarkDeltaUnPack64", mis)
}
