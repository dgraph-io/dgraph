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

package s1

import (
	"math"
)

// ChordAngle represents the angle subtended by a chord (i.e., the straight
// line segment connecting two points on the sphere). Its representation
// makes it very efficient for computing and comparing distances, but unlike
// Angle it is only capable of representing angles between 0 and π radians.
// Generally, ChordAngle should only be used in loops where many angles need
// to be calculated and compared. Otherwise it is simpler to use Angle.
//
// ChordAngle loses some accuracy as the angle approaches π radians.
// Specifically, the representation of (π - x) radians has an error of about
// (1e-15 / x), with a maximum error of about 2e-8 radians (about 13cm on the
// Earth's surface). For comparison, for angles up to π/2 radians (10000km)
// the worst-case representation error is about 2e-16 radians (1 nanonmeter),
// which is about the same as Angle.
//
// ChordAngles are represented by the squared chord length, which can
// range from 0 to 4. Positive infinity represents an infinite squared length.
type ChordAngle float64

const (
	// NegativeChordAngle represents a chord angle smaller than the zero angle.
	// The only valid operations on a NegativeChordAngle are comparisons and
	// Angle conversions.
	NegativeChordAngle = ChordAngle(-1)

	// RightChordAngle represents a chord angle of 90 degrees (a "right angle").
	RightChordAngle = ChordAngle(2)

	// StraightChordAngle represents a chord angle of 180 degrees (a "straight angle").
	// This is the maximum finite chord angle.
	StraightChordAngle = ChordAngle(4)
)

var (
	dblEpsilon = math.Nextafter(1, 2) - 1
)

// ChordFromAngle returns a ChordAngle from the given Angle.
func ChordFromAngle(a Angle) ChordAngle {
	if a < 0 {
		return NegativeChordAngle
	}
	if a.isInf() {
		return InfChordAngle()
	}
	l := 2 * math.Sin(0.5*math.Min(math.Pi, a.Radians()))
	return ChordAngle(l * l)
}

// Angle converts this ChordAngle to an Angle.
func (c ChordAngle) Angle() Angle {
	if c < 0 {
		return -1 * Radian
	}
	if c.isInf() {
		return InfAngle()
	}
	return Angle(2 * math.Asin(0.5*math.Sqrt(float64(c))))
}

// InfChordAngle returns a chord angle larger than any finite chord angle.
// The only valid operations on an InfChordAngle are comparisons and Angle conversions.
func InfChordAngle() ChordAngle {
	return ChordAngle(math.Inf(1))
}

// isInf reports whether this ChordAngle is infinite.
func (c ChordAngle) isInf() bool {
	return math.IsInf(float64(c), 1)
}

// isSpecial reports whether this ChordAngle is one of the special cases.
func (c ChordAngle) isSpecial() bool {
	return c < 0 || c.isInf()
}

// isValid reports whether this ChordAngle is valid or not.
func (c ChordAngle) isValid() bool {
	return (c >= 0 && c <= 4) || c.isSpecial()
}

// MaxPointError returns the maximum error size for a ChordAngle constructed
// from 2 Points x and y, assuming that x and y are normalized to within the
// bounds guaranteed by s2.Point.Normalize. The error is defined with respect to
// the true distance after the points are projected to lie exactly on the sphere.
func (c ChordAngle) MaxPointError() float64 {
	// There is a relative error of (2.5*dblEpsilon) when computing the squared
	// distance, plus an absolute error of (16 * dblEpsilon**2) because the
	// lengths of the input points may differ from 1 by up to (2*dblEpsilon) each.
	return 2.5*dblEpsilon*float64(c) + 16*dblEpsilon*dblEpsilon
}

// MaxAngleError returns the maximum error for a ChordAngle constructed
// as an Angle distance.
func (c ChordAngle) MaxAngleError() float64 {
	return dblEpsilon * float64(c)
}

// BUG(roberts): The major differences from the C++ version are:
//   - no S2Point constructors
//   - no FromLength constructor
//   - no PlusError
//   - no trigonometric or arithmetic operators
