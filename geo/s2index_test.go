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
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/golang/geo/s2"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
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

func TestIndexPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	cu := indexCellsForPoint(p, 0, 30)
	printCells(cu)
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

func TestCover(t *testing.T) {
	p, err := loadPolygon("zip.json")
	if err != nil {
		t.Error(err)
	}
	l, _ := loopFromPolygon(p)
	cu := coverLoop(l, 5, 16, 20)
	printCells(cu)
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

func BenchmarkCoverZip_20(b *testing.B) {
	benchCover(b, "zip.json", 20)
}

func BenchmarkCoverZip_35(b *testing.B) {
	benchCover(b, "zip.json", 35)
}

func BenchmarkCoverAruba_10(b *testing.B) {
	benchCover(b, "aruba.json", 10)
}

func BenchmarkCoverAruba_15(b *testing.B) {
	benchCover(b, "aruba.json", 15)
}

func BenchmarkCoverAruba_20(b *testing.B) {
	benchCover(b, "aruba.json", 20)
}

func BenchmarkCoverAruba_35(b *testing.B) {
	benchCover(b, "aruba.json", 35)
}

func benchCover(b *testing.B, file string, max int) {
	p, err := loadPolygon(file)
	if err != nil {
		b.Error(err)
	}
	l, _ := loopFromPolygon(p)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = coverLoop(l, 6, 16, max)
	}
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
