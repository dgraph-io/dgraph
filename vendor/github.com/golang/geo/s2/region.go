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

// A Region represents a two-dimensional region on the unit sphere.
//
// The purpose of this interface is to allow complex regions to be
// approximated as simpler regions. The interface is restricted to methods
// that are useful for computing approximations.
type Region interface {
	// CapBound returns a bounding spherical cap. This is not guaranteed to be exact.
	CapBound() Cap

	// RectBound returns a bounding latitude-longitude rectangle that contains
	// the region. The bounds are not guaranteed to be tight.
	RectBound() Rect

	// ContainsCell reports whether the region completely contains the given region.
	// It returns false if containment could not be determined.
	ContainsCell(c Cell) bool

	// IntersectsCell reports whether the region intersects the given cell or
	// if intersection could not be determined. It returns false if the region
	// does not intersect.
	IntersectsCell(c Cell) bool
}

// Enforce interface satisfaction.
var (
	_ Region = Cap{}
	_ Region = Cell{}
	_ Region = (*CellUnion)(nil)
	//_ Region = (*Polygon)(nil)
	_ Region = Rect{}
)
