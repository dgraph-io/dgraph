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
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/gogo/protobuf/proto"

	"github.com/dgraph-io/dgraph/query/graph"
)

func benchmarkHelper(b *testing.B, f func(*testing.B, string)) {
	for _, s := range []string{"actor", "director"} {
		for i := 0; i < 3; i++ {
			label := fmt.Sprintf("%s_%d", s, i)
			filename := fmt.Sprintf("benchmark/%s.%d.gob", s, i)
			b.Run(label, func(b *testing.B) {
				f(b, filename)
			})
		}
	}
}

func BenchmarkToJSON(b *testing.B) {
	benchmarkHelper(b, func(b *testing.B, file string) {
		b.ReportAllocs()
		var sg SubGraph
		var l Latency

		f, err := ioutil.ReadFile(file)
		if err != nil {
			b.Fatal(err)
		}

		buf := bytes.NewBuffer(f)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&sg)
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := sg.ToJSON(&l); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkToPB(b *testing.B) {
	benchmarkHelper(b, func(b *testing.B, file string) {
		b.ReportAllocs()
		var sg SubGraph
		var l Latency

		f, err := ioutil.ReadFile(file)
		if err != nil {
			b.Fatal(err)
		}

		buf := bytes.NewBuffer(f)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&sg)
		if err != nil {
			b.Fatal(err)
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
	})
}

func BenchmarkToPBMarshal(b *testing.B) {
	benchmarkHelper(b, func(b *testing.B, file string) {
		b.ReportAllocs()
		var sg SubGraph
		var l Latency

		f, err := ioutil.ReadFile(file)
		if err != nil {
			b.Fatal(err)
		}

		buf := bytes.NewBuffer(f)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&sg)
		if err != nil {
			b.Fatal(err)
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
	})
}

func BenchmarkToPBUnmarshal(b *testing.B) {
	benchmarkHelper(b, func(b *testing.B, file string) {
		b.ReportAllocs()
		var sg SubGraph
		var l Latency

		f, err := ioutil.ReadFile(file)
		if err != nil {
			b.Fatal(err)
		}

		buf := bytes.NewBuffer(f)
		dec := gob.NewDecoder(buf)
		err = dec.Decode(&sg)
		if err != nil {
			b.Fatal(err)
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
	})
}
