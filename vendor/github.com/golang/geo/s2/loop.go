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
	"github.com/golang/geo/r3"
	"github.com/golang/geo/s1"
)

// Loop represents a simple spherical polygon. It consists of a sequence
// of vertices where the first vertex is implicitly connected to the
// last. All loops are defined to have a CCW orientation, i.e. the interior of
// the loop is on the left side of the edges. This implies that a clockwise
// loop enclosing a small area is interpreted to be a CCW loop enclosing a
// very large area.
//
// Loops are not allowed to have any duplicate vertices (whether adjacent or
// not), and non-adjacent edges are not allowed to intersect. Loops must have
// at least 3 vertices (except for the "empty" and "full" loops discussed
// below).
//
// There are two special loops: the "empty" loop contains no points and the
// "full" loop contains all points. These loops do not have any edges, but to
// preserve the invariant that every loop can be represented as a vertex
// chain, they are defined as having exactly one vertex each (see EmptyLoop
// and FullLoop).
type Loop struct {
	vertices []Point

	// originInside keeps a precomputed value whether this loop contains the origin
	// versus computing from the set of vertices every time.
	originInside bool

	// bound is a conservative bound on all points contained by this loop.
	// If l.ContainsPoint(P), then l.bound.ContainsPoint(P).
	bound Rect

	// Since "bound" is not exact, it is possible that a loop A contains
	// another loop B whose bounds are slightly larger. subregionBound
	// has been expanded sufficiently to account for this error, i.e.
	// if A.Contains(B), then A.subregionBound.Contains(B.bound).
	subregionBound Rect
}

// LoopFromPoints constructs a loop from the given points.
func LoopFromPoints(pts []Point) *Loop {
	l := &Loop{
		vertices: pts,
	}

	l.initOriginAndBound()
	return l
}

// LoopFromCell constructs a loop corresponding to the given cell.
//
// Note that the loop and cell *do not* contain exactly the same set of
// points, because Loop and Cell have slightly different definitions of
// point containment. For example, a Cell vertex is contained by all
// four neighboring Cells, but it is contained by exactly one of four
// Loops constructed from those cells. As another example, the cell
// coverings of cell and LoopFromCell(cell) will be different, because the
// loop contains points on its boundary that actually belong to other cells
// (i.e., the covering will include a layer of neighboring cells).
func LoopFromCell(c Cell) *Loop {
	l := &Loop{
		vertices: []Point{
			c.Vertex(0),
			c.Vertex(1),
			c.Vertex(2),
			c.Vertex(3),
		},
	}

	l.initOriginAndBound()
	return l
}

// EmptyLoop returns a special "empty" loop.
func EmptyLoop() *Loop {
	return LoopFromPoints([]Point{{r3.Vector{X: 0, Y: 0, Z: 1}}})
}

// FullLoop returns a special "full" loop.
func FullLoop() *Loop {
	return LoopFromPoints([]Point{{r3.Vector{X: 0, Y: 0, Z: -1}}})
}

// initOriginAndBound sets the origin containment for the given point and then calls
// the initialization for the bounds objects and the internal index.
func (l *Loop) initOriginAndBound() {
	if len(l.vertices) < 3 {
		// Check for the special "empty" and "full" loops (which have one vertex).
		if !l.isEmptyOrFull() {
			l.originInside = false
			return
		}

		// This is the special empty or full loop, so the origin depends on if
		// the vertex is in the southern hemisphere or not.
		l.originInside = l.vertices[0].Z < 0
	} else {
		// Point containment testing is done by counting edge crossings starting
		// at a fixed point on the sphere (OriginPoint). We need to know whether
		// the reference point (OriginPoint) is inside or outside the loop before
		// we can construct the ShapeIndex. We do this by first guessing that
		// it is outside, and then seeing whether we get the correct containment
		// result for vertex 1. If the result is incorrect, the origin must be
		// inside the loop.
		//
		// A loop with consecutive vertices A,B,C contains vertex B if and only if
		// the fixed vector R = B.Ortho is contained by the wedge ABC. The
		// wedge is closed at A and open at C, i.e. the point B is inside the loop
		// if A = R but not if C = R. This convention is required for compatibility
		// with VertexCrossing. (Note that we can't use OriginPoint
		// as the fixed vector because of the possibility that B == OriginPoint.)
		l.originInside = false
		v1Inside := OrderedCCW(Point{l.vertices[1].Ortho()}, l.vertices[0], l.vertices[2], l.vertices[1])
		if v1Inside != l.ContainsPoint(l.vertices[1]) {
			l.originInside = true
		}
	}

	// We *must* call initBound before initIndex, because initBound calls
	// ContainsPoint(s2.Point), and ContainsPoint(s2.Point) does a bounds check whenever the
	// index is not fresh (i.e., the loop has been added to the index but the
	// index has not been updated yet).
	l.initBound()

	// TODO(roberts): Depends on s2shapeindex being implemented.
	// l.initIndex()
}

// initBound sets up the approximate bounding Rects for this loop.
func (l *Loop) initBound() {
	// Check for the special "empty" and "full" loops.
	if l.isEmptyOrFull() {
		if l.IsEmpty() {
			l.bound = EmptyRect()
		} else {
			l.bound = FullRect()
		}
		l.subregionBound = l.bound
		return
	}

	// The bounding rectangle of a loop is not necessarily the same as the
	// bounding rectangle of its vertices. First, the maximal latitude may be
	// attained along the interior of an edge. Second, the loop may wrap
	// entirely around the sphere (e.g. a loop that defines two revolutions of a
	// candy-cane stripe). Third, the loop may include one or both poles.
	// Note that a small clockwise loop near the equator contains both poles.
	bounder := NewRectBounder()
	for i := 0; i <= len(l.vertices); i++ { // add vertex 0 twice
		bounder.AddPoint(l.Vertex(i))
	}
	b := bounder.RectBound()

	if l.ContainsPoint(Point{r3.Vector{0, 0, 1}}) {
		b = Rect{r1.Interval{b.Lat.Lo, math.Pi / 2}, s1.FullInterval()}
	}
	// If a loop contains the south pole, then either it wraps entirely
	// around the sphere (full longitude range), or it also contains the
	// north pole in which case b.Lng.IsFull() due to the test above.
	// Either way, we only need to do the south pole containment test if
	// b.Lng.IsFull().
	if b.Lng.IsFull() && l.ContainsPoint(Point{r3.Vector{0, 0, -1}}) {
		b.Lat.Lo = -math.Pi / 2
	}
	l.bound = b
	l.subregionBound = ExpandForSubregions(l.bound)
}

// ContainsOrigin reports true if this loop contains s2.OriginPoint().
func (l Loop) ContainsOrigin() bool {
	return l.originInside
}

// HasInterior returns true because all loops have an interior.
func (l Loop) HasInterior() bool {
	return true
}

// NumEdges returns the number of edges in this shape.
func (l Loop) NumEdges() int {
	if l.isEmptyOrFull() {
		return 0
	}
	return len(l.vertices)
}

// Edge returns the endpoints for the given edge index.
func (l Loop) Edge(i int) (a, b Point) {
	return l.Vertex(i), l.Vertex(i + 1)
}

// IsEmpty reports true if this is the special "empty" loop that contains no points.
func (l Loop) IsEmpty() bool {
	return l.isEmptyOrFull() && !l.ContainsOrigin()
}

// IsFull reports true if this is the special "full" loop that contains all points.
func (l Loop) IsFull() bool {
	return l.isEmptyOrFull() && l.ContainsOrigin()
}

// isEmptyOrFull reports true if this loop is either the "empty" or "full" special loops.
func (l Loop) isEmptyOrFull() bool {
	return len(l.vertices) == 1
}

// RectBound returns a tight bounding rectangle. If the loop contains the point,
// the bound also contains it.
func (l Loop) RectBound() Rect {
	return l.bound
}

// CapBound returns a bounding cap that may have more padding than the corresponding
// RectBound. The bound is conservative such that if the loop contains a point P,
// the bound also contains it.
func (l Loop) CapBound() Cap {
	return l.bound.CapBound()
}

// Vertex returns the vertex for the given index. For convenience, the vertex indices
// wrap automatically for methods that do index math such as Edge.
// i.e., Vertex(NumEdges() + n) is the same as Vertex(n).
func (l Loop) Vertex(i int) Point {
	return l.vertices[i%len(l.vertices)]
}

// Vertices returns the vertices in the loop.
func (l Loop) Vertices() []Point {
	return l.vertices
}

// ContainsPoint returns true if the loop contains the point.
func (l Loop) ContainsPoint(p Point) bool {
	// TODO(sbeckman): Move to bruteForceContains and update with ShapeIndex when available.
	// Empty and full loops don't need a special case, but invalid loops with
	// zero vertices do, so we might as well handle them all at once.
	if len(l.vertices) < 3 {
		return l.originInside
	}

	origin := OriginPoint()
	inside := l.originInside
	crosser := NewChainEdgeCrosser(origin, p, l.Vertex(0))
	for i := 1; i <= len(l.vertices); i++ { // add vertex 0 twice
		inside = inside != crosser.EdgeOrVertexChainCrossing(l.Vertex(i))
	}
	return inside
}

// BUG(): The major differences from the C++ version is pretty much everything.
