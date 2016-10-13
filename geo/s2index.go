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
	"log"

	"github.com/golang/geo/s2"
	"github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// IndexKeys creates the keys to use in the index for the given data assuming that it is geo data.
func IndexKeys(val *types.Geo) ([][]byte, error) {
	// Try to parse the data as geo type.
	return IndexKeysFromGeo(*val)
}

// IndexKeysFromGeo returns the keys to be used in a geospatial index for the given geometry. If the
// geometry is not supported it returns an error.
func IndexKeysFromGeo(g types.Geo) ([][]byte, error) {
	cu, err := indexCells(g)
	if err != nil {
		return nil, err
	}
	keys := make([][]byte, len(cu))
	for i, c := range cu {
		keys[i] = indexKeyFromCellID(c)
	}
	return keys, nil
}

const indexPrefix = ":_loc_|"

func indexKeyFromCellID(c s2.CellID) []byte {
	var buf bytes.Buffer
	buf.WriteString(indexPrefix)
	buf.WriteString(c.ToToken())
	return buf.Bytes()
}

func indexCells(g types.Geo) (s2.CellUnion, error) {
	if g.Stride() != 2 {
		return nil, x.Errorf("Covering only available for 2D co-ordinates.")
	}
	switch v := g.T.(type) {
	case *geom.Point:
		return indexCellsForPoint(v, MinCellLevel, MaxCellLevel), nil
	case *geom.Polygon:
		l, err := loopFromPolygon(v)
		if err != nil {
			return nil, err
		}
		return coverLoop(l, MinCellLevel, MaxCellLevel, MaxCells), nil
	default:
		return nil, x.Errorf("Cannot index geometry of type %T", v)
	}
}

const (
	// MinCellLevel is the smallest cell level (largest cell size) used by indexing
	MinCellLevel = 5 // Approx 250km x 380km
	// MaxCellLevel is the largest cell leve (smallest cell size) used by indexing
	MaxCellLevel = 16 // Approx 120m x 180m
	// MaxCells is the maximum number of cells to use when indexing regions.
	MaxCells = 18
)

func pointFromCoord(r geom.Coord) s2.Point {
	// The geojson spec says that coordinates are specified as [long, lat]
	// We assume that any data encoded in the database follows that format.
	ll := s2.LatLngFromDegrees(r.Y(), r.X())
	return s2.PointFromLatLng(ll)
}

// We use loops instead of s2.Polygon as the s2.Polygon implemention is incomplete.
func loopFromPolygon(p *geom.Polygon) (*s2.Loop, error) {
	// go implementation of s2 does not support more than one loop (and will panic if the size of
	// the loops array > 1). So we will skip the holes in the polygon and just use the outer loop.
	r := p.LinearRing(0)
	n := r.NumCoords()
	if n < 4 {
		return nil, x.Errorf("Can't convert ring with less than 4 pts")
	}
	// S2 specifies that the orientation of the polygons should be CCW. However there is no
	// restriction on the orientation in WKB (or geojson). To get the correct orientation we assume
	// that the polygons are always less than one hemisphere. If they are bigger, we flip the
	// orientation.
	reverse := isClockwise(r)
	l := loopFromRing(r, reverse)

	// Since our clockwise check was approximate, we check the cap and reverse if needed.
	if l.CapBound().Radius().Degrees() > 90 {
		l = loopFromRing(r, !reverse)
	}
	return l, nil
}

// Checks if a ring is clockwise or counter-clockwise. Note: This uses the algorithm for planar
// polygons and doesn't work for spherical polygons that contain the poles or the antimeridan
// discontinuity. We use this as a fast approximation instead.
func isClockwise(r *geom.LinearRing) bool {
	// The algorithm is described here https://en.wikipedia.org/wiki/Shoelace_formula
	var a float64
	n := r.NumCoords()
	for i := 0; i < n; i++ {
		p1 := r.Coord(i)
		p2 := r.Coord((i + 1) % n)
		a += (p2.X() - p1.X()) * (p1.Y() + p2.Y())
	}
	return a > 0
}

func loopFromRing(r *geom.LinearRing, reverse bool) *s2.Loop {
	// In WKB, the last coordinate is repeated for a ring to form a closed loop. For s2 the points
	// aren't allowed to repeat and the loop is assumed to be closed, so we skip the last point.
	n := r.NumCoords()
	pts := make([]s2.Point, n-1)
	for i := 0; i < n-1; i++ {
		var c geom.Coord
		if reverse {
			c = r.Coord(n - 1 - i)
		} else {
			c = r.Coord(i)
		}
		p := pointFromCoord(c)
		pts[i] = p
	}
	return s2.LoopFromPoints(pts)
}

// create cells for point from the minLevel to maxLevel both inclusive.
func indexCellsForPoint(p *geom.Point, minLevel, maxLevel int) s2.CellUnion {
	if maxLevel < minLevel {
		log.Fatalf("Maxlevel should be greater than minLevel")
	}
	ll := s2.LatLngFromDegrees(p.Y(), p.X())
	c := s2.CellIDFromLatLng(ll)
	cells := make([]s2.CellID, maxLevel-minLevel+1)
	for l := minLevel; l <= maxLevel; l++ {
		cells[l-minLevel] = c.Parent(l)
	}
	return cells
}

// Make s2.Loop implement s2.Region
type loopRegion struct {
	*s2.Loop
}

func (l loopRegion) ContainsCell(c s2.Cell) bool {
	// Quick check if the cell is in the bounding box
	if !l.RectBound().ContainsCell(c) {
		return false
	}

	// Quick check to see if the first vertex is in the loop.
	if !l.ContainsPoint(c.Vertex(0)) {
		return false
	}

	// At this stage one vertex is in the loop, now we check that the edges of the cell do not cross
	// the loop.
	return !l.edgesCross(c)
}

// returns true if the edges of the cell cross the edges of the loop
func (l loopRegion) edgesCross(c s2.Cell) bool {
	for i := 0; i < 4; i++ {
		crosser := s2.NewChainEdgeCrosser(c.Vertex(i), c.Vertex((i+1)%4), l.Vertex(0))
		for i := 1; i <= l.NumEdges(); i++ { // add vertex 0 twice as it is a closed loop
			if crosser.EdgeOrVertexChainCrossing(l.Vertex(i)) {
				return true
			}
		}
	}
	return false
}

func (l loopRegion) IntersectsCell(c s2.Cell) bool {
	// Quick check if the cell intersects the bounding box
	if !l.RectBound().IntersectsCell(c) {
		return false
	}

	// Quick check to see if the first vertex is in the loop.
	if l.ContainsPoint(c.Vertex(0)) {
		return true
	}

	// At this stage the one vertex is outside the loop. We check if any of the edges of the cell
	// cross the loop.
	if l.edgesCross(c) {
		return true
	}

	// At this stage we know that there is one point of the cell outside the loop and the boundaries
	// do not interesect. The only way for the cell to intersect with the loop is if it contains the
	// the loop. We test this by checking if an arbitrary vertex of the loop is inside the cell.
	if c.RectBound().Contains(l.RectBound()) {
		return c.ContainsPoint(l.Vertex(0))
	}
	return false
}

func coverLoop(l *s2.Loop, minLevel int, maxLevel int, maxCells int) s2.CellUnion {
	rc := &s2.RegionCoverer{
		MinLevel: minLevel,
		MaxLevel: maxLevel,
		LevelMod: 0,
		MaxCells: maxCells,
	}
	return rc.Covering(loopRegion{l})
}
