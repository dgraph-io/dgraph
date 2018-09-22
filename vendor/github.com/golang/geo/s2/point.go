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
	"io"
	"math"

	"github.com/golang/geo/r3"
	"github.com/golang/geo/s1"
)

// Point represents a point on the unit sphere as a normalized 3D vector.
// Fields should be treated as read-only. Use one of the factory methods for creation.
type Point struct {
	r3.Vector
}

// PointFromCoords creates a new normalized point from coordinates.
//
// This always returns a valid point. If the given coordinates can not be normalized
// the origin point will be returned.
//
// This behavior is different from the C++ construction of a S2Point from coordinates
// (i.e. S2Point(x, y, z)) in that in C++ they do not Normalize.
func PointFromCoords(x, y, z float64) Point {
	if x == 0 && y == 0 && z == 0 {
		return OriginPoint()
	}
	return Point{r3.Vector{x, y, z}.Normalize()}
}

// OriginPoint returns a unique "origin" on the sphere for operations that need a fixed
// reference point. In particular, this is the "point at infinity" used for
// point-in-polygon testing (by counting the number of edge crossings).
//
// It should *not* be a point that is commonly used in edge tests in order
// to avoid triggering code to handle degenerate cases (this rules out the
// north and south poles). It should also not be on the boundary of any
// low-level S2Cell for the same reason.
func OriginPoint() Point {
	return Point{r3.Vector{-0.0099994664350250197, 0.0025924542609324121, 0.99994664350250195}}
}

// PointCross returns a Point that is orthogonal to both p and op. This is similar to
// p.Cross(op) (the true cross product) except that it does a better job of
// ensuring orthogonality when the Point is nearly parallel to op, it returns
// a non-zero result even when p == op or p == -op and the result is a Point.
//
// It satisfies the following properties (f == PointCross):
//
//   (1) f(p, op) != 0 for all p, op
//   (2) f(op,p) == -f(p,op) unless p == op or p == -op
//   (3) f(-p,op) == -f(p,op) unless p == op or p == -op
//   (4) f(p,-op) == -f(p,op) unless p == op or p == -op
func (p Point) PointCross(op Point) Point {
	// NOTE(dnadasi): In the C++ API the equivalent method here was known as "RobustCrossProd",
	// but PointCross more accurately describes how this method is used.
	x := p.Add(op.Vector).Cross(op.Sub(p.Vector))

	// Compare exactly to the 0 vector.
	if x == (r3.Vector{}) {
		// The only result that makes sense mathematically is to return zero, but
		// we find it more convenient to return an arbitrary orthogonal vector.
		return Point{p.Ortho()}
	}

	return Point{x}
}

// OrderedCCW returns true if the edges OA, OB, and OC are encountered in that
// order while sweeping CCW around the point O.
//
// You can think of this as testing whether A <= B <= C with respect to the
// CCW ordering around O that starts at A, or equivalently, whether B is
// contained in the range of angles (inclusive) that starts at A and extends
// CCW to C. Properties:
//
//  (1) If OrderedCCW(a,b,c,o) && OrderedCCW(b,a,c,o), then a == b
//  (2) If OrderedCCW(a,b,c,o) && OrderedCCW(a,c,b,o), then b == c
//  (3) If OrderedCCW(a,b,c,o) && OrderedCCW(c,b,a,o), then a == b == c
//  (4) If a == b or b == c, then OrderedCCW(a,b,c,o) is true
//  (5) Otherwise if a == c, then OrderedCCW(a,b,c,o) is false
func OrderedCCW(a, b, c, o Point) bool {
	sum := 0
	if RobustSign(b, o, a) != Clockwise {
		sum++
	}
	if RobustSign(c, o, b) != Clockwise {
		sum++
	}
	if RobustSign(a, o, c) == CounterClockwise {
		sum++
	}
	return sum >= 2
}

// Distance returns the angle between two points.
func (p Point) Distance(b Point) s1.Angle {
	return p.Vector.Angle(b.Vector)
}

// ApproxEqual reports whether the two points are similar enough to be equal.
func (p Point) ApproxEqual(other Point) bool {
	return p.Vector.Angle(other.Vector) <= s1.Angle(epsilon)
}

// PointArea returns the area on the unit sphere for the triangle defined by the
// given points.
//
// This method is based on l'Huilier's theorem,
//
//   tan(E/4) = sqrt(tan(s/2) tan((s-a)/2) tan((s-b)/2) tan((s-c)/2))
//
// where E is the spherical excess of the triangle (i.e. its area),
//       a, b, c are the side lengths, and
//       s is the semiperimeter (a + b + c) / 2.
//
// The only significant source of error using l'Huilier's method is the
// cancellation error of the terms (s-a), (s-b), (s-c). This leads to a
// *relative* error of about 1e-16 * s / min(s-a, s-b, s-c). This compares
// to a relative error of about 1e-15 / E using Girard's formula, where E is
// the true area of the triangle. Girard's formula can be even worse than
// this for very small triangles, e.g. a triangle with a true area of 1e-30
// might evaluate to 1e-5.
//
// So, we prefer l'Huilier's formula unless dmin < s * (0.1 * E), where
// dmin = min(s-a, s-b, s-c). This basically includes all triangles
// except for extremely long and skinny ones.
//
// Since we don't know E, we would like a conservative upper bound on
// the triangle area in terms of s and dmin. It's possible to show that
// E <= k1 * s * sqrt(s * dmin), where k1 = 2*sqrt(3)/Pi (about 1).
// Using this, it's easy to show that we should always use l'Huilier's
// method if dmin >= k2 * s^5, where k2 is about 1e-2. Furthermore,
// if dmin < k2 * s^5, the triangle area is at most k3 * s^4, where
// k3 is about 0.1. Since the best case error using Girard's formula
// is about 1e-15, this means that we shouldn't even consider it unless
// s >= 3e-4 or so.
func PointArea(a, b, c Point) float64 {
	sa := float64(b.Angle(c.Vector))
	sb := float64(c.Angle(a.Vector))
	sc := float64(a.Angle(b.Vector))
	s := 0.5 * (sa + sb + sc)
	if s >= 3e-4 {
		// Consider whether Girard's formula might be more accurate.
		dmin := s - math.Max(sa, math.Max(sb, sc))
		if dmin < 1e-2*s*s*s*s*s {
			// This triangle is skinny enough to use Girard's formula.
			area := GirardArea(a, b, c)
			if dmin < s*0.1*area {
				return area
			}
		}
	}

	// Use l'Huilier's formula.
	return 4 * math.Atan(math.Sqrt(math.Max(0.0, math.Tan(0.5*s)*math.Tan(0.5*(s-sa))*
		math.Tan(0.5*(s-sb))*math.Tan(0.5*(s-sc)))))
}

// GirardArea returns the area of the triangle computed using Girard's formula.
// All points should be unit length, and no two points should be antipodal.
//
// This method is about twice as fast as PointArea() but has poor relative
// accuracy for small triangles. The maximum error is about 5e-15 (about
// 0.25 square meters on the Earth's surface) and the average error is about
// 1e-15. These bounds apply to triangles of any size, even as the maximum
// edge length of the triangle approaches 180 degrees. But note that for
// such triangles, tiny perturbations of the input points can change the
// true mathematical area dramatically.
func GirardArea(a, b, c Point) float64 {
	// This is equivalent to the usual Girard's formula but is slightly more
	// accurate, faster to compute, and handles a == b == c without a special
	// case. PointCross is necessary to get good accuracy when two of
	// the input points are very close together.
	ab := a.PointCross(b)
	bc := b.PointCross(c)
	ac := a.PointCross(c)
	area := float64(ab.Angle(ac.Vector) - ab.Angle(bc.Vector) + bc.Angle(ac.Vector))
	if area < 0 {
		area = 0
	}
	return area
}

// SignedArea returns a positive value for counterclockwise triangles and a negative
// value otherwise (similar to PointArea).
func SignedArea(a, b, c Point) float64 {
	return float64(RobustSign(a, b, c)) * PointArea(a, b, c)
}

// TrueCentroid returns the true centroid of the spherical triangle ABC multiplied by the
// signed area of spherical triangle ABC. The result is not normalized.
// The reasons for multiplying by the signed area are (1) this is the quantity
// that needs to be summed to compute the centroid of a union or difference of triangles,
// and (2) it's actually easier to calculate this way. All points must have unit length.
//
// The true centroid (mass centroid) is defined as the surface integral
// over the spherical triangle of (x,y,z) divided by the triangle area.
// This is the point that the triangle would rotate around if it was
// spinning in empty space.
//
// The best centroid for most purposes is the true centroid. Unlike the
// planar and surface centroids, the true centroid behaves linearly as
// regions are added or subtracted. That is, if you split a triangle into
// pieces and compute the average of their centroids (weighted by triangle
// area), the result equals the centroid of the original triangle. This is
// not true of the other centroids.
func TrueCentroid(a, b, c Point) Point {
	ra := float64(1)
	if sa := float64(b.Distance(c)); sa != 0 {
		ra = sa / math.Sin(sa)
	}
	rb := float64(1)
	if sb := float64(c.Distance(a)); sb != 0 {
		rb = sb / math.Sin(sb)
	}
	rc := float64(1)
	if sc := float64(a.Distance(b)); sc != 0 {
		rc = sc / math.Sin(sc)
	}

	// Now compute a point M such that:
	//
	//  [Ax Ay Az] [Mx]                       [ra]
	//  [Bx By Bz] [My]  = 0.5 * det(A,B,C) * [rb]
	//  [Cx Cy Cz] [Mz]                       [rc]
	//
	// To improve the numerical stability we subtract the first row (A) from the
	// other two rows; this reduces the cancellation error when A, B, and C are
	// very close together. Then we solve it using Cramer's rule.
	//
	// This code still isn't as numerically stable as it could be.
	// The biggest potential improvement is to compute B-A and C-A more
	// accurately so that (B-A)x(C-A) is always inside triangle ABC.
	x := r3.Vector{a.X, b.X - a.X, c.X - a.X}
	y := r3.Vector{a.Y, b.Y - a.Y, c.Y - a.Y}
	z := r3.Vector{a.Z, b.Z - a.Z, c.Z - a.Z}
	r := r3.Vector{ra, rb - ra, rc - ra}

	return Point{r3.Vector{y.Cross(z).Dot(r), z.Cross(x).Dot(r), x.Cross(y).Dot(r)}.Mul(0.5)}
}

// PlanarCentroid returns the centroid of the planar triangle ABC, which is not normalized.
// It can be normalized to unit length to obtain the "surface centroid" of the corresponding
// spherical triangle, i.e. the intersection of the three medians. However,
// note that for large spherical triangles the surface centroid may be
// nowhere near the intuitive "center" (see example in TrueCentroid comments).
//
// Note that the surface centroid may be nowhere near the intuitive
// "center" of a spherical triangle. For example, consider the triangle
// with vertices A=(1,eps,0), B=(0,0,1), C=(-1,eps,0) (a quarter-sphere).
// The surface centroid of this triangle is at S=(0, 2*eps, 1), which is
// within a distance of 2*eps of the vertex B. Note that the median from A
// (the segment connecting A to the midpoint of BC) passes through S, since
// this is the shortest path connecting the two endpoints. On the other
// hand, the true centroid is at M=(0, 0.5, 0.5), which when projected onto
// the surface is a much more reasonable interpretation of the "center" of
// this triangle.
func PlanarCentroid(a, b, c Point) Point {
	return Point{a.Add(b.Vector).Add(c.Vector).Mul(1. / 3)}
}

// ChordAngleBetweenPoints constructs a ChordAngle corresponding to the distance
// between the two given points. The points must be unit length.
func ChordAngleBetweenPoints(x, y Point) s1.ChordAngle {
	return s1.ChordAngle(math.Min(4.0, x.Sub(y.Vector).Norm2()))
}

// regularPoints generates a slice of points shaped as a regular polygon with
// the numVertices vertices, all located on a circle of the specified angular radius
// around the center. The radius is the actual distance from center to each vertex.
func regularPoints(center Point, radius s1.Angle, numVertices int) []Point {
	return regularPointsForFrame(getFrame(center), radius, numVertices)
}

// regularPointsForFrame generates a slice of points shaped as a regular polygon
// with numVertices vertices, all on a circle of the specified angular radius around
// the center. The radius is the actual distance from the center to each vertex.
func regularPointsForFrame(frame matrix3x3, radius s1.Angle, numVertices int) []Point {
	// We construct the loop in the given frame coordinates, with the center at
	// (0, 0, 1). For a loop of radius r, the loop vertices have the form
	// (x, y, z) where x^2 + y^2 = sin(r) and z = cos(r). The distance on the
	// sphere (arc length) from each vertex to the center is acos(cos(r)) = r.
	z := math.Cos(radius.Radians())
	r := math.Sin(radius.Radians())
	radianStep := 2 * math.Pi / float64(numVertices)
	var vertices []Point

	for i := 0; i < numVertices; i++ {
		angle := float64(i) * radianStep
		p := Point{r3.Vector{r * math.Cos(angle), r * math.Sin(angle), z}}
		vertices = append(vertices, Point{fromFrame(frame, p).Normalize()})
	}

	return vertices
}

// CapBound returns a bounding cap for this point.
func (p Point) CapBound() Cap {
	return CapFromPoint(p)
}

// RectBound returns a bounding latitude-longitude rectangle from this point.
func (p Point) RectBound() Rect {
	return RectFromLatLng(LatLngFromPoint(p))
}

// ContainsCell returns false as Points do not contain any other S2 types.
func (p Point) ContainsCell(c Cell) bool { return false }

// IntersectsCell reports whether this Point intersects the given cell.
func (p Point) IntersectsCell(c Cell) bool {
	return c.ContainsPoint(p)
}

// ContainsPoint reports if this Point contains the other Point.
// (This method is named to satisfy the Region interface.)
func (p Point) ContainsPoint(other Point) bool {
	return p.Contains(other)
}

// CellUnionBound computes a covering of the Point.
func (p Point) CellUnionBound() []CellID {
	return p.CapBound().CellUnionBound()
}

// Contains reports if this Point contains the other Point.
// (This method matches all other s2 types where the reflexive Contains
// method does not contain the type's name.)
func (p Point) Contains(other Point) bool { return p == other }

// Encode encodes the Point.
func (p Point) Encode(w io.Writer) error {
	e := &encoder{w: w}
	p.encode(e)
	return e.err
}

func (p Point) encode(e *encoder) {
	e.writeInt8(encodingVersion)
	e.writeFloat64(p.X)
	e.writeFloat64(p.Y)
	e.writeFloat64(p.Z)
}

// Angle returns the interior angle at the vertex B in the triangle ABC. The
// return value is always in the range [0, pi]. All points should be
// normalized. Ensures that Angle(a,b,c) == Angle(c,b,a) for all a,b,c.
//
// The angle is undefined if A or C is diametrically opposite from B, and
// becomes numerically unstable as the length of edge AB or BC approaches
// 180 degrees.
func Angle(a, b, c Point) s1.Angle {
	return a.PointCross(b).Angle(c.PointCross(b).Vector)
}

// TurnAngle returns the exterior angle at vertex B in the triangle ABC. The
// return value is positive if ABC is counterclockwise and negative otherwise.
// If you imagine an ant walking from A to B to C, this is the angle that the
// ant turns at vertex B (positive = left = CCW, negative = right = CW).
// This quantity is also known as the "geodesic curvature" at B.
//
// Ensures that TurnAngle(a,b,c) == -TurnAngle(c,b,a) for all distinct
// a,b,c. The result is undefined if (a == b || b == c), but is either
// -Pi or Pi if (a == c). All points should be normalized.
func TurnAngle(a, b, c Point) s1.Angle {
	// We use PointCross to get good accuracy when two points are very
	// close together, and RobustSign to ensure that the sign is correct for
	// turns that are close to 180 degrees.
	angle := a.PointCross(b).Angle(b.PointCross(c).Vector)

	// Don't return RobustSign * angle because it is legal to have (a == c).
	if RobustSign(a, b, c) == CounterClockwise {
		return angle
	}
	return -angle
}

// Rotate the given point about the given axis by the given angle. p and
// axis must be unit length; angle has no restrictions (e.g., it can be
// positive, negative, greater than 360 degrees, etc).
func Rotate(p, axis Point, angle s1.Angle) Point {
	// Let M be the plane through P that is perpendicular to axis, and let
	// center be the point where M intersects axis. We construct a
	// right-handed orthogonal frame (dx, dy, center) such that dx is the
	// vector from center to P, and dy has the same length as dx. The
	// result can then be expressed as (cos(angle)*dx + sin(angle)*dy + center).
	center := axis.Mul(p.Dot(axis.Vector))
	dx := p.Sub(center)
	dy := axis.Cross(p.Vector)
	// Mathematically the result is unit length, but normalization is necessary
	// to ensure that numerical errors don't accumulate.
	return Point{dx.Mul(math.Cos(angle.Radians())).Add(dy.Mul(math.Sin(angle.Radians()))).Add(center).Normalize()}
}
