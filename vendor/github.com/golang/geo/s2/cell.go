/*
Copyright 2014 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package s2

import (
	"math"

	"github.com/golang/geo/r1"
	"github.com/golang/geo/r2"
	"github.com/golang/geo/s1"
)

// Cell is an S2 region object that represents a cell. Unlike CellIDs,
// it supports efficient containment and intersection tests. However, it is
// also a more expensive representation.
type Cell struct {
	face        int8
	level       int8
	orientation int8
	id          CellID
	uv          r2.Rect
}

// CellFromCellID constructs a Cell corresponding to the given CellID.
func CellFromCellID(id CellID) Cell {
	c := Cell{}
	c.id = id
	f, i, j, o := c.id.faceIJOrientation()
	c.face = int8(f)
	c.level = int8(c.id.Level())
	c.orientation = int8(o)
	c.uv = ijLevelToBoundUV(i, j, int(c.level))
	return c
}

// CellFromPoint constructs a cell for the given Point.
func CellFromPoint(p Point) Cell {
	return CellFromCellID(cellIDFromPoint(p))
}

// CellFromLatLng constructs a cell for the given LatLng.
func CellFromLatLng(ll LatLng) Cell {
	return CellFromCellID(CellIDFromLatLng(ll))
}

// IsLeaf returns whether this Cell is a leaf or not.
func (c Cell) IsLeaf() bool {
	return c.level == maxLevel
}

// SizeIJ returns the CellID value for the cells level.
func (c Cell) SizeIJ() int {
	return sizeIJ(int(c.level))
}

// Vertex returns the k-th vertex of the cell (k = 0,1,2,3) in CCW order
// (lower left, lower right, upper right, upper left in the UV plane).
func (c Cell) Vertex(k int) Point {
	return Point{faceUVToXYZ(int(c.face), c.uv.Vertices()[k].X, c.uv.Vertices()[k].Y).Normalize()}
}

// Edge returns the inward-facing normal of the great circle passing through
// the CCW ordered edge from vertex k to vertex k+1 (mod 4) (for k = 0,1,2,3).
func (c Cell) Edge(k int) Point {
	switch k {
	case 0:
		return Point{vNorm(int(c.face), c.uv.Y.Lo).Normalize()} // Bottom
	case 1:
		return Point{uNorm(int(c.face), c.uv.X.Hi).Normalize()} // Right
	case 2:
		return Point{vNorm(int(c.face), c.uv.Y.Hi).Mul(-1.0).Normalize()} // Top
	default:
		return Point{uNorm(int(c.face), c.uv.X.Lo).Mul(-1.0).Normalize()} // Left
	}
}

// BoundUV returns the bounds of this cell in (u,v)-space.
func (c Cell) BoundUV() r2.Rect {
	return c.uv
}

// Center returns the direction vector corresponding to the center in
// (s,t)-space of the given cell. This is the point at which the cell is
// divided into four subcells; it is not necessarily the centroid of the
// cell in (u,v)-space or (x,y,z)-space
func (c Cell) Center() Point {
	return Point{c.id.rawPoint().Normalize()}
}

// ExactArea returns the area of this cell as accurately as possible.
func (c Cell) ExactArea() float64 {
	v0, v1, v2, v3 := c.Vertex(0), c.Vertex(1), c.Vertex(2), c.Vertex(3)
	return PointArea(v0, v1, v2) + PointArea(v0, v2, v3)
}

// ApproxArea returns the approximate area of this cell. This method is accurate
// to within 3% percent for all cell sizes and accurate to within 0.1% for cells
// at level 5 or higher (i.e. squares 350km to a side or smaller on the Earth's
// surface). It is moderately cheap to compute.
func (c Cell) ApproxArea() float64 {
	// All cells at the first two levels have the same area.
	if c.level < 2 {
		return c.AverageArea()
	}

	// First, compute the approximate area of the cell when projected
	// perpendicular to its normal. The cross product of its diagonals gives
	// the normal, and the length of the normal is twice the projected area.
	flatArea := 0.5 * (c.Vertex(2).Sub(c.Vertex(0).Vector).
		Cross(c.Vertex(3).Sub(c.Vertex(1).Vector)).Norm())

	// Now, compensate for the curvature of the cell surface by pretending
	// that the cell is shaped like a spherical cap. The ratio of the
	// area of a spherical cap to the area of its projected disc turns out
	// to be 2 / (1 + sqrt(1 - r*r)) where r is the radius of the disc.
	// For example, when r=0 the ratio is 1, and when r=1 the ratio is 2.
	// Here we set Pi*r*r == flatArea to find the equivalent disc.
	return flatArea * 2 / (1 + math.Sqrt(1-math.Min(1/math.Pi*flatArea, 1)))
}

// AverageArea returns the average area of cells at the level of this cell.
// This is accurate to within a factor of 1.7.
func (c Cell) AverageArea() float64 {
	return AvgAreaMetric.Value(int(c.level))
}

// IntersectsCell reports whether the intersection of this cell and the other cell is not nil.
func (c Cell) IntersectsCell(oc Cell) bool {
	return c.id.Intersects(oc.id)
}

// ContainsCell reports whether this cell contains the other cell.
func (c Cell) ContainsCell(oc Cell) bool {
	return c.id.Contains(oc.id)
}

// latitude returns the latitude of the cell vertex given by (i,j), where "i" and "j" are either 0 or 1.
func (c Cell) latitude(i, j int) float64 {
	var u, v float64
	switch {
	case i == 0 && j == 0:
		u = c.uv.X.Lo
		v = c.uv.Y.Lo
	case i == 0 && j == 1:
		u = c.uv.X.Lo
		v = c.uv.Y.Hi
	case i == 1 && j == 0:
		u = c.uv.X.Hi
		v = c.uv.Y.Lo
	case i == 1 && j == 1:
		u = c.uv.X.Hi
		v = c.uv.Y.Hi
	default:
		panic("i and/or j is out of bound")
	}
	return latitude(Point{faceUVToXYZ(int(c.face), u, v)}).Radians()
}

// longitude returns the longitude of the cell vertex given by (i,j), where "i" and "j" are either 0 or 1.
func (c Cell) longitude(i, j int) float64 {
	var u, v float64
	switch {
	case i == 0 && j == 0:
		u = c.uv.X.Lo
		v = c.uv.Y.Lo
	case i == 0 && j == 1:
		u = c.uv.X.Lo
		v = c.uv.Y.Hi
	case i == 1 && j == 0:
		u = c.uv.X.Hi
		v = c.uv.Y.Lo
	case i == 1 && j == 1:
		u = c.uv.X.Hi
		v = c.uv.Y.Hi
	default:
		panic("i and/or j is out of bound")
	}
	return longitude(Point{faceUVToXYZ(int(c.face), u, v)}).Radians()
}

// TODO(akashagrawal): move these package private variables to a more appropriate location.
var (
	dblEpsilon = math.Nextafter(1, 2) - 1
	poleMinLat = math.Asin(math.Sqrt(1.0/3)) - 0.5*dblEpsilon
)

// RectBound returns the bounding rectangle of this cell.
func (c Cell) RectBound() Rect {
	if c.level > 0 {
		// Except for cells at level 0, the latitude and longitude extremes are
		// attained at the vertices.  Furthermore, the latitude range is
		// determined by one pair of diagonally opposite vertices and the
		// longitude range is determined by the other pair.
		//
		// We first determine which corner (i,j) of the cell has the largest
		// absolute latitude.  To maximize latitude, we want to find the point in
		// the cell that has the largest absolute z-coordinate and the smallest
		// absolute x- and y-coordinates.  To do this we look at each coordinate
		// (u and v), and determine whether we want to minimize or maximize that
		// coordinate based on the axis direction and the cell's (u,v) quadrant.
		u := c.uv.X.Lo + c.uv.X.Hi
		v := c.uv.Y.Lo + c.uv.Y.Hi
		var i, j int
		if uAxis(int(c.face)).Z == 0 {
			if u < 0 {
				i = 1
			}
		} else if u > 0 {
			i = 1
		}
		if vAxis(int(c.face)).Z == 0 {
			if v < 0 {
				j = 1
			}
		} else if v > 0 {
			j = 1
		}
		lat := r1.IntervalFromPoint(c.latitude(i, j)).AddPoint(c.latitude(1-i, 1-j))
		lng := s1.EmptyInterval().AddPoint(c.longitude(i, 1-j)).AddPoint(c.longitude(1-i, j))

		// We grow the bounds slightly to make sure that the bounding rectangle
		// contains LatLngFromPoint(P) for any point P inside the loop L defined by the
		// four *normalized* vertices.  Note that normalization of a vector can
		// change its direction by up to 0.5 * dblEpsilon radians, and it is not
		// enough just to add Normalize calls to the code above because the
		// latitude/longitude ranges are not necessarily determined by diagonally
		// opposite vertex pairs after normalization.
		//
		// We would like to bound the amount by which the latitude/longitude of a
		// contained point P can exceed the bounds computed above.  In the case of
		// longitude, the normalization error can change the direction of rounding
		// leading to a maximum difference in longitude of 2 * dblEpsilon.  In
		// the case of latitude, the normalization error can shift the latitude by
		// up to 0.5 * dblEpsilon and the other sources of error can cause the
		// two latitudes to differ by up to another 1.5 * dblEpsilon, which also
		// leads to a maximum difference of 2 * dblEpsilon.
		return Rect{lat, lng}.expanded(LatLng{s1.Angle(2 * dblEpsilon), s1.Angle(2 * dblEpsilon)}).PolarClosure()
	}

	// The 4 cells around the equator extend to +/-45 degrees latitude at the
	// midpoints of their top and bottom edges.  The two cells covering the
	// poles extend down to +/-35.26 degrees at their vertices.  The maximum
	// error in this calculation is 0.5 * dblEpsilon.
	var bound Rect
	switch c.face {
	case 0:
		bound = Rect{r1.Interval{-math.Pi / 4, math.Pi / 4}, s1.Interval{-math.Pi / 4, math.Pi / 4}}
	case 1:
		bound = Rect{r1.Interval{-math.Pi / 4, math.Pi / 4}, s1.Interval{math.Pi / 4, 3 * math.Pi / 4}}
	case 2:
		bound = Rect{r1.Interval{poleMinLat, math.Pi / 2}, s1.FullInterval()}
	case 3:
		bound = Rect{r1.Interval{-math.Pi / 4, math.Pi / 4}, s1.Interval{3 * math.Pi / 4, -3 * math.Pi / 4}}
	case 4:
		bound = Rect{r1.Interval{-math.Pi / 4, math.Pi / 4}, s1.Interval{-3 * math.Pi / 4, -math.Pi / 4}}
	default:
		bound = Rect{r1.Interval{-math.Pi / 2, -poleMinLat}, s1.FullInterval()}
	}

	// Finally, we expand the bound to account for the error when a point P is
	// converted to an LatLng to test for containment. (The bound should be
	// large enough so that it contains the computed LatLng of any contained
	// point, not just the infinite-precision version.) We don't need to expand
	// longitude because longitude is calculated via a single call to math.Atan2,
	// which is guaranteed to be semi-monotonic.
	return bound.expanded(LatLng{s1.Angle(dblEpsilon), s1.Angle(0)})
}

// CapBound returns the bounding cap of this cell.
func (c Cell) CapBound() Cap {
	// We use the cell center in (u,v)-space as the cap axis.  This vector is very close
	// to GetCenter() and faster to compute.  Neither one of these vectors yields the
	// bounding cap with minimal surface area, but they are both pretty close.
	cap := CapFromPoint(Point{faceUVToXYZ(int(c.face), c.uv.Center().X, c.uv.Center().Y).Normalize()})
	for k := 0; k < 4; k++ {
		cap = cap.AddPoint(c.Vertex(k))
	}
	return cap
}

// ContainsPoint reports whether this cell contains the given point. Note that
// unlike Loop/Polygon, a Cell is considered to be a closed set. This means
// that a point on a Cell's edge or vertex belong to the Cell and the relevant
// adjacent Cells too.
//
// If you want every point to be contained by exactly one Cell,
// you will need to convert the Cell to a Loop.
func (c Cell) ContainsPoint(p Point) bool {
	var uv r2.Point
	var ok bool
	if uv.X, uv.Y, ok = faceXYZToUV(int(c.face), p); !ok {
		return false
	}

	// Expand the (u,v) bound to ensure that
	//
	//   CellFromPoint(p).ContainsPoint(p)
	//
	// is always true. To do this, we need to account for the error when
	// converting from (u,v) coordinates to (s,t) coordinates. In the
	// normal case the total error is at most dblEpsilon.
	return c.uv.ExpandedByMargin(dblEpsilon).ContainsPoint(uv)
}

// BUG(roberts): Differences from C++:
// Accessor methods
// Subdivide
// BoundUV
// Distance/DistanceToEdge
// VertexChordDistance
