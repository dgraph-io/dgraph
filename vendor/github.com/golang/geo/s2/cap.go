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
	"fmt"
	"math"

	"github.com/golang/geo/r1"
	"github.com/golang/geo/s1"
)

const (
	emptyHeight = -1.0
	zeroHeight  = 0.0
	fullHeight  = 2.0

	roundUp = 1.0 + 1.0/(1<<52)
)

var (
	// centerPoint is the default center for S2Caps
	centerPoint = Point{PointFromCoords(1.0, 0, 0).Normalize()}
)

// Cap represents a disc-shaped region defined by a center and radius.
// Technically this shape is called a "spherical cap" (rather than disc)
// because it is not planar; the cap represents a portion of the sphere that
// has been cut off by a plane. The boundary of the cap is the circle defined
// by the intersection of the sphere and the plane. For containment purposes,
// the cap is a closed set, i.e. it contains its boundary.
//
// For the most part, you can use a spherical cap wherever you would use a
// disc in planar geometry. The radius of the cap is measured along the
// surface of the sphere (rather than the straight-line distance through the
// interior). Thus a cap of radius π/2 is a hemisphere, and a cap of radius
// π covers the entire sphere.
//
// The center is a point on the surface of the unit sphere. (Hence the need for
// it to be of unit length.)
//
// Internally, the cap is represented by its center and "height". The height
// is the distance from the center point to the cutoff plane. This
// representation is much more efficient for containment tests than the
// (center, radius) representation. There is also support for "empty" and
// "full" caps, which contain no points and all points respectively.
//
// The zero value of Cap is an invalid cap. Use EmptyCap to get a valid empty cap.
type Cap struct {
	center Point
	height float64
}

// CapFromPoint constructs a cap containing a single point.
func CapFromPoint(p Point) Cap {
	return CapFromCenterHeight(p, zeroHeight)
}

// CapFromCenterAngle constructs a cap with the given center and angle.
func CapFromCenterAngle(center Point, angle s1.Angle) Cap {
	return CapFromCenterHeight(center, radiusToHeight(angle))
}

// CapFromCenterHeight constructs a cap with the given center and height. A
// negative height yields an empty cap; a height of 2 or more yields a full cap.
func CapFromCenterHeight(center Point, height float64) Cap {
	return Cap{
		center: Point{center.Normalize()},
		height: height,
	}
}

// CapFromCenterArea constructs a cap with the given center and surface area.
// Note that the area can also be interpreted as the solid angle subtended by the
// cap (because the sphere has unit radius). A negative area yields an empty cap;
// an area of 4*π or more yields a full cap.
func CapFromCenterArea(center Point, area float64) Cap {
	return CapFromCenterHeight(center, area/(math.Pi*2.0))
}

// EmptyCap returns a cap that contains no points.
func EmptyCap() Cap {
	return CapFromCenterHeight(centerPoint, emptyHeight)
}

// FullCap returns a cap that contains all points.
func FullCap() Cap {
	return CapFromCenterHeight(centerPoint, fullHeight)
}

// IsValid reports whether the Cap is considered valid.
// Heights are normalized so that they do not exceed 2.
func (c Cap) IsValid() bool {
	return c.center.Vector.IsUnit() && c.height <= fullHeight
}

// IsEmpty reports whether the cap is empty, i.e. it contains no points.
func (c Cap) IsEmpty() bool {
	return c.height < zeroHeight
}

// IsFull reports whether the cap is full, i.e. it contains all points.
func (c Cap) IsFull() bool {
	return c.height == fullHeight
}

// Center returns the cap's center point.
func (c Cap) Center() Point {
	return c.center
}

// Height returns the cap's "height".
func (c Cap) Height() float64 {
	return c.height
}

// Radius returns the cap's radius.
func (c Cap) Radius() s1.Angle {
	if c.IsEmpty() {
		return s1.Angle(emptyHeight)
	}

	// This could also be computed as acos(1 - height_), but the following
	// formula is much more accurate when the cap height is small. It
	// follows from the relationship h = 1 - cos(r) = 2 sin^2(r/2).
	return s1.Angle(2 * math.Asin(math.Sqrt(0.5*c.height)))
}

// Area returns the surface area of the Cap on the unit sphere.
func (c Cap) Area() float64 {
	return 2.0 * math.Pi * math.Max(zeroHeight, c.height)
}

// Contains reports whether this cap contains the other.
func (c Cap) Contains(other Cap) bool {
	// In a set containment sense, every cap contains the empty cap.
	if c.IsFull() || other.IsEmpty() {
		return true
	}
	return c.Radius() >= c.center.Distance(other.center)+other.Radius()
}

// Intersects reports whether this cap intersects the other cap.
// i.e. whether they have any points in common.
func (c Cap) Intersects(other Cap) bool {
	if c.IsEmpty() || other.IsEmpty() {
		return false
	}

	return c.Radius()+other.Radius() >= c.center.Distance(other.center)
}

// InteriorIntersects reports whether this caps interior intersects the other cap.
func (c Cap) InteriorIntersects(other Cap) bool {
	// Make sure this cap has an interior and the other cap is non-empty.
	if c.height <= zeroHeight || other.IsEmpty() {
		return false
	}

	return c.Radius()+other.Radius() > c.center.Distance(other.center)
}

// ContainsPoint reports whether this cap contains the point.
func (c Cap) ContainsPoint(p Point) bool {
	return c.center.Sub(p.Vector).Norm2() <= 2*c.height
}

// InteriorContainsPoint reports whether the point is within the interior of this cap.
func (c Cap) InteriorContainsPoint(p Point) bool {
	return c.IsFull() || c.center.Sub(p.Vector).Norm2() < 2*c.height
}

// Complement returns the complement of the interior of the cap. A cap and its
// complement have the same boundary but do not share any interior points.
// The complement operator is not a bijection because the complement of a
// singleton cap (containing a single point) is the same as the complement
// of an empty cap.
func (c Cap) Complement() Cap {
	height := emptyHeight
	if !c.IsFull() {
		height = fullHeight - math.Max(c.height, zeroHeight)
	}
	return CapFromCenterHeight(Point{c.center.Mul(-1.0)}, height)
}

// CapBound returns a bounding spherical cap. This is not guaranteed to be exact.
func (c Cap) CapBound() Cap {
	return c
}

// RectBound returns a bounding latitude-longitude rectangle.
// The bounds are not guaranteed to be tight.
func (c Cap) RectBound() Rect {
	if c.IsEmpty() {
		return EmptyRect()
	}

	capAngle := c.Radius().Radians()
	allLongitudes := false
	lat := r1.Interval{
		Lo: latitude(c.center).Radians() - capAngle,
		Hi: latitude(c.center).Radians() + capAngle,
	}
	lng := s1.FullInterval()

	// Check whether cap includes the south pole.
	if lat.Lo <= -math.Pi/2 {
		lat.Lo = -math.Pi / 2
		allLongitudes = true
	}

	// Check whether cap includes the north pole.
	if lat.Hi >= math.Pi/2 {
		lat.Hi = math.Pi / 2
		allLongitudes = true
	}

	if !allLongitudes {
		// Compute the range of longitudes covered by the cap. We use the law
		// of sines for spherical triangles. Consider the triangle ABC where
		// A is the north pole, B is the center of the cap, and C is the point
		// of tangency between the cap boundary and a line of longitude. Then
		// C is a right angle, and letting a,b,c denote the sides opposite A,B,C,
		// we have sin(a)/sin(A) = sin(c)/sin(C), or sin(A) = sin(a)/sin(c).
		// Here "a" is the cap angle, and "c" is the colatitude (90 degrees
		// minus the latitude). This formula also works for negative latitudes.
		//
		// The formula for sin(a) follows from the relationship h = 1 - cos(a).
		sinA := math.Sqrt(c.height * (2 - c.height))
		sinC := math.Cos(latitude(c.center).Radians())
		if sinA <= sinC {
			angleA := math.Asin(sinA / sinC)
			lng.Lo = math.Remainder(longitude(c.center).Radians()-angleA, math.Pi*2)
			lng.Hi = math.Remainder(longitude(c.center).Radians()+angleA, math.Pi*2)
		}
	}
	return Rect{lat, lng}
}

// ApproxEqual reports if this caps' center and height are within
// a reasonable epsilon from the other cap.
func (c Cap) ApproxEqual(other Cap) bool {
	const epsilon = 1e-14
	return c.center.ApproxEqual(other.center) &&
		math.Abs(c.height-other.height) <= epsilon ||
		c.IsEmpty() && other.height <= epsilon ||
		other.IsEmpty() && c.height <= epsilon ||
		c.IsFull() && other.height >= 2-epsilon ||
		other.IsFull() && c.height >= 2-epsilon
}

// AddPoint increases the cap if necessary to include the given point. If this cap is empty,
// then the center is set to the point with a zero height. p must be unit-length.
func (c Cap) AddPoint(p Point) Cap {
	if c.IsEmpty() {
		return Cap{center: p}
	}

	// To make sure that the resulting cap actually includes this point,
	// we need to round up the distance calculation.  That is, after
	// calling cap.AddPoint(p), cap.Contains(p) should be true.
	dist2 := c.center.Sub(p.Vector).Norm2()
	c.height = math.Max(c.height, roundUp*0.5*dist2)
	return c
}

// AddCap increases the cap height if necessary to include the other cap. If this cap is empty,
// it is set to the other cap.
func (c Cap) AddCap(other Cap) Cap {
	if c.IsEmpty() {
		return other
	}
	if other.IsEmpty() {
		return c
	}

	radius := c.center.Angle(other.center.Vector) + other.Radius()
	c.height = math.Max(c.height, roundUp*radiusToHeight(radius))
	return c
}

// Expanded returns a new cap expanded by the given angle. If the cap is empty,
// it returns an empty cap.
func (c Cap) Expanded(distance s1.Angle) Cap {
	if c.IsEmpty() {
		return EmptyCap()
	}
	return CapFromCenterAngle(c.center, c.Radius()+distance)
}

func (c Cap) String() string {
	return fmt.Sprintf("[Center=%v, Radius=%f]", c.center.Vector, c.Radius().Degrees())
}

// radiusToHeight converts an s1.Angle into the height of the cap.
func radiusToHeight(r s1.Angle) float64 {
	if r.Radians() < 0 {
		return emptyHeight
	}
	if r.Radians() >= math.Pi {
		return fullHeight
	}
	// The height of the cap can be computed as 1 - cos(r), but this isn't very
	// accurate for angles close to zero (where cos(r) is almost 1). The
	// formula below has much better precision.
	d := math.Sin(0.5 * r.Radians())
	return 2 * d * d

}

// ContainsCell reports whether the cap contains the given cell.
func (c Cap) ContainsCell(cell Cell) bool {
	// If the cap does not contain all cell vertices, return false.
	var vertices [4]Point
	for k := 0; k < 4; k++ {
		vertices[k] = cell.Vertex(k)
		if !c.ContainsPoint(vertices[k]) {
			return false
		}
	}
	// Otherwise, return true if the complement of the cap does not intersect the cell.
	return !c.Complement().intersects(cell, vertices)
}

// IntersectsCell reports whether the cap intersects the cell.
func (c Cap) IntersectsCell(cell Cell) bool {
	// If the cap contains any cell vertex, return true.
	var vertices [4]Point
	for k := 0; k < 4; k++ {
		vertices[k] = cell.Vertex(k)
		if c.ContainsPoint(vertices[k]) {
			return true
		}
	}
	return c.intersects(cell, vertices)
}

// intersects reports whether the cap intersects any point of the cell excluding
// its vertices (which are assumed to already have been checked).
func (c Cap) intersects(cell Cell, vertices [4]Point) bool {
	// If the cap is a hemisphere or larger, the cell and the complement of the cap
	// are both convex. Therefore since no vertex of the cell is contained, no other
	// interior point of the cell is contained either.
	if c.height >= 1 {
		return false
	}

	// We need to check for empty caps due to the center check just below.
	if c.IsEmpty() {
		return false
	}

	// Optimization: return true if the cell contains the cap center. This allows half
	// of the edge checks below to be skipped.
	if cell.ContainsPoint(c.center) {
		return true
	}

	// At this point we know that the cell does not contain the cap center, and the cap
	// does not contain any cell vertex. The only way that they can intersect is if the
	// cap intersects the interior of some edge.
	sin2Angle := c.height * (2 - c.height)
	for k := 0; k < 4; k++ {
		edge := cell.Edge(k).Vector
		dot := c.center.Vector.Dot(edge)
		if dot > 0 {
			// The center is in the interior half-space defined by the edge. We do not need
			// to consider these edges, since if the cap intersects this edge then it also
			// intersects the edge on the opposite side of the cell, because the center is
			// not contained with the cell.
			continue
		}

		// The Norm2() factor is necessary because "edge" is not normalized.
		if dot*dot > sin2Angle*edge.Norm2() {
			return false
		}

		// Otherwise, the great circle containing this edge intersects the interior of the cap. We just
		// need to check whether the point of closest approach occurs between the two edge endpoints.
		dir := edge.Cross(c.center.Vector)
		if dir.Dot(vertices[k].Vector) < 0 && dir.Dot(vertices[(k+1)&3].Vector) > 0 {
			return true
		}
	}
	return false
}

// TODO(roberts): Differences from C++
// Centroid, Union
