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

package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/golang/geo/s2"
	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
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
	parents, cover, err := indexCells(p)
	require.NoError(t, err)
	require.Len(t, parents, MaxCellLevel-MinCellLevel+1)
	c := parents[0]
	if c.Level() != MinCellLevel {
		t.Errorf("Expected cell level %d. Got %d instead.", MinCellLevel, c.Level())
	}
	if c.ToToken() != "808c" {
		t.Errorf("Unexpected cell token %s.", c.ToToken())
	}
	c = parents[len(parents)-1]
	if c.Level() != MaxCellLevel {
		t.Errorf("Expected cell level %d. Got %d instead.", MaxCellLevel, c.Level())
	}
	if c.ToToken() != "808fb9f81" {
		t.Errorf("Unexpected cell token %s.", c.ToToken())
	}
	// check that all cell levels are different
	pc := parents[0]
	for _, c := range parents[1:] {
		if c.Level() <= pc.Level() {
			t.Errorf("Expected cell to have level greater than %d. Got %d", pc.Level(), c.Level())
		}
		pc = c
	}

	// Check that cover only has one item
	require.Len(t, cover, 1)
	c = cover[0]
	require.Equal(t, c.Level(), MaxCellLevel)
	require.Equal(t, c.ToToken(), "808fb9f81")
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
	p, err := loadPolygon("testdata/zip.json")
	require.NoError(t, err)
	parents, cover, err := indexCells(p)
	require.NoError(t, err)
	if len(cover) > MaxCells {
		t.Errorf("Expected less than %d cells. Got %d instead.", MaxCells, len(cover))
	}
	for _, c := range cover {
		if c.Level() > MaxCellLevel || c.Level() < MinCellLevel {
			t.Errorf("Invalid cell level %d.", c.Level())
		}
		require.Contains(t, parents, c)
	}
	require.True(t, len(parents) > len(cover))
}

func TestKeyGeneratorPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	src := ValueForType(BinaryID)
	src.Value = data
	gc, err := Convert(src, GeoID)
	require.NoError(t, err)
	g := gc.Value.(geom.T)

	keys, err := IndexGeoTokens(g)
	require.NoError(t, err)
	require.Len(t, keys, MaxCellLevel-MinCellLevel+1+1) // +1 for the cover
}

func TestKeyGeneratorPolygon(t *testing.T) {
	p, err := loadPolygon("testdata/zip.json")
	require.NoError(t, err)
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	src := ValueForType(BinaryID)
	src.Value = data
	gc, err := Convert(src, GeoID)
	require.NoError(t, err)
	g := gc.Value.(geom.T)

	keys, err := IndexGeoTokens(g)
	require.NoError(t, err)
	require.Len(t, keys, 66)
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
	benchToLoop(b, "testdata/zip.json")
}

func BenchmarkToLoopAruba(b *testing.B) {
	benchToLoop(b, "testdata/aruba.json")
}

func BenchmarkCoverZip_10(b *testing.B) {
	benchCover(b, "testdata/zip.json", 10)
}

func BenchmarkCoverZip_15(b *testing.B) {
	benchCover(b, "testdata/zip.json", 15)
}

func BenchmarkCoverZip_18(b *testing.B) {
	benchCover(b, "testdata/zip.json", 18)
}

func BenchmarkCoverZip_30(b *testing.B) {
	benchCover(b, "testdata/zip.json", 30)
}

func BenchmarkCoverAruba_10(b *testing.B) {
	benchCover(b, "testdata/aruba.json", 10)
}

func BenchmarkCoverAruba_15(b *testing.B) {
	benchCover(b, "testdata/aruba.json", 15)
}

func BenchmarkCoverAruba_18(b *testing.B) {
	benchCover(b, "testdata/aruba.json", 18)
}

func BenchmarkCoverAruba_30(b *testing.B) {
	benchCover(b, "testdata/aruba.json", 30)
}

func BenchmarkKeyGeneratorPoint(b *testing.B) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	if err != nil {
		b.Error(err)
	}

	src := ValueForType(BinaryID)
	src.Value = data
	gc, err := Convert(src, GeoID)
	require.NoError(b, err)
	g := gc.Value.(geom.T)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		IndexGeoTokens(g)
	}
}

func BenchmarkKeyGeneratorPolygon(b *testing.B) {
	p, err := loadPolygon("testdata/zip.json")
	if err != nil {
		b.Error(err)
	}
	data, err := wkb.Marshal(p, binary.LittleEndian)
	if err != nil {
		b.Error(err)
	}

	src := ValueForType(GeoID)
	src.Value = data
	gc, err := Convert(src, GeoID)
	require.NoError(b, err)
	g := gc.Value.(geom.T)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		IndexGeoTokens(g)
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
