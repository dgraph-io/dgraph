/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package geo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/golang/geo/s2"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgraph/types"
)

func loadPolygon(name string) (*geom.Polygon, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var b bytes.Buffer
	_, err = io.Copy(&b, f)
	if err != nil {
		return nil, err
	}

	var gf geojson.Feature
	if err := gf.UnmarshalJSON(b.Bytes()); err != nil {
		return nil, err
	}
	if p, ok := gf.Geometry.(*geom.Polygon); ok {
		return p, nil
	}
	return nil, fmt.Errorf("Did not load a polygon from the json.")
}

func TestIndexCellsPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	cu, err := indexCells(types.Geo{p})
	if err != nil {
		t.Error(err)
	}
	if len(cu) != (MaxCellLevel - MinCellLevel + 1) {
		t.Errorf("Expected 12 cells. Got %d instead.", len(cu))
	}
	c := cu[0]
	if c.Level() != MinCellLevel {
		t.Errorf("Expected cell level %d. Got %d instead.", MinCellLevel, c.Level())
	}
	if c.ToToken() != "808c" {
		t.Errorf("Unexpected cell token %s.", c.ToToken())
	}
	c = cu[len(cu)-1]
	if c.Level() != MaxCellLevel {
		t.Errorf("Expected cell level %d. Got %d instead.", MaxCellLevel, c.Level())
	}
	if c.ToToken() != "808fb9f81" {
		t.Errorf("Unexpected cell token %s.", c.ToToken())
	}
	// check that all cell levels are different
	pc := cu[0]
	for _, c := range cu[1:] {
		if c.Level() <= pc.Level() {
			t.Errorf("Expected cell to have level greater than %d. Got %d", pc.Level(), c.Level())
		}
		pc = c
	}
}

func printCells(cu s2.CellUnion) {
	for _, c := range cu {
		cell := s2.CellFromCellID(c)
		area := EarthArea(cell.ExactArea())
		r := cell.RectBound()
		top := r.Vertex(0).Distance(r.Vertex(1))
		side := r.Vertex(1).Distance(r.Vertex(2))
		fmt.Printf("Level: %d, Cell: %s, area: %s, boundary: %s x %s\n", c.Level(), c.ToToken(),
			area, EarthDistance(top), EarthDistance(side))
	}
}

func TestIndexCellsPolygon(t *testing.T) {
	p, err := loadPolygon("zip.json")
	if err != nil {
		t.Error(err)
	}
	cu, err := indexCells(types.Geo{p})
	if err != nil {
		t.Error(err)
	}
	if len(cu) > MaxCells {
		t.Errorf("Expected less than %d cells. Got %d instead.", MaxCells, len(cu))
	}
	for _, c := range cu {
		if c.Level() > MaxCellLevel || c.Level() < MinCellLevel {
			t.Errorf("Invalid cell level %d.", c.Level())
		}
	}
}

func TestKeyGeneratorPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	if err != nil {
		t.Error(err)
	}

	keys, err := IndexKeys(data)
	if err != nil {
		t.Error(err)
	}
	if len(keys) != (MaxCellLevel - MinCellLevel + 1) {
		t.Errorf("Expected 12 keys. Got %d instead.", len(keys))
	}
	for _, key := range keys {
		if !strings.HasPrefix(string(key), ":_loc_|") {
			t.Errorf("Expected prefix ':_loc_|' for key %s", key)
		}
	}
}

func TestKeyGeneratorPolygon(t *testing.T) {
	p, err := loadPolygon("zip.json")
	if err != nil {
		t.Error(err)
	}
	data, err := wkb.Marshal(p, binary.LittleEndian)
	if err != nil {
		t.Error(err)
	}

	keys, err := IndexKeys(data)
	if err != nil {
		t.Error(err)
	}
	if len(keys) != 18 {
		t.Errorf("Expected 18 keys. Got %d instead.", len(keys))
	}
	for _, key := range keys {
		if !strings.HasPrefix(string(key), ":_loc_|") {
			t.Errorf("Expected prefix ':_loc_|' for key %s", key)
		}
	}
}

func testCover(file string, max int) {
	fmt.Printf("Testing %s with max %d\n", file, max)
	p, err := loadPolygon(file)
	if err != nil {
		return
	}
	l, _ := loopFromPolygon(p)
	cu := coverLoop(l, MinCellLevel, MaxCellLevel, max)
	printCells(cu)
	printCoverAccuracy(l, cu)
}

func printCoverAccuracy(l *s2.Loop, cu s2.CellUnion) {
	a1 := cellUnionArea(cu)
	a2 := loopArea(l)
	fmt.Printf("Loop area: %v. Cell area %v. Ratio %.3f\n", EarthArea(a2), EarthArea(a1), a1/a2)
}

func cellUnionArea(cu s2.CellUnion) float64 {
	var area float64
	for _, c := range cu {
		cell := s2.CellFromCellID(c)
		area += cell.ExactArea()
	}
	return area
}

func loopArea(l *s2.Loop) float64 {
	n := l.NumEdges()
	origin := l.Vertex(0)
	var area float64
	for i := 1; i < n-1; i++ {
		area += s2.PointArea(origin, l.Vertex(i), l.Vertex(i+1)) * float64(s2.RobustSign(origin, l.Vertex(i), l.Vertex(i+1)))
	}
	return area
}

func BenchmarkToLoopZip(b *testing.B) {
	benchToLoop(b, "zip.json")
}

func BenchmarkToLoopAruba(b *testing.B) {
	benchToLoop(b, "aruba.json")
}

func BenchmarkCoverZip_10(b *testing.B) {
	benchCover(b, "zip.json", 10)
}

func BenchmarkCoverZip_15(b *testing.B) {
	benchCover(b, "zip.json", 15)
}

func BenchmarkCoverZip_18(b *testing.B) {
	benchCover(b, "zip.json", 18)
}

func BenchmarkCoverZip_30(b *testing.B) {
	benchCover(b, "zip.json", 30)
}

func BenchmarkCoverAruba_10(b *testing.B) {
	benchCover(b, "aruba.json", 10)
}

func BenchmarkCoverAruba_15(b *testing.B) {
	benchCover(b, "aruba.json", 15)
}

func BenchmarkCoverAruba_18(b *testing.B) {
	benchCover(b, "aruba.json", 18)
}

func BenchmarkCoverAruba_30(b *testing.B) {
	benchCover(b, "aruba.json", 30)
}

func BenchmarkKeyGeneratorPoint(b *testing.B) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	if err != nil {
		b.Error(err)
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		IndexKeys(data)
	}
}

func BenchmarkKeyGeneratorPolygon(b *testing.B) {
	p, err := loadPolygon("zip.json")
	if err != nil {
		b.Error(err)
	}
	data, err := wkb.Marshal(p, binary.LittleEndian)
	if err != nil {
		b.Error(err)
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		IndexKeys(data)
	}
}

func benchCover(b *testing.B, file string, max int) {
	p, err := loadPolygon(file)
	if err != nil {
		b.Error(err)
	}
	l, _ := loopFromPolygon(p)
	var cu s2.CellUnion
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		cu = coverLoop(l, MinCellLevel, MaxCellLevel, max)
	}
	printCoverAccuracy(l, cu)
}

func benchToLoop(b *testing.B, file string) {
	p, err := loadPolygon(file)
	if err != nil {
		b.Error(err)
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, _ = loopFromPolygon(p)
	}
}
