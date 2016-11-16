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
	"time"

	"github.com/google/flatbuffers/go"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func prepareTest(b *testing.B) (*store.Store, string, string) {
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		b.Fatal(err)
	}

	ps, err := store.NewStore(dir)
	if err != nil {
		b.Fatal(err)
	}

	schema.ParseBytes([]byte(""))
	posting.Init(ps)
	worker.Init(ps)

	group.ParseGroupConfig("")
	dir2, err := ioutil.TempDir("", "wal_")
	if err != nil {
		b.Fatal(err)
	}
	worker.StartRaftNodes(dir2)
	time.Sleep(3 * time.Second)
	return ps, dir, dir2
}

func buildValueList(data []string) *x.ValueList {
	b := flatbuffers.NewBuilder(0)
	offsets := make([]flatbuffers.UOffsetT, 0, len(data))
	for _, s := range data {
		bvo := b.CreateString(s)
		task.ValueStart(b)
		task.ValueAddVal(b, bvo)
		offsets = append(offsets, task.ValueEnd(b))
	}

	task.ValueListStartValuesVector(b, len(data))
	for i := 0; i < len(data); i++ {
		b.PrependUOffsetT(offsets[i])
	}
	voffset := b.EndVector(len(data))

	task.ValueListStart(b)
	task.ValueListAddValues(b, voffset)
	b.Finish(task.ValueListEnd(b))
	buf := b.FinishedBytes()

	out := new(x.ValueList)
	x.Check(out.UnmarshalBinary(buf))
	return out
}

// benchmarkHelper runs against some data from benchmark folder.
func benchmarkHelper(b *testing.B, f func(*testing.B, *SubGraph)) {
	for _, s := range []string{"actor", "director"} {
		for i := 0; i < 3; i++ {
			label := fmt.Sprintf("%s_%d", s, i)
			filename := fmt.Sprintf("benchmark/%s.%d.gob", s, i)
			if !b.Run(label, func(b *testing.B) {
				b.ReportAllocs()
				sg := new(SubGraph)
				data, err := ioutil.ReadFile(filename)
				if err != nil {
					b.Fatal(err)
				}
				buf := bytes.NewBuffer(data)
				dec := gob.NewDecoder(buf)
				err = dec.Decode(sg)
				if err != nil {
					b.Fatal(err)
				}
				f(b, sg)
			}) {
				b.FailNow()
			}
		}
	}
}

func BenchmarkToJSON(b *testing.B) {
	benchmarkHelper(b, func(b *testing.B, sg *SubGraph) {
		var l Latency
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := sg.ToJSON(&l); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkToProto(b *testing.B) {
	benchmarkHelper(b, func(b *testing.B, sg *SubGraph) {
		var l Latency
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

func valueSubGraph(attr string, uid []uint64, data []string) *SubGraph {
	x.AssertTrue(len(uid) == len(data))
	var r []*algo.UIDList
	for i := 0; i < len(uid); i++ {
		r = append(r, algo.NewUIDList([]uint64{}))
	}
	return &SubGraph{
		Attr:    attr,
		SrcUIDs: algo.NewUIDList(uid),
		Values:  buildValueList(data),
		Result:  r,
	}
}

func uint64Range(a, b int) *algo.UIDList {
	var out []uint64
	for i := a; i < b; i++ {
		out = append(out, uint64(i))
	}
	return algo.NewUIDList(out)
}

func leafSubGraphs(srcUID []uint64) []*SubGraph {
	var children []*SubGraph
	data := make([]string, 0, len(srcUID))
	for i := 0; i < len(srcUID); i++ {
		data = append(data, "somedata")
	}
	for i := 0; i < 50; i++ {
		children = append(children, valueSubGraph(
			fmt.Sprintf("attr%d", i), srcUID, data))
	}
	return children
}

const uidStart = 10000
const uidEnd = 15000

// sampleSubGraph creates a subgraph with given number of unique descendents.
func sampleSubGraph(numUnique int) *SubGraph {
	var r []*algo.UIDList
	for i := uidStart; i < uidEnd; i++ {
		r = append(r, algo.NewUIDList([]uint64{uint64(100000000 + (i % numUnique))}))
	}

	var destUID []uint64
	for i := 0; i < numUnique; i++ {
		destUID = append(destUID, uint64(100000000+i))
	}

	c := &SubGraph{
		Attr:     "aaa",
		SrcUIDs:  uint64Range(uidStart, uidEnd),
		DestUIDs: algo.NewUIDList(destUID),
		Values:   createNilValuesList(len(r)),
		Result:   r,
		Children: leafSubGraphs(destUID),
	}

	d := &SubGraph{
		Attr:     "bbb",
		SrcUIDs:  algo.NewUIDList([]uint64{1}),
		DestUIDs: uint64Range(uidStart, uidEnd),
		Result:   []*algo.UIDList{uint64Range(uidStart, uidEnd)},
		Values:   createNilValuesList(1),
		Children: []*SubGraph{c},
	}

	return &SubGraph{
		Attr:     "ignore",
		SrcUIDs:  algo.NewUIDList([]uint64{1}),
		DestUIDs: algo.NewUIDList([]uint64{1}),
		Values:   createNilValuesList(1),
		Result:   []*algo.UIDList{algo.NewUIDList([]uint64{1})},
		Children: []*SubGraph{d},
	}
}

func BenchmarkToProtoSynthetic(b *testing.B) {
	for _, numUnique := range []int{1, 1000, 2000, 3000, 4000, 5000} {
		b.Run(fmt.Sprintf("unique%d", numUnique), func(b *testing.B) {
			b.ReportAllocs()
			sg := sampleSubGraph(numUnique)
			var l Latency
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
}

func BenchmarkToJSONSynthetic(b *testing.B) {
	for _, numUnique := range []int{1, 1000, 2000, 3000, 4000, 5000} {
		b.Run(fmt.Sprintf("unique%d", numUnique), func(b *testing.B) {
			b.ReportAllocs()
			sg := sampleSubGraph(numUnique)
			var l Latency
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := sg.ToJSON(&l); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
