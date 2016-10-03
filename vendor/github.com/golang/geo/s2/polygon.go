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

// Polygon represents a sequence of zero or more loops; recall that the
// interior of a loop is defined to be its left-hand side (see Loop).
//
// When the polygon is initialized, the given loops are automatically converted
// into a canonical form consisting of "shells" and "holes". Shells and holes
// are both oriented CCW, and are nested hierarchically. The loops are
// reordered to correspond to a preorder traversal of the nesting hierarchy.
//
// Polygons may represent any region of the sphere with a polygonal boundary,
// including the entire sphere (known as the "full" polygon). The full polygon
// consists of a single full loop (see Loop), whereas the empty polygon has no
// loops at all.
//
// Use FullPolygon() to construct a full polygon. The zero value of Polygon is
// treated as the empty polygon.
//
// Polygons have the following restrictions:
//
//  - Loops may not cross, i.e. the boundary of a loop may not intersect
//    both the interior and exterior of any other loop.
//
//  - Loops may not share edges, i.e. if a loop contains an edge AB, then
//    no other loop may contain AB or BA.
//
//  - Loops may share vertices, however no vertex may appear twice in a
//    single loop (see Loop).
//
//  - No loop may be empty. The full loop may appear only in the full polygon.
type Polygon struct {
	loops []*Loop

	// loopDepths keeps track of how deep a given loop is in this polygon.
	// The depths tracked in this slice are kept in 1:1 lockstep with the
	// elements in the loops list.
	// Holes inside a polygon are stored as odd numbers, and shells are even.
	loopDepths []int

	// index is a spatial index of all the polygon loops.
	index ShapeIndex

	// hasHoles tracks if this polygon has at least one hole.
	hasHoles bool

	// numVertices keeps the running total of all of the vertices of the contained loops.
	numVertices int

	// bound is a conservative bound on all points contained by this loop.
	// If l.ContainsPoint(P), then l.bound.ContainsPoint(P).
	bound Rect

	// Since bound is not exact, it is possible that a loop A contains
	// another loop B whose bounds are slightly larger. subregionBound
	// has been expanded sufficiently to account for this error, i.e.
	// if A.Contains(B), then A.subregionBound.Contains(B.bound).
	subregionBound Rect
}

// PolygonFromLoops constructs a polygon from the given hierarchically nested
// loops. The polygon interior consists of the points contained by an odd
// number of loops. (Recall that a loop contains the set of points on its
// left-hand side.)
//
// This method figures out the loop nesting hierarchy and assigns every loop a
// depth. Shells have even depths, and holes have odd depths.
//
// NOTE: this function is NOT YET IMPLEMENTED for more than one loop and will
// panic if given a slice of length > 1.
func PolygonFromLoops(loops []*Loop) *Polygon {
	if len(loops) > 1 {
		panic("s2.PolygonFromLoops for multiple loops is not yet implemented")
	}
	return &Polygon{
		loops: loops,
		// TODO(roberts): This is explicitly set as depth of 0 for the one loop in
		// the polygon. When multiple loops are supported, fix this to set the depths.
		loopDepths:     []int{0},
		numVertices:    len(loops[0].Vertices()), // TODO(roberts): Once multi-loop is supported, fix this.
		bound:          EmptyRect(),
		subregionBound: EmptyRect(),
	}
}

// FullPolygon returns a special "full" polygon.
func FullPolygon() *Polygon {
	return &Polygon{
		loops: []*Loop{
			FullLoop(),
		},
		loopDepths:     []int{0},
		numVertices:    len(FullLoop().Vertices()),
		bound:          FullRect(),
		subregionBound: FullRect(),
	}
}

// IsEmpty reports whether this is the special "empty" polygon (consisting of no loops).
func (p *Polygon) IsEmpty() bool {
	return len(p.loops) == 0
}

// IsFull reports whether this is the special "full" polygon (consisting of a
// single loop that encompasses the entire sphere).
func (p *Polygon) IsFull() bool {
	return len(p.loops) == 1 && p.loops[0].IsFull()
}

// NumLoops returns the number of loops in this polygon.
func (p *Polygon) NumLoops() int {
	return len(p.loops)
}

// Loops returns the loops in this polygon.
func (p *Polygon) Loops() []*Loop {
	return p.loops
}

// Loop returns the loop at the given index. Note that during initialization,
// the given loops are reordered according to a preorder traversal of the loop
// nesting hierarchy. This implies that every loop is immediately followed by
// its descendants. This hierarchy can be traversed using the methods Parent,
// LastDescendant, and Loop.depth.
func (p *Polygon) Loop(k int) *Loop {
	return p.loops[k]
}

// Parent returns the index of the parent of loop k.
// If the loop does not have a parent, ok=false is returned.
func (p *Polygon) Parent(k int) (index int, ok bool) {
	// See where we are on the depth heirarchy.
	depth := p.loopDepths[k]
	if depth == 0 {
		return -1, false
	}

	// There may be several loops at the same nesting level as us that share a
	// parent loop with us. (Imagine a slice of swiss cheese, of which we are one loop.
	// we don't know how many may be next to us before we get back to our parent loop.)
	// Move up one position from us, and then begin traversing back through the set of loops
	// until we find the one that is our parent or we get to the top of the polygon.
	for k--; k >= 0 && p.loopDepths[k] <= depth; k-- {
	}
	return k, true
}

// LastDescendant returns the index of the last loop that is contained within loop k.
// If k is negative, it returns the last loop in the polygon.
// Note that loops are indexed according to a preorder traversal of the nesting
// hierarchy, so the immediate children of loop k can be found by iterating over
// the loops (k+1)..LastDescendant(k) and selecting those whose depth is equal
// to Loop(k).depth+1.
func (p *Polygon) LastDescendant(k int) int {
	if k < 0 {
		return len(p.loops) - 1
	}

	depth := p.loopDepths[k]

	// Find the next loop immediately past us in the set of loops, and then start
	// moving down the list until we either get to the end or find the next loop
	// that is higher up the heirarchy than we are.
	for k++; k < len(p.loops) && p.loopDepths[k] > depth; k++ {
	}
	return k - 1
}

// loopIsHole reports whether the given loop represents a hole in this polygon.
func (p *Polygon) loopIsHole(k int) bool {
	return p.loopDepths[k]&1 != 0
}

// loopSign returns -1 if this loop represents a hole in this polygon.
// Otherwise, it returns +1. This is used when computing the area of a polygon.
// (holes are subtracted from the total area).
func (p *Polygon) loopSign(k int) int {
	if p.loopIsHole(k) {
		return -1
	}
	return 1
}

// CapBound returns a bounding spherical cap.
func (p *Polygon) CapBound() Cap { return p.bound.CapBound() }

// RectBound returns a bounding latitude-longitude rectangle.
func (p *Polygon) RectBound() Rect { return p.bound }

// ContainsCell reports whether the polygon contains the given cell.
// TODO(roberts)
//func (p *Polygon) ContainsCell(c Cell) bool { ... }

// IntersectsCell reports whether the polygon intersects the given cell.
// TODO(roberts)
//func (p *Polygon) IntersectsCell(c Cell) bool { ... }
