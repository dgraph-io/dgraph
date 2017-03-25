/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package query

/*
// TODO: Fix this test.
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

func buildValueList(data []string) *taskp.ValueList {
	b := flatbuffers.NewBuilder(0)
	offsets := make([]flatbuffers.UOffsetT, 0, len(data))
	for _, s := range data {
		bvo := b.CreateString(s)
		taskp.ValueStart(b)
		taskp.ValueAddVal(b, bvo)
		offsets = append(offsets, taskp.ValueEnd(b))
	}

	taskp.ValueListStartValuesVector(b, len(data))
	for i := 0; i < len(data); i++ {
		b.PrependUOffsetT(offsets[i])
	}
	voffset := b.EndVector(len(data))

	taskp.ValueListStart(b)
	taskp.ValueListAddValues(b, voffset)
	b.Finish(taskp.ValueListEnd(b))
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
*/
