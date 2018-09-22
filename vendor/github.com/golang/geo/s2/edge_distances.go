/*
Copyright 2017 Google Inc. All rights reserved.

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

// This file defines a collection of methods for computing the distance to an edge,
// interpolating along an edge, projecting points onto edges, etc.

import (
	"math"

	"github.com/golang/geo/s1"
)

// DistanceFromSegment returns the distance of point X from line segment AB.
// The points are expected to be normalized. The result is very accurate for small
// distances but may have some numerical error if the distance is large
// (approximately pi/2 or greater). The case A == B is handled correctly.
func DistanceFromSegment(x, a, b Point) s1.Angle {
	var minDist s1.ChordAngle
	minDist, _ = updateMinDistance(x, a, b, minDist, true)
	return minDist.Angle()
}

// IsDistanceLess reports whether the distance from X to the edge AB is less
// than limit. This method is faster than DistanceFromSegment(). If you want to
// compare against a fixed s1.Angle, you should convert it to an s1.ChordAngle
// once and save the value, since this conversion is relatively expensive.
func IsDistanceLess(x, a, b Point, limit s1.ChordAngle) bool {
	_, less := UpdateMinDistance(x, a, b, limit)
	return less
}

// UpdateMinDistance checks if the distance from X to the edge AB is less
// then minDist, and if so, returns the updated value and true.
// The case A == B is handled correctly.
//
// Use this method when you want to compute many distances and keep track of
// the minimum. It is significantly faster than using DistanceFromSegment
// because (1) using s1.ChordAngle is much faster than s1.Angle, and (2) it
// can save a lot of work by not actually computing the distance when it is
// obviously larger than the current minimum.
func UpdateMinDistance(x, a, b Point, minDist s1.ChordAngle) (s1.ChordAngle, bool) {
	return updateMinDistance(x, a, b, minDist, false)
}

// IsInteriorDistanceLess reports whether the minimum distance from X to the
// edge AB is attained at an interior point of AB (i.e., not an endpoint), and
// that distance is less than limit.
func IsInteriorDistanceLess(x, a, b Point, limit s1.ChordAngle) bool {
	_, less := UpdateMinInteriorDistance(x, a, b, limit)
	return less
}

// UpdateMinInteriorDistance reports whether the minimum distance from X to AB
// is attained at an interior point of AB (i.e., not an endpoint), and that distance
// is less than minDist. If so, the value of minDist is updated and true is returned.
// Otherwise it is unchanged and returns false.
func UpdateMinInteriorDistance(x, a, b Point, minDist s1.ChordAngle) (s1.ChordAngle, bool) {
	return interiorDist(x, a, b, minDist, false)
}

// Project returns the point along the edge AB that is closest to the point X.
// The fractional distance of this point along the edge AB can be obtained
// using DistanceFraction.
//
// This requires that all points are unit length.
func Project(x, a, b Point) Point {
	aXb := a.PointCross(b)
	// Find the closest point to X along the great circle through AB.
	p := x.Sub(aXb.Mul(x.Dot(aXb.Vector) / aXb.Vector.Norm2()))

	// If this point is on the edge AB, then it's the closest point.
	if Sign(aXb, a, Point{p}) && Sign(Point{p}, b, aXb) {
		return Point{p.Normalize()}
	}

	// Otherwise, the closest point is either A or B.
	if x.Sub(a.Vector).Norm2() <= x.Sub(b.Vector).Norm2() {
		return a
	}
	return b
}

// DistanceFraction returns the distance ratio of the point X along an edge AB.
// If X is on the line segment AB, this is the fraction T such
// that X == Interpolate(T, A, B).
//
// This requires that A and B are distinct.
func DistanceFraction(x, a, b Point) float64 {
	d0 := x.Angle(a.Vector)
	d1 := x.Angle(b.Vector)
	return float64(d0 / (d0 + d1))
}

// Interpolate returns the point X along the line segment AB whose distance from A
// is the given fraction "t" of the distance AB. Does NOT require that "t" be
// between 0 and 1. Note that all distances are measured on the surface of
// the sphere, so this is more complicated than just computing (1-t)*a + t*b
// and normalizing the result.
func Interpolate(t float64, a, b Point) Point {
	if t == 0 {
		return a
	}
	if t == 1 {
		return b
	}
	ab := a.Angle(b.Vector)
	return InterpolateAtDistance(s1.Angle(t)*ab, a, b)
}

// InterpolateAtDistance returns the point X along the line segment AB whose
// distance from A is the angle ax.
func InterpolateAtDistance(ax s1.Angle, a, b Point) Point {
	aRad := ax.Radians()

	// Use PointCross to compute the tangent vector at A towards B. The
	// result is always perpendicular to A, even if A=B or A=-B, but it is not
	// necessarily unit length. (We effectively normalize it below.)
	normal := a.PointCross(b)
	tangent := normal.Vector.Cross(a.Vector)

	// Now compute the appropriate linear combination of A and "tangent". With
	// infinite precision the result would always be unit length, but we
	// normalize it anyway to ensure that the error is within acceptable bounds.
	// (Otherwise errors can build up when the result of one interpolation is
	// fed into another interpolation.)
	return Point{(a.Mul(math.Cos(aRad)).Add(tangent.Mul(math.Sin(aRad) / tangent.Norm()))).Normalize()}
}

// minUpdateDistanceMaxError returns the maximum error in the result of
// UpdateMinDistance (and the associated functions such as
// UpdateMinInteriorDistance, IsDistanceLess, etc), assuming that all
// input points are normalized to within the bounds guaranteed by r3.Vector's
// Normalize. The error can be added or subtracted from an s1.ChordAngle
// using its Expanded method.
func minUpdateDistanceMaxError(dist s1.ChordAngle) float64 {
	// There are two cases for the maximum error in UpdateMinDistance(),
	// depending on whether the closest point is interior to the edge.
	return math.Max(minUpdateInteriorDistanceMaxError(dist), dist.MaxPointError())
}

// minUpdateInteriorDistanceMaxError returns the maximum error in the result of
// UpdateMinInteriorDistance, assuming that all input points are normalized
// to within the bounds guaranteed by Point's Normalize. The error can be added
// or subtracted from an s1.ChordAngle using its Expanded method.
func minUpdateInteriorDistanceMaxError(dist s1.ChordAngle) float64 {
	// This bound includes all source of error, assuming that the input points
	// are normalized. a and b are components of chord length that are
	// perpendicular and parallel to a plane containing the edge respectively.
	b := 0.5 * float64(dist) * float64(dist)
	a := float64(dist) * math.Sqrt(1-0.5*b)
	return ((2.5+2*math.Sqrt(3)+8.5*a)*a +
		(2+2*math.Sqrt(3)/3+6.5*(1-b))*b +
		(23+16/math.Sqrt(3))*dblEpsilon) * dblEpsilon
}

// updateMinDistance computes the distance from a point X to a line segment AB,
// and if either the distance was less than the given minDist, or alwaysUpdate is
// true, the value and whether it was updated are returned.
func updateMinDistance(x, a, b Point, minDist s1.ChordAngle, alwaysUpdate bool) (s1.ChordAngle, bool) {
	if d, ok := interiorDist(x, a, b, minDist, alwaysUpdate); ok {
		// Minimum distance is attained along the edge interior.
		return d, true
	}

	// Otherwise the minimum distance is to one of the endpoints.
	xa2, xb2 := (x.Sub(a.Vector)).Norm2(), x.Sub(b.Vector).Norm2()
	dist := s1.ChordAngle(math.Min(xa2, xb2))
	if !alwaysUpdate && dist >= minDist {
		return minDist, false
	}
	return dist, true
}

// interiorDist returns the shortest distance from point x to edge ab, assuming
// that the closest point to X is interior to AB. If the closest point is not
// interior to AB, interiorDist returns (minDist, false). If alwaysUpdate is set to
// false, the distance is only updated when the value exceeds certain the given minDist.
func interiorDist(x, a, b Point, minDist s1.ChordAngle, alwaysUpdate bool) (s1.ChordAngle, bool) {
	// Chord distance of x to both end points a and b.
	xa2, xb2 := (x.Sub(a.Vector)).Norm2(), x.Sub(b.Vector).Norm2()

	// The closest point on AB could either be one of the two vertices (the
	// vertex case) or in the interior (the interior case). Let C = A x B.
	// If X is in the spherical wedge extending from A to B around the axis
	// through C, then we are in the interior case. Otherwise we are in the
	// vertex case.
	//
	// Check whether we might be in the interior case. For this to be true, XAB
	// and XBA must both be acute angles. Checking this condition exactly is
	// expensive, so instead we consider the planar triangle ABX (which passes
	// through the sphere's interior). The planar angles XAB and XBA are always
	// less than the corresponding spherical angles, so if we are in the
	// interior case then both of these angles must be acute.
	//
	// We check this by computing the squared edge lengths of the planar
	// triangle ABX, and testing acuteness using the law of cosines:
	//
	//   max(XA^2, XB^2) < min(XA^2, XB^2) + AB^2
	if math.Max(xa2, xb2) >= math.Min(xa2, xb2)+(a.Sub(b.Vector)).Norm2() {
		return minDist, false
	}

	// The minimum distance might be to a point on the edge interior. Let R
	// be closest point to X that lies on the great circle through AB. Rather
	// than computing the geodesic distance along the surface of the sphere,
	// instead we compute the "chord length" through the sphere's interior.
	//
	// The squared chord length XR^2 can be expressed as XQ^2 + QR^2, where Q
	// is the point X projected onto the plane through the great circle AB.
	// The distance XQ^2 can be written as (X.C)^2 / |C|^2 where C = A x B.
	// We ignore the QR^2 term and instead use XQ^2 as a lower bound, since it
	// is faster and the corresponding distance on the Earth's surface is
	// accurate to within 1% for distances up to about 1800km.
	c := a.PointCross(b)
	c2 := c.Norm2()
	xDotC := x.Dot(c.Vector)
	xDotC2 := xDotC * xDotC
	if !alwaysUpdate && xDotC2 >= c2*float64(minDist) {
		// The closest point on the great circle AB is too far away.
		return minDist, false
	}

	// Otherwise we do the exact, more expensive test for the interior case.
	// This test is very likely to succeed because of the conservative planar
	// test we did initially.
	cx := c.Cross(x.Vector)
	if a.Dot(cx) >= 0 || b.Dot(cx) <= 0 {
		return minDist, false
	}

	// Compute the squared chord length XR^2 = XQ^2 + QR^2 (see above).
	// This calculation has good accuracy for all chord lengths since it
	// is based on both the dot product and cross product (rather than
	// deriving one from the other). However, note that the chord length
	// representation itself loses accuracy as the angle approaches π.
	qr := 1 - math.Sqrt(cx.Norm2()/c2)
	dist := s1.ChordAngle((xDotC2 / c2) + (qr * qr))

	if !alwaysUpdate && dist >= minDist {
		return minDist, false
	}

	return dist, true
}

// TODO(roberts): UpdateEdgePairMinDistance
// TODO(roberts): GetEdgePairClosestPoints
// TODO(roberts): IsEdgeBNearEdgeA
