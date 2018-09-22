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
	"fmt"
	"io"
)

// Polygon represents a sequence of zero or more loops; recall that the
// interior of a loop is defined to be its left-hand side (see Loop).
//
// When the polygon is initialized, the given loops are automatically converted
// into a canonical form consisting of "shells" and "holes". Shells and holes
// are both oriented CCW, and are nested hierarchically. The loops are
// reordered to correspond to a pre-order traversal of the nesting hierarchy.
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

	// index is a spatial index of all the polygon loops.
	index *ShapeIndex

	// hasHoles tracks if this polygon has at least one hole.
	hasHoles bool

	// numVertices keeps the running total of all of the vertices of the contained loops.
	numVertices int

	// numEdges tracks the total number of edges in all the loops in this polygon.
	numEdges int

	// bound is a conservative bound on all points contained by this loop.
	// If l.ContainsPoint(P), then l.bound.ContainsPoint(P).
	bound Rect

	// Since bound is not exact, it is possible that a loop A contains
	// another loop B whose bounds are slightly larger. subregionBound
	// has been expanded sufficiently to account for this error, i.e.
	// if A.Contains(B), then A.subregionBound.Contains(B.bound).
	subregionBound Rect

	// A slice where element i is the cumulative number of edges in the
	// preceding loops in the polygon. This field is used for polygons that
	// have a large number of loops, and may be empty for polygons with few loops.
	cumulativeEdges []int
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
		panic("PolygonFromLoops for multiple loops is not yet implemented")
	}

	p := &Polygon{
		loops:       loops,
		numVertices: len(loops[0].Vertices()), // TODO(roberts): Once multi-loop is supported, fix this.
		// TODO(roberts): Compute these bounds.
		bound:          loops[0].RectBound(),
		subregionBound: EmptyRect(),
	}

	const maxLinearSearchLoops = 12 // Based on benchmarks.
	if len(loops) > maxLinearSearchLoops {
		p.cumulativeEdges = make([]int, 0, len(loops))
	}

	for _, l := range loops {
		if p.cumulativeEdges != nil {
			p.cumulativeEdges = append(p.cumulativeEdges, p.numEdges)
		}
		p.numEdges += len(l.Vertices())
	}

	p.index = NewShapeIndex()
	p.index.Add(p)
	return p
}

// FullPolygon returns a special "full" polygon.
func FullPolygon() *Polygon {
	return &Polygon{
		loops: []*Loop{
			FullLoop(),
		},
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
// the given loops are reordered according to a pre-order traversal of the loop
// nesting hierarchy. This implies that every loop is immediately followed by
// its descendants. This hierarchy can be traversed using the methods Parent,
// LastDescendant, and Loop.depth.
func (p *Polygon) Loop(k int) *Loop {
	return p.loops[k]
}

// Parent returns the index of the parent of loop k.
// If the loop does not have a parent, ok=false is returned.
func (p *Polygon) Parent(k int) (index int, ok bool) {
	// See where we are on the depth hierarchy.
	depth := p.loops[k].depth
	if depth == 0 {
		return -1, false
	}

	// There may be several loops at the same nesting level as us that share a
	// parent loop with us. (Imagine a slice of swiss cheese, of which we are one loop.
	// we don't know how many may be next to us before we get back to our parent loop.)
	// Move up one position from us, and then begin traversing back through the set of loops
	// until we find the one that is our parent or we get to the top of the polygon.
	for k--; k >= 0 && p.loops[k].depth <= depth; k-- {
	}
	return k, true
}

// LastDescendant returns the index of the last loop that is contained within loop k.
// If k is negative, it returns the last loop in the polygon.
// Note that loops are indexed according to a pre-order traversal of the nesting
// hierarchy, so the immediate children of loop k can be found by iterating over
// the loops (k+1)..LastDescendant(k) and selecting those whose depth is equal
// to Loop(k).depth+1.
func (p *Polygon) LastDescendant(k int) int {
	if k < 0 {
		return len(p.loops) - 1
	}

	depth := p.loops[k].depth

	// Find the next loop immediately past us in the set of loops, and then start
	// moving down the list until we either get to the end or find the next loop
	// that is higher up the hierarchy than we are.
	for k++; k < len(p.loops) && p.loops[k].depth > depth; k++ {
	}
	return k - 1
}

// loopIsHole reports whether the given loop represents a hole in this polygon.
func (p *Polygon) loopIsHole(k int) bool {
	return p.loops[k].depth&1 != 0
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

// ContainsPoint reports whether the polygon contains the point.
func (p *Polygon) ContainsPoint(point Point) bool {
	// NOTE: A bounds check slows down this function by about 50%. It is
	// worthwhile only when it might allow us to delay building the index.
	if !p.index.IsFresh() && !p.bound.ContainsPoint(point) {
		return false
	}

	// For small polygons, and during initial construction, it is faster to just
	// check all the crossing.
	const maxBruteForceVertices = 32
	if p.numVertices < maxBruteForceVertices || p.index == nil {
		inside := false
		for _, l := range p.loops {
			// use loops bruteforce to avoid building the index on each loop.
			inside = inside != l.bruteForceContainsPoint(point)
		}
		return inside
	}

	// Otherwise, look up the ShapeIndex cell containing this point.
	it := p.index.Iterator()
	if !it.LocatePoint(point) {
		return false
	}

	return p.iteratorContainsPoint(it, point)
}

// ContainsCell reports whether the polygon contains the given cell.
func (p *Polygon) ContainsCell(cell Cell) bool {
	it := p.index.Iterator()
	relation := it.LocateCellID(cell.ID())

	// If "cell" is disjoint from all index cells, it is not contained.
	// Similarly, if "cell" is subdivided into one or more index cells then it
	// is not contained, since index cells are subdivided only if they (nearly)
	// intersect a sufficient number of edges.  (But note that if "cell" itself
	// is an index cell then it may be contained, since it could be a cell with
	// no edges in the loop interior.)
	if relation != Indexed {
		return false
	}

	// Otherwise check if any edges intersect "cell".
	if p.boundaryApproxIntersects(it, cell) {
		return false
	}

	// Otherwise check if the loop contains the center of "cell".
	return p.iteratorContainsPoint(it, cell.Center())
}

// IntersectsCell reports whether the polygon intersects the given cell.
func (p *Polygon) IntersectsCell(cell Cell) bool {
	it := p.index.Iterator()
	relation := it.LocateCellID(cell.ID())

	// If cell does not overlap any index cell, there is no intersection.
	if relation == Disjoint {
		return false
	}
	// If cell is subdivided into one or more index cells, there is an
	// intersection to within the S2ShapeIndex error bound (see Contains).
	if relation == Subdivided {
		return true
	}
	// If cell is an index cell, there is an intersection because index cells
	// are created only if they have at least one edge or they are entirely
	// contained by the loop.
	if it.CellID() == cell.id {
		return true
	}
	// Otherwise check if any edges intersect cell.
	if p.boundaryApproxIntersects(it, cell) {
		return true
	}
	// Otherwise check if the loop contains the center of cell.
	return p.iteratorContainsPoint(it, cell.Center())
}

// CellUnionBound computes a covering of the Polygon.
func (p *Polygon) CellUnionBound() []CellID {
	// TODO(roberts): Use ShapeIndexRegion when it's available.
	return p.CapBound().CellUnionBound()
}

// boundaryApproxIntersects reports whether the loop's boundary intersects cell.
// It may also return true when the loop boundary does not intersect cell but
// some edge comes within the worst-case error tolerance.
//
// This requires that it.Locate(cell) returned Indexed.
func (p *Polygon) boundaryApproxIntersects(it *ShapeIndexIterator, cell Cell) bool {
	aClipped := it.IndexCell().findByShapeID(0)

	// If there are no edges, there is no intersection.
	if len(aClipped.edges) == 0 {
		return false
	}

	// We can save some work if cell is the index cell itself.
	if it.CellID() == cell.ID() {
		return true
	}

	// Otherwise check whether any of the edges intersect cell.
	maxError := (faceClipErrorUVCoord + intersectsRectErrorUVDist)
	bound := cell.BoundUV().ExpandedByMargin(maxError)
	for _, e := range aClipped.edges {
		edge := p.index.Shape(0).Edge(e)
		v0, v1, ok := ClipToPaddedFace(edge.V0, edge.V1, cell.Face(), maxError)
		if ok && edgeIntersectsRect(v0, v1, bound) {
			return true
		}
	}

	return false
}

// iteratorContainsPoint reports whether the iterator that is positioned at the
// ShapeIndexCell that may contain p, contains the point p.
func (p *Polygon) iteratorContainsPoint(it *ShapeIndexIterator, point Point) bool {
	// Test containment by drawing a line segment from the cell center to the
	// given point and counting edge crossings.
	aClipped := it.IndexCell().findByShapeID(0)
	inside := aClipped.containsCenter

	if len(aClipped.edges) == 0 {
		return inside
	}

	// This block requires ShapeIndex.
	crosser := NewEdgeCrosser(it.Center(), point)
	shape := p.index.Shape(0)
	for _, e := range aClipped.edges {
		edge := shape.Edge(e)
		inside = inside != crosser.EdgeOrVertexCrossing(edge.V0, edge.V1)
	}

	return inside
}

// Shape Interface

// NumEdges returns the number of edges in this shape.
func (p *Polygon) NumEdges() int {
	return p.numEdges
}

// Edge returns endpoints for the given edge index.
func (p *Polygon) Edge(e int) Edge {
	var i int

	if len(p.cumulativeEdges) > 0 {
		for i = range p.cumulativeEdges {
			if i+1 >= len(p.cumulativeEdges) || e < p.cumulativeEdges[i+1] {
				e -= p.cumulativeEdges[i]
				break
			}
		}
	} else {
		// When the number of loops is small, use linear search. Most often
		// there is exactly one loop and the code below executes zero times.
		for i = 0; e >= len(p.Loop(i).vertices); i++ {
			e -= len(p.Loop(i).vertices)
		}
	}

	return Edge{p.Loop(i).OrientedVertex(e), p.Loop(i).OrientedVertex(e + 1)}
}

// HasInterior reports whether this Polygon has an interior.
func (p *Polygon) HasInterior() bool {
	return p.dimension() == polygonGeometry
}

// ContainsOrigin returns whether this shape contains the origin.
func (p *Polygon) ContainsOrigin() bool {
	containsOrigin := false
	for _, l := range p.loops {
		containsOrigin = containsOrigin != l.ContainsOrigin()
	}
	return containsOrigin
}

// NumChains reports the number of contiguous edge chains in the Polygon.
func (p *Polygon) NumChains() int {
	if p.IsFull() {
		return 0
	}

	return p.NumLoops()
}

// Chain returns the i-th edge Chain (loop) in the Shape.
func (p *Polygon) Chain(chainID int) Chain {
	if p.cumulativeEdges != nil {
		return Chain{p.cumulativeEdges[chainID], len(p.Loop(chainID).vertices)}
	}
	e := 0
	for j := 0; j < chainID; j++ {
		e += len(p.Loop(j).vertices)
	}
	return Chain{e, len(p.Loop(chainID).vertices)}
}

// ChainEdge returns the j-th edge of the i-th edge Chain (loop).
func (p *Polygon) ChainEdge(i, j int) Edge {
	return Edge{p.Loop(i).OrientedVertex(j), p.Loop(i).OrientedVertex(j + 1)}
}

// ChainPosition returns a pair (i, j) such that edgeID is the j-th edge
// of the i-th edge Chain.
func (p *Polygon) ChainPosition(edgeID int) ChainPosition {
	var i int

	if len(p.cumulativeEdges) > 0 {
		for i = range p.cumulativeEdges {
			if i+1 >= len(p.cumulativeEdges) || edgeID < p.cumulativeEdges[i+1] {
				edgeID -= p.cumulativeEdges[i]
				break
			}
		}
	} else {
		// When the number of loops is small, use linear search. Most often
		// there is exactly one loop and the code below executes zero times.
		for i = 0; edgeID >= len(p.Loop(i).vertices); i++ {
			edgeID -= len(p.Loop(i).vertices)
		}
	}
	// TODO(roberts): unify this and Edge since they are mostly identical.
	return ChainPosition{i, edgeID}
}

// dimension returns the dimension of the geometry represented by this Polygon.
func (p *Polygon) dimension() dimension { return polygonGeometry }

// Encode encodes the Polygon
func (p *Polygon) Encode(w io.Writer) error {
	e := &encoder{w: w}
	p.encode(e)
	return e.err
}

// encode only supports lossless encoding and not compressed format.
func (p *Polygon) encode(e *encoder) {
	if p.numVertices == 0 {
		//p.encodeCompressed(e, nil, maxLevel)
		e.err = fmt.Errorf("compressed encoding not yet implemented")
		return
	}

	// TODO(roberts): C++ computes a heurstic at encoding time to decide between
	// using compressed and lossless format. Add that calculation once XYZFaceSiTi
	// type is implemented.

	p.encodeLossless(e)
}

// encodeLossless encodes the polygon's Points as float64s.
func (p *Polygon) encodeLossless(e *encoder) {
	e.writeInt8(encodingVersion)
	e.writeBool(true) // a legacy c++ value. must be true.
	e.writeBool(p.hasHoles)
	e.writeUint32(uint32(len(p.loops)))

	for _, l := range p.loops {
		l.encode(e)
	}

	// Encode the bound.
	p.bound.encode(e)
}

// TODO(roberts): Differences from C++
// InitNestedFromLoops
// InitFromLoop
// InitOrientedFromLoops
// IsValid
// Area
// Centroid
// SnapLevel
// DistanceToPoint
// DistanceToBoundary
// Project
// ProjectToBoundary
// Contains/ApproxContains/Intersects/ApproxDisjoint for Polygons
// InitTo{Intersection/ApproxIntersection/Union/ApproxUnion/Diff/ApproxDiff}
// InitToSimplified
// InitToSnapped
// IntersectWithPolyline
// ApproxIntersectWithPolyline
// SubtractFromPolyline
// ApproxSubtractFromPolyline
// DestructiveUnion
// DestructiveApproxUnion
// InitToCellUnionBorder
// IsNormalized
// Equals/BoundaryEquals/BoundaryApproxEquals/BoundaryNear Polygons
// BreakEdgesAndAddToBuilder
// clearLoops
// findLoopNestingError
// initLoops
// initToSimplifiedInternal
// internalClipPolyline
// compareBoundary
// containsBoundary
// excludesBoundary
// containsNonCrossingBoundary
// excludesNonCrossingShells
// anyLoopContains(Loop)
// anyLoopIntersects(Loop)
// clipBoundary
