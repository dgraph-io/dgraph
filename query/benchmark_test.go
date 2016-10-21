/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package query

import (
	"bytes"
	"encoding/gob"
	"io/ioutil"
	"testing"

	"github.com/gogo/protobuf/proto"

	"github.com/dgraph-io/dgraph/query/graph"
)

func benchmarkToJson(file string, b *testing.B) {
	b.ReportAllocs()
	var sg SubGraph
	var l Latency

	f, err := ioutil.ReadFile(file)
	if err != nil {
		b.Error(err)
	}

	buf := bytes.NewBuffer(f)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(&sg)
	if err != nil {
		b.Error(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sg.ToJSON(&l); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToJSON_10_Actor(b *testing.B)      { benchmarkToJson("benchmark/actors10.bin", b) }
func BenchmarkToJSON_10_Director(b *testing.B)   { benchmarkToJson("benchmark/directors10.bin", b) }
func BenchmarkToJSON_100_Actor(b *testing.B)     { benchmarkToJson("benchmark/actors100.bin", b) }
func BenchmarkToJSON_100_Director(b *testing.B)  { benchmarkToJson("benchmark/directors100.bin", b) }
func BenchmarkToJSON_1000_Actor(b *testing.B)    { benchmarkToJson("benchmark/actors1000.bin", b) }
func BenchmarkToJSON_1000_Director(b *testing.B) { benchmarkToJson("benchmark/directors1000.bin", b) }

func benchmarkToPB(file string, b *testing.B) {
	b.ReportAllocs()
	var sg SubGraph
	var l Latency

	f, err := ioutil.ReadFile(file)
	if err != nil {
		b.Error(err)
	}

	buf := bytes.NewBuffer(f)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(&sg)
	if err != nil {
		b.Error(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pb, err := sg.ToProtocolBuffer(&l)
		if err != nil {
			b.Fatal(err)
		}
		r := new(graph.Response)
		r.N = pb
		var c Codec
		if _, err = c.Marshal(r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToPB_10_Actor(b *testing.B)      { benchmarkToPB("benchmark/actors10.bin", b) }
func BenchmarkToPB_10_Director(b *testing.B)   { benchmarkToPB("benchmark/directors10.bin", b) }
func BenchmarkToPB_100_Actor(b *testing.B)     { benchmarkToPB("benchmark/actors100.bin", b) }
func BenchmarkToPB_100_Director(b *testing.B)  { benchmarkToPB("benchmark/directors100.bin", b) }
func BenchmarkToPB_1000_Actor(b *testing.B)    { benchmarkToPB("benchmark/actors1000.bin", b) }
func BenchmarkToPB_1000_Director(b *testing.B) { benchmarkToPB("benchmark/directors1000.bin", b) }

func benchmarkToPBMarshal(file string, b *testing.B) {
	b.ReportAllocs()
	var sg SubGraph
	var l Latency

	f, err := ioutil.ReadFile(file)
	if err != nil {
		b.Error(err)
	}

	buf := bytes.NewBuffer(f)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(&sg)
	if err != nil {
		b.Error(err)
	}
	p, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err = proto.Marshal(p); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToPBMarshal_10_Actor(b *testing.B) {
	benchmarkToPBMarshal("benchmark/actors10.bin", b)
}
func BenchmarkToPBMarshal_10_Director(b *testing.B) {
	benchmarkToPBMarshal("benchmark/directors10.bin", b)
}
func BenchmarkToPBMarshal_100_Actor(b *testing.B) {
	benchmarkToPBMarshal("benchmark/actors100.bin", b)
}
func BenchmarkToPBMarshal_100_Director(b *testing.B) {
	benchmarkToPBMarshal("benchmark/directors100.bin", b)
}
func BenchmarkToPBMarshal_1000_Actor(b *testing.B) {
	benchmarkToPBMarshal("benchmark/actors1000.bin", b)
}
func BenchmarkToPBMarshal_1000_Director(b *testing.B) {
	benchmarkToPBMarshal("benchmark/directors1000.bin", b)
}

func benchmarkToPBUnmarshal(file string, b *testing.B) {
	b.ReportAllocs()
	var sg SubGraph
	var l Latency

	f, err := ioutil.ReadFile(file)
	if err != nil {
		b.Error(err)
	}

	buf := bytes.NewBuffer(f)
	dec := gob.NewDecoder(buf)
	err = dec.Decode(&sg)
	if err != nil {
		b.Error(err)
	}
	p, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		b.Fatal(err)
	}

	pbb, err := proto.Marshal(p)
	if err != nil {
		b.Fatal(err)
	}

	pdu := &graph.Node{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = proto.Unmarshal(pbb, pdu)
		if err != nil {
			b.Fatal(err)
		}

	}
}

func BenchmarkToPBUnmarshal_10_Actor(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/actors10.bin", b)
}
func BenchmarkToPBUnmarshal_10_Director(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/directors10.bin", b)
}
func BenchmarkToPBUnmarshal_100_Actor(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/actors100.bin", b)
}
func BenchmarkToPBUnmarshal_100_Director(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/directors100.bin", b)
}
func BenchmarkToPBUnmarshal_1000_Actor(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/actors1000.bin", b)
}
func BenchmarkToPBUnmarshal_1000_Director(b *testing.B) {
	benchmarkToPBUnmarshal("benchmark/directors1000.bin", b)
}
