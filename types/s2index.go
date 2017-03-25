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
	"log"

	"github.com/golang/geo/s2"
	"github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/x"
)

func parentCoverTokens(parents s2.CellUnion, cover s2.CellUnion) []string {
	// We index parents and cover using different prefix, that makes it more
	// performant at query time to only look up parents/cover depending on what
	// kind of query it is.
	tokens := make([]string, 0, len(parents)+len(cover))
	tokens = append(tokens, createTokens(parents, parentPrefix)...)
	tokens = append(tokens, createTokens(cover, coverPrefix)...)
	x.AssertTruef(len(tokens) == len(parents)+len(cover), "%d %d %d",
		len(tokens), len(parents), len(cover))
	return tokens
}

// IndexTokens returns the tokens to be used in a geospatial index for the given geometry. If the
// geometry is not supported it returns an error.
func IndexGeoTokens(g geom.T) ([]string, error) {
	parents, cover, err := indexCells(g)
	if err != nil {
		return nil, err
	}
	return parentCoverTokens(parents, cover), nil
}

// IndexKeysForCap returns the keys to be used in a geospatial index for a Cap.
func indexCellsForCap(c s2.Cap) s2.CellUnion {
	rc := &s2.RegionCoverer{
		MinLevel: MinCellLevel,
		MaxLevel: MaxCellLevel,
		LevelMod: 0,
		MaxCells: MaxCells,
	}
	return rc.Covering(c)
}

const (
	parentPrefix = "p/"
	coverPrefix  = "c/"
)

// IndexCells returns two cellunions. The first is a list of parents, which are all the cells upto
// the min level that contain this geometry. The second is the cover, which are the smallest
// possible cells required to cover the region. This makes it easier at query time to query only the
// parents or only the cover or both depending on whether it is a within, contains or intersects
// query.
func indexCells(g geom.T) (parents, cover s2.CellUnion, err error) {
	if g.Stride() != 2 {
		return nil, nil, x.Errorf("Covering only available for 2D co-ordinates.")
	}
	switch v := g.(type) {
	case *geom.Point:
		p, c := indexCellsForPoint(v, MinCellLevel, MaxCellLevel)
		return p, c, nil
	case *geom.Polygon:
		l, err := loopFromPolygon(v)
		if err != nil {
			return nil, nil, err
		}
		cover := coverLoop(l, MinCellLevel, MaxCellLevel, MaxCells)
		parents := getParentCells(cover, MinCellLevel)
		return parents, cover, nil
	default:
		return nil, nil, x.Errorf("Cannot index geometry of type %T", v)
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

// PointFromPoint converts a geom.Point to a s2.Point
func pointFromPoint(p *geom.Point) s2.Point {
	return pointFromCoord(p.Coords())
}

// loopFromPolygon converts a geom.Polygon to a s2.Loop. We use loops instead of s2.Polygon as the
// s2.Polygon implemention is incomplete.
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
func indexCellsForPoint(p *geom.Point, minLevel, maxLevel int) (s2.CellUnion, s2.CellUnion) {
	if maxLevel < minLevel {
		log.Fatalf("Maxlevel should be greater than minLevel")
	}
	ll := s2.LatLngFromDegrees(p.Y(), p.X())
	c := s2.CellIDFromLatLng(ll)
	cells := make([]s2.CellID, maxLevel-minLevel+1)
	for l := minLevel; l <= maxLevel; l++ {
		cells[l-minLevel] = c.Parent(l)
	}
	return cells, []s2.CellID{c.Parent(maxLevel)}
}

func getParentCells(cu s2.CellUnion, minLevel int) s2.CellUnion {
	parents := make(map[s2.CellID]bool)
	for _, c := range cu {
		for l := c.Level(); l >= minLevel; l-- {
			parents[c.Parent(l)] = true
		}
	}
	// convert the parents map to an array
	cells := make([]s2.CellID, len(parents))
	i := 0
	for k := range parents {
		cells[i] = k
		i++
	}
	return cells
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

// appendTokens creates tokens with a certain prefix and append.
func createTokens(cu s2.CellUnion, prefix string) (toks []string) {
	for _, c := range cu {
		toks = append(toks, prefix+c.ToToken())
	}
	return toks
}
