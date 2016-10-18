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

package r3

import (
	"fmt"
	"math"

	"github.com/golang/geo/s1"
)

// Vector represents a point in ℝ³.
type Vector struct {
	X, Y, Z float64
}

// ApproxEqual reports whether v and ov are equal within a small epsilon.
func (v Vector) ApproxEqual(ov Vector) bool {
	const epsilon = 1e-16
	return math.Abs(v.X-ov.X) < epsilon && math.Abs(v.Y-ov.Y) < epsilon && math.Abs(v.Z-ov.Z) < epsilon
}

func (v Vector) String() string { return fmt.Sprintf("(%v, %v, %v)", v.X, v.Y, v.Z) }

// Norm returns the vector's norm.
func (v Vector) Norm() float64 { return math.Sqrt(v.Dot(v)) }

// Norm2 returns the square of the norm.
func (v Vector) Norm2() float64 { return v.Dot(v) }

// Normalize returns a unit vector in the same direction as v.
func (v Vector) Normalize() Vector {
	if v == (Vector{0, 0, 0}) {
		return v
	}
	return v.Mul(1 / v.Norm())
}

// IsUnit returns whether this vector is of approximately unit length.
func (v Vector) IsUnit() bool {
	const epsilon = 5e-14
	return math.Abs(v.Norm2()-1) <= epsilon
}

// Abs returns the vector with nonnegative components.
func (v Vector) Abs() Vector { return Vector{math.Abs(v.X), math.Abs(v.Y), math.Abs(v.Z)} }

// Add returns the standard vector sum of v and ov.
func (v Vector) Add(ov Vector) Vector { return Vector{v.X + ov.X, v.Y + ov.Y, v.Z + ov.Z} }

// Sub returns the standard vector difference of v and ov.
func (v Vector) Sub(ov Vector) Vector { return Vector{v.X - ov.X, v.Y - ov.Y, v.Z - ov.Z} }

// Mul returns the standard scalar product of v and m.
func (v Vector) Mul(m float64) Vector { return Vector{m * v.X, m * v.Y, m * v.Z} }

// Dot returns the standard dot product of v and ov.
func (v Vector) Dot(ov Vector) float64 { return v.X*ov.X + v.Y*ov.Y + v.Z*ov.Z }

// Cross returns the standard cross product of v and ov.
func (v Vector) Cross(ov Vector) Vector {
	return Vector{
		v.Y*ov.Z - v.Z*ov.Y,
		v.Z*ov.X - v.X*ov.Z,
		v.X*ov.Y - v.Y*ov.X,
	}
}

// Distance returns the Euclidean distance between v and ov.
func (v Vector) Distance(ov Vector) float64 { return v.Sub(ov).Norm() }

// Angle returns the angle between v and ov.
func (v Vector) Angle(ov Vector) s1.Angle {
	return s1.Angle(math.Atan2(v.Cross(ov).Norm(), v.Dot(ov))) * s1.Radian
}

// Ortho returns a unit vector that is orthogonal to v.
// Ortho(-v) = -Ortho(v) for all v.
func (v Vector) Ortho() Vector {
	ov := Vector{0.012, 0.0053, 0.00457}
	// Grow a component other than the largest in v, to guarantee that they aren't
	// parallel (which would make the cross product zero).
	if math.Abs(v.X) > math.Abs(v.Y) {
		ov.Y = 1
	} else {
		ov.X = 1
	}
	return v.Cross(ov).Normalize()
}
