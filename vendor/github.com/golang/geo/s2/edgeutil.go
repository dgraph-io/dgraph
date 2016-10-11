/*
Copyright 2015 Google Inc. All rights reserved.

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
	"github.com/golang/geo/r3"
	"github.com/golang/geo/s1"
)

var (
	// edgeClipErrorUVCoord is the maximum error in a u- or v-coordinate
	// compared to the exact result, assuming that the points A and B are in
	// the rectangle [-1,1]x[1,1] or slightly outside it (by 1e-10 or less).
	edgeClipErrorUVCoord = 2.25 * dblEpsilon

	// edgeClipErrorUVDist is the maximum distance from a clipped point to
	// the corresponding exact result. It is equal to the error in a single
	// coordinate because at most one coordinate is subject to error.
	edgeClipErrorUVDist = 2.25 * dblEpsilon

	// faceClipErrorRadians is the maximum angle between a returned vertex
	// and the nearest point on the exact edge AB. It is equal to the
	// maximum directional error in PointCross, plus the error when
	// projecting points onto a cube face.
	faceClipErrorRadians = 3 * dblEpsilon

	// faceClipErrorDist is the same angle expressed as a maximum distance
	// in (u,v)-space. In other words, a returned vertex is at most this far
	// from the exact edge AB projected into (u,v)-space.
	faceClipErrorUVDist = 9 * dblEpsilon

	// faceClipErrorUVCoord is the maximum angle between a returned vertex
	// and the nearest point on the exact edge AB expressed as the maximum error
	// in an individual u- or v-coordinate. In other words, for each
	// returned vertex there is a point on the exact edge AB whose u- and
	// v-coordinates differ from the vertex by at most this amount.
	faceClipErrorUVCoord = 9.0 * (1.0 / math.Sqrt2) * dblEpsilon

	// intersectsRectErrorUVDist is the maximum error when computing if a point
	// intersects with a given Rect. If some point of AB is inside the
	// rectangle by at least this distance, the result is guaranteed to be true;
	// if all points of AB are outside the rectangle by at least this distance,
	// the result is guaranteed to be false. This bound assumes that rect is
	// a subset of the rectangle [-1,1]x[-1,1] or extends slightly outside it
	// (e.g., by 1e-10 or less).
	intersectsRectErrorUVDist = 3 * math.Sqrt2 * dblEpsilon

	// intersectionError can be set somewhat arbitrarily, because the algorithm
	// uses more precision if necessary in order to achieve the specified error.
	// The only strict requirement is that intersectionError >= dblEpsilon
	// radians. However, using a larger error tolerance makes the algorithm more
	// efficient because it reduces the number of cases where exact arithmetic is
	// needed.
	intersectionError = s1.Angle(4 * dblEpsilon)

	// intersectionMergeRadius is used to ensure that intersection points that
	// are supposed to be coincident are merged back together into a single
	// vertex. This is required in order for various polygon operations (union,
	// intersection, etc) to work correctly. It is twice the intersection error
	// because two coincident intersection points might have errors in
	// opposite directions.
	intersectionMergeRadius = 2 * intersectionError
)

// SimpleCrossing reports whether edge AB crosses CD at a point that is interior
// to both edges. Properties:
//
//  (1) SimpleCrossing(b,a,c,d) == SimpleCrossing(a,b,c,d)
//  (2) SimpleCrossing(c,d,a,b) == SimpleCrossing(a,b,c,d)
func SimpleCrossing(a, b, c, d Point) bool {
	// We compute the equivalent of Sign for triangles ACB, CBD, BDA,
	// and DAC. All of these triangles need to have the same orientation
	// (CW or CCW) for an intersection to exist.

	ab := a.Vector.Cross(b.Vector)
	acb := -(ab.Dot(c.Vector))
	bda := ab.Dot(d.Vector)
	if acb*bda <= 0 {
		return false
	}

	cd := c.Vector.Cross(d.Vector)
	cbd := -(cd.Dot(b.Vector))
	dac := cd.Dot(a.Vector)
	return (acb*cbd > 0) && (acb*dac > 0)
}

// VertexCrossing reports whether two edges "cross" in such a way that point-in-polygon
// containment tests can be implemented by counting the number of edge crossings.
//
// Given two edges AB and CD where at least two vertices are identical
// (i.e. CrossingSign(a,b,c,d) == 0), the basic rule is that a "crossing"
// occurs if AB is encountered after CD during a CCW sweep around the shared
// vertex starting from a fixed reference point.
//
// Note that according to this rule, if AB crosses CD then in general CD
// does not cross AB.  However, this leads to the correct result when
// counting polygon edge crossings.  For example, suppose that A,B,C are
// three consecutive vertices of a CCW polygon.  If we now consider the edge
// crossings of a segment BP as P sweeps around B, the crossing number
// changes parity exactly when BP crosses BA or BC.
//
// Useful properties of VertexCrossing (VC):
//
//  (1) VC(a,a,c,d) == VC(a,b,c,c) == false
//  (2) VC(a,b,a,b) == VC(a,b,b,a) == true
//  (3) VC(a,b,c,d) == VC(a,b,d,c) == VC(b,a,c,d) == VC(b,a,d,c)
//  (3) If exactly one of a,b equals one of c,d, then exactly one of
//      VC(a,b,c,d) and VC(c,d,a,b) is true
//
// It is an error to call this method with 4 distinct vertices.
func VertexCrossing(a, b, c, d Point) bool {
	// If A == B or C == D there is no intersection. We need to check this
	// case first in case 3 or more input points are identical.
	if a.ApproxEqual(b) || c.ApproxEqual(d) {
		return false
	}

	// If any other pair of vertices is equal, there is a crossing if and only
	// if OrderedCCW indicates that the edge AB is further CCW around the
	// shared vertex O (either A or B) than the edge CD, starting from an
	// arbitrary fixed reference point.
	switch {
	case a.ApproxEqual(d):
		return OrderedCCW(Point{a.Ortho()}, c, b, a)
	case b.ApproxEqual(c):
		return OrderedCCW(Point{b.Ortho()}, d, a, b)
	case a.ApproxEqual(c):
		return OrderedCCW(Point{a.Ortho()}, d, b, a)
	case b.ApproxEqual(d):
		return OrderedCCW(Point{b.Ortho()}, c, a, b)
	}

	return false
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

// RectBounder is used to compute a bounding rectangle that contains all edges
// defined by a vertex chain (v0, v1, v2, ...). All vertices must be unit length.
// Note that the bounding rectangle of an edge can be larger than the bounding
// rectangle of its endpoints, e.g. consider an edge that passes through the North Pole.
//
// The bounds are calculated conservatively to account for numerical errors
// when points are converted to LatLngs. More precisely, this function
// guarantees the following:
// Let L be a closed edge chain (Loop) such that the interior of the loop does
// not contain either pole. Now if P is any point such that L.ContainsPoint(P),
// then RectBound(L).ContainsPoint(LatLngFromPoint(P)).
type RectBounder struct {
	// The previous vertex in the chain.
	a Point
	// The previous vertex latitude longitude.
	aLL   LatLng
	bound Rect
}

// NewRectBounder returns a new instance of a RectBounder.
func NewRectBounder() *RectBounder {
	return &RectBounder{
		bound: EmptyRect(),
	}
}

// AddPoint adds the given point to the chain. The Point must be unit length.
func (r *RectBounder) AddPoint(b Point) {
	bLL := LatLngFromPoint(b)

	if r.bound.IsEmpty() {
		r.a = b
		r.aLL = bLL
		r.bound = r.bound.AddPoint(bLL)
		return
	}

	// First compute the cross product N = A x B robustly. This is the normal
	// to the great circle through A and B. We don't use RobustSign
	// since that method returns an arbitrary vector orthogonal to A if the two
	// vectors are proportional, and we want the zero vector in that case.
	n := r.a.Sub(b.Vector).Cross(r.a.Add(b.Vector)) // N = 2 * (A x B)

	// The relative error in N gets large as its norm gets very small (i.e.,
	// when the two points are nearly identical or antipodal). We handle this
	// by choosing a maximum allowable error, and if the error is greater than
	// this we fall back to a different technique. Since it turns out that
	// the other sources of error in converting the normal to a maximum
	// latitude add up to at most 1.16 * dblEpsilon, and it is desirable to
	// have the total error be a multiple of dblEpsilon, we have chosen to
	// limit the maximum error in the normal to be 3.84 * dblEpsilon.
	// It is possible to show that the error is less than this when
	//
	// n.Norm() >= 8 * sqrt(3) / (3.84 - 0.5 - sqrt(3)) * dblEpsilon
	//          = 1.91346e-15 (about 8.618 * dblEpsilon)
	nNorm := n.Norm()
	if nNorm < 1.91346e-15 {
		// A and B are either nearly identical or nearly antipodal (to within
		// 4.309 * dblEpsilon, or about 6 nanometers on the earth's surface).
		if r.a.Dot(b.Vector) < 0 {
			// The two points are nearly antipodal. The easiest solution is to
			// assume that the edge between A and B could go in any direction
			// around the sphere.
			r.bound = FullRect()
		} else {
			// The two points are nearly identical (to within 4.309 * dblEpsilon).
			// In this case we can just use the bounding rectangle of the points,
			// since after the expansion done by GetBound this Rect is
			// guaranteed to include the (lat,lng) values of all points along AB.
			r.bound = r.bound.Union(RectFromLatLng(r.aLL).AddPoint(bLL))
		}
		r.a = b
		r.aLL = bLL
		return
	}

	// Compute the longitude range spanned by AB.
	lngAB := s1.EmptyInterval().AddPoint(r.aLL.Lng.Radians()).AddPoint(bLL.Lng.Radians())
	if lngAB.Length() >= math.Pi-2*dblEpsilon {
		// The points lie on nearly opposite lines of longitude to within the
		// maximum error of the calculation. The easiest solution is to assume
		// that AB could go on either side of the pole.
		lngAB = s1.FullInterval()
	}

	// Next we compute the latitude range spanned by the edge AB. We start
	// with the range spanning the two endpoints of the edge:
	latAB := r1.IntervalFromPoint(r.aLL.Lat.Radians()).AddPoint(bLL.Lat.Radians())

	// This is the desired range unless the edge AB crosses the plane
	// through N and the Z-axis (which is where the great circle through A
	// and B attains its minimum and maximum latitudes). To test whether AB
	// crosses this plane, we compute a vector M perpendicular to this
	// plane and then project A and B onto it.
	m := n.Cross(PointFromCoords(0, 0, 1).Vector)
	mA := m.Dot(r.a.Vector)
	mB := m.Dot(b.Vector)

	// We want to test the signs of "mA" and "mB", so we need to bound
	// the error in these calculations. It is possible to show that the
	// total error is bounded by
	//
	// (1 + sqrt(3)) * dblEpsilon * nNorm + 8 * sqrt(3) * (dblEpsilon**2)
	//   = 6.06638e-16 * nNorm + 6.83174e-31

	mError := 6.06638e-16*nNorm + 6.83174e-31
	if mA*mB < 0 || math.Abs(mA) <= mError || math.Abs(mB) <= mError {
		// Minimum/maximum latitude *may* occur in the edge interior.
		//
		// The maximum latitude is 90 degrees minus the latitude of N. We
		// compute this directly using atan2 in order to get maximum accuracy
		// near the poles.
		//
		// Our goal is compute a bound that contains the computed latitudes of
		// all S2Points P that pass the point-in-polygon containment test.
		// There are three sources of error we need to consider:
		// - the directional error in N (at most 3.84 * dblEpsilon)
		// - converting N to a maximum latitude
		// - computing the latitude of the test point P
		// The latter two sources of error are at most 0.955 * dblEpsilon
		// individually, but it is possible to show by a more complex analysis
		// that together they can add up to at most 1.16 * dblEpsilon, for a
		// total error of 5 * dblEpsilon.
		//
		// We add 3 * dblEpsilon to the bound here, and GetBound() will pad
		// the bound by another 2 * dblEpsilon.
		maxLat := math.Min(
			math.Atan2(math.Sqrt(n.X*n.X+n.Y*n.Y), math.Abs(n.Z))+3*dblEpsilon,
			math.Pi/2)

		// In order to get tight bounds when the two points are close together,
		// we also bound the min/max latitude relative to the latitudes of the
		// endpoints A and B. First we compute the distance between A and B,
		// and then we compute the maximum change in latitude between any two
		// points along the great circle that are separated by this distance.
		// This gives us a latitude change "budget". Some of this budget must
		// be spent getting from A to B; the remainder bounds the round-trip
		// distance (in latitude) from A or B to the min or max latitude
		// attained along the edge AB.
		latBudget := 2 * math.Asin(0.5*(r.a.Sub(b.Vector)).Norm()*math.Sin(maxLat))
		maxDelta := 0.5*(latBudget-latAB.Length()) + dblEpsilon

		// Test whether AB passes through the point of maximum latitude or
		// minimum latitude. If the dot product(s) are small enough then the
		// result may be ambiguous.
		if mA <= mError && mB >= -mError {
			latAB.Hi = math.Min(maxLat, latAB.Hi+maxDelta)
		}
		if mB <= mError && mA >= -mError {
			latAB.Lo = math.Max(-maxLat, latAB.Lo-maxDelta)
		}
	}
	r.a = b
	r.aLL = bLL
	r.bound = r.bound.Union(Rect{latAB, lngAB})
}

// RectBound returns the bounding rectangle of the edge chain that connects the
// vertices defined so far. This bound satisfies the guarantee made
// above, i.e. if the edge chain defines a Loop, then the bound contains
// the LatLng coordinates of all Points contained by the loop.
func (r *RectBounder) RectBound() Rect {
	return r.bound.expanded(LatLng{s1.Angle(2 * dblEpsilon), 0}).PolarClosure()
}

// ExpandForSubregions expands a bounding Rect so that it is guaranteed to
// contain the bounds of any subregion whose bounds are computed using
// ComputeRectBound. For example, consider a loop L that defines a square.
// GetBound ensures that if a point P is contained by this square, then
// LatLngFromPoint(P) is contained by the bound. But now consider a diamond
// shaped loop S contained by L. It is possible that GetBound returns a
// *larger* bound for S than it does for L, due to rounding errors. This
// method expands the bound for L so that it is guaranteed to contain the
// bounds of any subregion S.
//
// More precisely, if L is a loop that does not contain either pole, and S
// is a loop such that L.Contains(S), then
//
//   ExpandForSubregions(L.RectBound).Contains(S.RectBound).
//
func ExpandForSubregions(bound Rect) Rect {
	// Empty bounds don't need expansion.
	if bound.IsEmpty() {
		return bound
	}

	// First we need to check whether the bound B contains any nearly-antipodal
	// points (to within 4.309 * dblEpsilon). If so then we need to return
	// FullRect, since the subregion might have an edge between two
	// such points, and AddPoint returns Full for such edges. Note that
	// this can happen even if B is not Full for example, consider a loop
	// that defines a 10km strip straddling the equator extending from
	// longitudes -100 to +100 degrees.
	//
	// It is easy to check whether B contains any antipodal points, but checking
	// for nearly-antipodal points is trickier. Essentially we consider the
	// original bound B and its reflection through the origin B', and then test
	// whether the minimum distance between B and B' is less than 4.309 * dblEpsilon.

	// lngGap is a lower bound on the longitudinal distance between B and its
	// reflection B'. (2.5 * dblEpsilon is the maximum combined error of the
	// endpoint longitude calculations and the Length call.)
	lngGap := math.Max(0, math.Pi-bound.Lng.Length()-2.5*dblEpsilon)

	// minAbsLat is the minimum distance from B to the equator (if zero or
	// negative, then B straddles the equator).
	minAbsLat := math.Max(bound.Lat.Lo, -bound.Lat.Hi)

	// latGapSouth and latGapNorth measure the minimum distance from B to the
	// south and north poles respectively.
	latGapSouth := math.Pi/2 + bound.Lat.Lo
	latGapNorth := math.Pi/2 - bound.Lat.Hi

	if minAbsLat >= 0 {
		// The bound B does not straddle the equator. In this case the minimum
		// distance is between one endpoint of the latitude edge in B closest to
		// the equator and the other endpoint of that edge in B'. The latitude
		// distance between these two points is 2*minAbsLat, and the longitude
		// distance is lngGap. We could compute the distance exactly using the
		// Haversine formula, but then we would need to bound the errors in that
		// calculation. Since we only need accuracy when the distance is very
		// small (close to 4.309 * dblEpsilon), we substitute the Euclidean
		// distance instead. This gives us a right triangle XYZ with two edges of
		// length x = 2*minAbsLat and y ~= lngGap. The desired distance is the
		// length of the third edge z, and we have
		//
		//         z  ~=  sqrt(x^2 + y^2)  >=  (x + y) / sqrt(2)
		//
		// Therefore the region may contain nearly antipodal points only if
		//
		//  2*minAbsLat + lngGap  <  sqrt(2) * 4.309 * dblEpsilon
		//                        ~= 1.354e-15
		//
		// Note that because the given bound B is conservative, minAbsLat and
		// lngGap are both lower bounds on their true values so we do not need
		// to make any adjustments for their errors.
		if 2*minAbsLat+lngGap < 1.354e-15 {
			return FullRect()
		}
	} else if lngGap >= math.Pi/2 {
		// B spans at most Pi/2 in longitude. The minimum distance is always
		// between one corner of B and the diagonally opposite corner of B'. We
		// use the same distance approximation that we used above; in this case
		// we have an obtuse triangle XYZ with two edges of length x = latGapSouth
		// and y = latGapNorth, and angle Z >= Pi/2 between them. We then have
		//
		//         z  >=  sqrt(x^2 + y^2)  >=  (x + y) / sqrt(2)
		//
		// Unlike the case above, latGapSouth and latGapNorth are not lower bounds
		// (because of the extra addition operation, and because math.Pi/2 is not
		// exactly equal to Pi/2); they can exceed their true values by up to
		// 0.75 * dblEpsilon. Putting this all together, the region may contain
		// nearly antipodal points only if
		//
		//   latGapSouth + latGapNorth  <  (sqrt(2) * 4.309 + 1.5) * dblEpsilon
		//                              ~= 1.687e-15
		if latGapSouth+latGapNorth < 1.687e-15 {
			return FullRect()
		}
	} else {
		// Otherwise we know that (1) the bound straddles the equator and (2) its
		// width in longitude is at least Pi/2. In this case the minimum
		// distance can occur either between a corner of B and the diagonally
		// opposite corner of B' (as in the case above), or between a corner of B
		// and the opposite longitudinal edge reflected in B'. It is sufficient
		// to only consider the corner-edge case, since this distance is also a
		// lower bound on the corner-corner distance when that case applies.

		// Consider the spherical triangle XYZ where X is a corner of B with
		// minimum absolute latitude, Y is the closest pole to X, and Z is the
		// point closest to X on the opposite longitudinal edge of B'. This is a
		// right triangle (Z = Pi/2), and from the spherical law of sines we have
		//
		//     sin(z) / sin(Z)  =  sin(y) / sin(Y)
		//     sin(maxLatGap) / 1  =  sin(dMin) / sin(lngGap)
		//     sin(dMin)  =  sin(maxLatGap) * sin(lngGap)
		//
		// where "maxLatGap" = max(latGapSouth, latGapNorth) and "dMin" is the
		// desired minimum distance. Now using the facts that sin(t) >= (2/Pi)*t
		// for 0 <= t <= Pi/2, that we only need an accurate approximation when
		// at least one of "maxLatGap" or lngGap is extremely small (in which
		// case sin(t) ~= t), and recalling that "maxLatGap" has an error of up
		// to 0.75 * dblEpsilon, we want to test whether
		//
		//   maxLatGap * lngGap  <  (4.309 + 0.75) * (Pi/2) * dblEpsilon
		//                       ~= 1.765e-15
		if math.Max(latGapSouth, latGapNorth)*lngGap < 1.765e-15 {
			return FullRect()
		}
	}
	// Next we need to check whether the subregion might contain any edges that
	// span (math.Pi - 2 * dblEpsilon) radians or more in longitude, since AddPoint
	// sets the longitude bound to Full in that case. This corresponds to
	// testing whether (lngGap <= 0) in lngExpansion below.

	// Otherwise, the maximum latitude error in AddPoint is 4.8 * dblEpsilon.
	// In the worst case, the errors when computing the latitude bound for a
	// subregion could go in the opposite direction as the errors when computing
	// the bound for the original region, so we need to double this value.
	// (More analysis shows that it's okay to round down to a multiple of
	// dblEpsilon.)
	//
	// For longitude, we rely on the fact that atan2 is correctly rounded and
	// therefore no additional bounds expansion is necessary.

	latExpansion := 9 * dblEpsilon
	lngExpansion := 0.0
	if lngGap <= 0 {
		lngExpansion = math.Pi
	}
	return bound.expanded(LatLng{s1.Angle(latExpansion), s1.Angle(lngExpansion)}).PolarClosure()
}

// EdgeCrosser allows edges to be efficiently tested for intersection with a
// given fixed edge AB. It is especially efficient when testing for
// intersection with an edge chain connecting vertices v0, v1, v2, ...
type EdgeCrosser struct {
	a   Point
	b   Point
	aXb Point

	// To reduce the number of calls to expensiveSign, we compute an
	// outward-facing tangent at A and B if necessary. If the plane
	// perpendicular to one of these tangents separates AB from CD (i.e., one
	// edge on each side) then there is no intersection.
	aTangent Point // Outward-facing tangent at A.
	bTangent Point // Outward-facing tangent at B.

	// The fields below are updated for each vertex in the chain.
	c   Point     // Previous vertex in the vertex chain.
	acb Direction // The orientation of triangle ACB.
}

// NewEdgeCrosser returns an EdgeCrosser with the fixed edge AB.
func NewEdgeCrosser(a, b Point) *EdgeCrosser {
	norm := a.PointCross(b)
	return &EdgeCrosser{
		a:        a,
		b:        b,
		aXb:      a.PointCross(b),
		aTangent: Point{a.Cross(norm.Vector)},
		bTangent: Point{norm.Cross(b.Vector)},
	}
}

// A Crossing indicates how edges cross.
type Crossing int

const (
	// Cross means the edges cross.
	Cross Crossing = iota
	// MaybeCross means two vertices from different edges are the same.
	MaybeCross
	// DoNotCross means the edges do not cross.
	DoNotCross
)

// CrossingSign reports whether the edge AB intersects the edge CD.
// If any two vertices from different edges are the same, returns MaybeCross.
// If either edge is degenerate (A == B or C == D), returns DoNotCross or MaybeCross.
//
// Properties of CrossingSign:
//
//  (1) CrossingSign(b,a,c,d) == CrossingSign(a,b,c,d)
//  (2) CrossingSign(c,d,a,b) == CrossingSign(a,b,c,d)
//  (3) CrossingSign(a,b,c,d) == MaybeCross if a==c, a==d, b==c, b==d
//  (3) CrossingSign(a,b,c,d) == DoNotCross or MaybeCross if a==b or c==d
//
// Note that if you want to check an edge against a chain of other edges,
// it is slightly more efficient to use the single-argument version
// ChainCrossingSign below.
func (e *EdgeCrosser) CrossingSign(c, d Point) Crossing {
	if c != e.c {
		e.RestartAt(c)
	}
	return e.ChainCrossingSign(d)
}

// EdgeOrVertexCrossing reports whether if CrossingSign(c, d) > 0, or AB and
// CD share a vertex and VertexCrossing(a, b, c, d) is true.
//
// This method extends the concept of a "crossing" to the case where AB
// and CD have a vertex in common. The two edges may or may not cross,
// according to the rules defined in VertexCrossing above. The rules
// are designed so that point containment tests can be implemented simply
// by counting edge crossings. Similarly, determining whether one edge
// chain crosses another edge chain can be implemented by counting.
func (e *EdgeCrosser) EdgeOrVertexCrossing(c, d Point) bool {
	if c != e.c {
		e.RestartAt(c)
	}
	return e.EdgeOrVertexChainCrossing(d)
}

// NewChainEdgeCrosser is a convenience constructor that uses AB as the fixed edge,
// and C as the first vertex of the vertex chain (equivalent to calling RestartAt(c)).
//
// You don't need to use this or any of the chain functions unless you're trying to
// squeeze out every last drop of performance. Essentially all you are saving is a test
// whether the first vertex of the current edge is the same as the second vertex of the
// previous edge.
func NewChainEdgeCrosser(a, b, c Point) *EdgeCrosser {
	e := NewEdgeCrosser(a, b)
	e.RestartAt(c)
	return e
}

// RestartAt sets the current point of the edge crosser to be c.
// Call this method when your chain 'jumps' to a new place.
// The argument must point to a value that persists until the next call.
func (e *EdgeCrosser) RestartAt(c Point) {
	e.c = c
	e.acb = -triageSign(e.a, e.b, e.c)
}

// ChainCrossingSign is like CrossingSign, but uses the last vertex passed to one of
// the crossing methods (or RestartAt) as the first vertex of the current edge.
func (e *EdgeCrosser) ChainCrossingSign(d Point) Crossing {
	// For there to be an edge crossing, the triangles ACB, CBD, BDA, DAC must
	// all be oriented the same way (CW or CCW). We keep the orientation of ACB
	// as part of our state. When each new point D arrives, we compute the
	// orientation of BDA and check whether it matches ACB. This checks whether
	// the points C and D are on opposite sides of the great circle through AB.

	// Recall that triageSign is invariant with respect to rotating its
	// arguments, i.e. ABD has the same orientation as BDA.
	bda := triageSign(e.a, e.b, d)
	if e.acb == -bda && bda != Indeterminate {
		// The most common case -- triangles have opposite orientations. Save the
		// current vertex D as the next vertex C, and also save the orientation of
		// the new triangle ACB (which is opposite to the current triangle BDA).
		e.c = d
		e.acb = -bda
		return DoNotCross
	}
	return e.crossingSign(d, bda)
}

// EdgeOrVertexChainCrossing is like EdgeOrVertexCrossing, but uses the last vertex
// passed to one of the crossing methods (or RestartAt) as the first vertex of the current edge.
func (e *EdgeCrosser) EdgeOrVertexChainCrossing(d Point) bool {
	// We need to copy e.c since it is clobbered by ChainCrossingSign.
	c := e.c
	switch e.ChainCrossingSign(d) {
	case DoNotCross:
		return false
	case Cross:
		return true
	}
	return VertexCrossing(e.a, e.b, c, d)
}

// crossingSign handle the slow path of CrossingSign.
func (e *EdgeCrosser) crossingSign(d Point, bda Direction) Crossing {
	// Compute the actual result, and then save the current vertex D as the next
	// vertex C, and save the orientation of the next triangle ACB (which is
	// opposite to the current triangle BDA).
	defer func() {
		e.c = d
		e.acb = -bda
	}()

	// RobustSign is very expensive, so we avoid calling it if at all possible.
	// First eliminate the cases where two vertices are equal.
	if e.a == e.c || e.a == d || e.b == e.c || e.b == d {
		return MaybeCross
	}

	// At this point, a very common situation is that A,B,C,D are four points on
	// a line such that AB does not overlap CD. (For example, this happens when
	// a line or curve is sampled finely, or when geometry is constructed by
	// computing the union of S2CellIds.) Most of the time, we can determine
	// that AB and CD do not intersect using the two outward-facing
	// tangents at A and B (parallel to AB) and testing whether AB and CD are on
	// opposite sides of the plane perpendicular to one of these tangents. This
	// is moderately expensive but still much cheaper than expensiveSign.

	// The error in RobustCrossProd is insignificant. The maximum error in
	// the call to CrossProd (i.e., the maximum norm of the error vector) is
	// (0.5 + 1/sqrt(3)) * dblEpsilon. The maximum error in each call to
	// DotProd below is dblEpsilon. (There is also a small relative error
	// term that is insignificant because we are comparing the result against a
	// constant that is very close to zero.)
	maxError := (1.5 + 1/math.Sqrt(3)) * dblEpsilon
	if (e.c.Dot(e.aTangent.Vector) > maxError && d.Dot(e.aTangent.Vector) > maxError) || (e.c.Dot(e.bTangent.Vector) > maxError && d.Dot(e.bTangent.Vector) > maxError) {
		return DoNotCross
	}

	// Otherwise it's time to break out the big guns.
	if e.acb == Indeterminate {
		e.acb = -expensiveSign(e.a, e.b, e.c)
	}
	if bda == Indeterminate {
		bda = expensiveSign(e.a, e.b, d)
	}

	if bda != e.acb {
		return DoNotCross
	}

	cbd := -RobustSign(e.c, d, e.b)
	if cbd != e.acb {
		return DoNotCross
	}
	dac := RobustSign(e.c, d, e.a)
	if dac == e.acb {
		return Cross
	}
	return DoNotCross
}

// pointUVW represents a Point in (u,v,w) coordinate space of a cube face.
type pointUVW Point

// intersectsFace reports whether a given directed line L intersects the cube face F.
// The line L is defined by its normal N in the (u,v,w) coordinates of F.
func (p pointUVW) intersectsFace() bool {
	// L intersects the [-1,1]x[-1,1] square in (u,v) if and only if the dot
	// products of N with the four corner vertices (-1,-1,1), (1,-1,1), (1,1,1),
	// and (-1,1,1) do not all have the same sign. This is true exactly when
	// |Nu| + |Nv| >= |Nw|. The code below evaluates this expression exactly.
	u := math.Abs(p.X)
	v := math.Abs(p.Y)
	w := math.Abs(p.Z)

	// We only need to consider the cases where u or v is the smallest value,
	// since if w is the smallest then both expressions below will have a
	// positive LHS and a negative RHS.
	return (v >= w-u) && (u >= w-v)
}

// intersectsOppositeEdges reports whether a directed line L intersects two
// opposite edges of a cube face F. This includs the case where L passes
// exactly through a corner vertex of F. The directed line L is defined
// by its normal N in the (u,v,w) coordinates of F.
func (p pointUVW) intersectsOppositeEdges() bool {
	// The line L intersects opposite edges of the [-1,1]x[-1,1] (u,v) square if
	// and only exactly two of the corner vertices lie on each side of L. This
	// is true exactly when ||Nu| - |Nv|| >= |Nw|. The code below evaluates this
	// expression exactly.
	u := math.Abs(p.X)
	v := math.Abs(p.Y)
	w := math.Abs(p.Z)

	// If w is the smallest, the following line returns an exact result.
	if math.Abs(u-v) != w {
		return math.Abs(u-v) >= w
	}

	// Otherwise u - v = w exactly, or w is not the smallest value. In either
	// case the following returns the correct result.
	if u >= v {
		return u-w >= v
	}
	return v-w >= u
}

// axis represents the possible results of exitAxis.
type axis int

const (
	axisU axis = iota
	axisV
)

// exitAxis reports which axis the directed line L exits the cube face F on.
// The directed line L is represented by its CCW normal N in the (u,v,w) coordinates
// of F. It returns axisU if L exits through the u=-1 or u=+1 edge, and axisV if L exits
// through the v=-1 or v=+1 edge. Either result is acceptable if L exits exactly
// through a corner vertex of the cube face.
func (p pointUVW) exitAxis() axis {
	if p.intersectsOppositeEdges() {
		// The line passes through through opposite edges of the face.
		// It exits through the v=+1 or v=-1 edge if the u-component of N has a
		// larger absolute magnitude than the v-component.
		if math.Abs(p.X) >= math.Abs(p.Y) {
			return axisV
		}
		return axisU
	}

	// The line passes through through two adjacent edges of the face.
	// It exits the v=+1 or v=-1 edge if an even number of the components of N
	// are negative. We test this using signbit() rather than multiplication
	// to avoid the possibility of underflow.
	var x, y, z int
	if math.Signbit(p.X) {
		x = 1
	}
	if math.Signbit(p.Y) {
		y = 1
	}
	if math.Signbit(p.Z) {
		z = 1
	}

	if x^y^z == 0 {
		return axisV
	}
	return axisU
}

// exitPoint returns the UV coordinates of the point where a directed line L (represented
// by the CCW normal of this point), exits the cube face this point is derived from along
// the given axis.
func (p pointUVW) exitPoint(a axis) r2.Point {
	if a == axisU {
		u := -1.0
		if p.Y > 0 {
			u = 1.0
		}
		return r2.Point{u, (-u*p.X - p.Z) / p.Y}
	}

	v := -1.0
	if p.X < 0 {
		v = 1.0
	}
	return r2.Point{(-v*p.Y - p.Z) / p.X, v}
}

// clipDestination returns a score which is used to indicate if the clipped edge AB
// on the given face intersects the face at all. This function returns the score for
// the given endpoint, which is an integer ranging from 0 to 3. If the sum of the scores
// from both of the endpoints is 3 or more, then edge AB does not intersect this face.
//
// First, it clips the line segment AB to find the clipped destination B' on a given
// face. (The face is specified implicitly by expressing *all arguments* in the (u,v,w)
// coordinates of that face.) Second, it partially computes whether the segment AB
// intersects this face at all. The actual condition is fairly complicated, but it
// turns out that it can be expressed as a "score" that can be computed independently
// when clipping the two endpoints A and B.
func clipDestination(a, b, scaledN, aTan, bTan pointUVW, scaleUV float64) (r2.Point, int) {
	var uv r2.Point

	// Optimization: if B is within the safe region of the face, use it.
	maxSafeUVCoord := 1 - faceClipErrorUVCoord
	if b.Z > 0 {
		uv = r2.Point{b.X / b.Z, b.Y / b.Z}
		if math.Max(math.Abs(uv.X), math.Abs(uv.Y)) <= maxSafeUVCoord {
			return uv, 0
		}
	}

	// Otherwise find the point B' where the line AB exits the face.
	uv = scaledN.exitPoint(scaledN.exitAxis()).Mul(scaleUV)

	p := pointUVW(PointFromCoords(uv.X, uv.Y, 1.0))

	// Determine if the exit point B' is contained within the segment. We do this
	// by computing the dot products with two inward-facing tangent vectors at A
	// and B. If either dot product is negative, we say that B' is on the "wrong
	// side" of that point. As the point B' moves around the great circle AB past
	// the segment endpoint B, it is initially on the wrong side of B only; as it
	// moves further it is on the wrong side of both endpoints; and then it is on
	// the wrong side of A only. If the exit point B' is on the wrong side of
	// either endpoint, we can't use it; instead the segment is clipped at the
	// original endpoint B.
	//
	// We reject the segment if the sum of the scores of the two endpoints is 3
	// or more. Here is what that rule encodes:
	//  - If B' is on the wrong side of A, then the other clipped endpoint A'
	//    must be in the interior of AB (otherwise AB' would go the wrong way
	//    around the circle). There is a similar rule for A'.
	//  - If B' is on the wrong side of either endpoint (and therefore we must
	//    use the original endpoint B instead), then it must be possible to
	//    project B onto this face (i.e., its w-coordinate must be positive).
	//    This rule is only necessary to handle certain zero-length edges (A=B).
	score := 0
	if p.Sub(a.Vector).Dot(aTan.Vector) < 0 {
		score = 2 // B' is on wrong side of A.
	} else if p.Sub(b.Vector).Dot(bTan.Vector) < 0 {
		score = 1 // B' is on wrong side of B.
	}

	if score > 0 { // B' is not in the interior of AB.
		if b.Z <= 0 {
			score = 3 // B cannot be projected onto this face.
		} else {
			uv = r2.Point{b.X / b.Z, b.Y / b.Z}
		}
	}

	return uv, score
}

// ClipToFace returns the (u,v) coordinates for the portion of the edge AB that
// intersects the given face, or false if the edge AB does not intersect.
// This method guarantees that the clipped vertices lie within the [-1,1]x[-1,1]
// cube face rectangle and are within faceClipErrorUVDist of the line AB, but
// the results may differ from those produced by faceSegments.
func ClipToFace(a, b Point, face int) (aUV, bUV r2.Point, intersects bool) {
	return ClipToPaddedFace(a, b, face, 0.0)
}

// ClipToPaddedFace returns the (u,v) coordinates for the portion of the edge AB that
// intersects the given face, but rather than clipping to the square [-1,1]x[-1,1]
// in (u,v) space, this method clips to [-R,R]x[-R,R] where R=(1+padding).
// Padding must be non-negative.
func ClipToPaddedFace(a, b Point, f int, padding float64) (aUV, bUV r2.Point, intersects bool) {
	// Fast path: both endpoints are on the given face.
	if face(a.Vector) == f && face(b.Vector) == f {
		au, av := validFaceXYZToUV(f, a.Vector)
		bu, bv := validFaceXYZToUV(f, b.Vector)
		return r2.Point{au, av}, r2.Point{bu, bv}, true
	}

	// Convert everything into the (u,v,w) coordinates of the given face. Note
	// that the cross product *must* be computed in the original (x,y,z)
	// coordinate system because PointCross (unlike the mathematical cross
	// product) can produce different results in different coordinate systems
	// when one argument is a linear multiple of the other, due to the use of
	// symbolic perturbations.
	normUVW := pointUVW(faceXYZtoUVW(f, a.PointCross(b)))
	aUVW := pointUVW(faceXYZtoUVW(f, a))
	bUVW := pointUVW(faceXYZtoUVW(f, b))

	// Padding is handled by scaling the u- and v-components of the normal.
	// Letting R=1+padding, this means that when we compute the dot product of
	// the normal with a cube face vertex (such as (-1,-1,1)), we will actually
	// compute the dot product with the scaled vertex (-R,-R,1). This allows
	// methods such as intersectsFace, exitAxis, etc, to handle padding
	// with no further modifications.
	scaleUV := 1 + padding
	scaledN := pointUVW{r3.Vector{X: scaleUV * normUVW.X, Y: scaleUV * normUVW.Y, Z: normUVW.Z}}
	if !scaledN.intersectsFace() {
		return aUV, bUV, false
	}

	// TODO(roberts): This is a workaround for extremely small vectors where some
	// loss of precision can occur in Normalize causing underflow. When PointCross
	// is updated to work around this, this can be removed.
	if math.Max(math.Abs(normUVW.X), math.Max(math.Abs(normUVW.Y), math.Abs(normUVW.Z))) < math.Ldexp(1, -511) {
		normUVW = pointUVW{normUVW.Mul(math.Ldexp(1, 563))}
	}

	normUVW = pointUVW{normUVW.Normalize()}

	aTan := pointUVW{normUVW.Cross(aUVW.Vector)}
	bTan := pointUVW{bUVW.Cross(normUVW.Vector)}

	// As described in clipDestination, if the sum of the scores from clipping the two
	// endpoints is 3 or more, then the segment does not intersect this face.
	aUV, aScore := clipDestination(bUVW, aUVW, pointUVW{scaledN.Mul(-1)}, bTan, aTan, scaleUV)
	bUV, bScore := clipDestination(aUVW, bUVW, scaledN, aTan, bTan, scaleUV)

	return aUV, bUV, aScore+bScore < 3
}
