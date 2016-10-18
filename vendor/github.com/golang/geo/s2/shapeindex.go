/*
Copyright 2016 Google Inc. All rights reserved.

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
	"github.com/golang/geo/r2"
)

// Shape defines an interface for any s2 type that needs to be indexable.
type Shape interface {
	// NumEdges returns the number of edges in this shape.
	NumEdges() int

	// Edge returns endpoints for the given edge index.
	Edge(i int) (a, b Point)

	// HasInterior returns true if this shape has an interior.
	// i.e. the Shape consists of one or more closed non-intersecting loops.
	HasInterior() bool

	// ContainsOrigin returns true if this shape contains s2.Origin.
	// Shapes that do not have an interior will return false.
	ContainsOrigin() bool
}

// A minimal check for types that should satisfy the Shape interface.
var (
	_ Shape = Loop{}
)

// CellRelation describes the possible relationships between a target cell
// and the cells of the ShapeIndex. If the target is an index cell or is
// contained by an index cell, it is Indexed. If the target is subdivided
// into one or more index cells, it is Subdivided. Otherwise it is Disjoint.
type CellRelation int

// The possible CellRelations for a ShapeIndex.
const (
	Indexed CellRelation = iota
	Subdivided
	Disjoint
)

var (
	// cellPadding defines the total error when clipping an edge which comes
	// from two sources:
	// (1) Clipping the original spherical edge to a cube face (the face edge).
	//     The maximum error in this step is faceClipErrorUVCoord.
	// (2) Clipping the face edge to the u- or v-coordinate of a cell boundary.
	//     The maximum error in this step is edgeClipErrorUVCoord.
	// Finally, since we encounter the same errors when clipping query edges, we
	// double the total error so that we only need to pad edges during indexing
	// and not at query time.
	cellPadding = 2.0 * (faceClipErrorUVCoord + edgeClipErrorUVCoord)
)

type clippedShape struct {
	// shapeID is the index of the shape this clipped shape is a part of.
	shapeID int32

	// containsCenter indicates if the center of the CellID this shape has been
	// clipped to falls inside this shape. This is false for shapes that do not
	// have an interior.
	containsCenter bool

	// edges is the ordered set of ShapeIndex original edge ids. Edges
	// are stored in increasing order of edge id.
	edges []int
}

// init initializes this shape for the given shapeID and number of expected edges.
func newClippedShape(id int32, numEdges int) *clippedShape {
	return &clippedShape{
		shapeID: id,
		edges:   make([]int, numEdges),
	}
}

// shapeIndexCell stores the index contents for a particular CellID.
type shapeIndexCell struct {
	shapes []*clippedShape
}

// add adds the given clipped shape to this index cell.
func (s *shapeIndexCell) add(c *clippedShape) {
	s.shapes = append(s.shapes, c)
}

// findByID returns the clipped shape that contains the given shapeID,
// or nil if none of the clipped shapes contain it.
func (s *shapeIndexCell) findByID(shapeID int32) *clippedShape {
	// Linear search is fine because the number of shapes per cell is typically
	// very small (most often 1), and is large only for pathological inputs
	// (e.g. very deeply nested loops).
	for _, clipped := range s.shapes {
		if clipped.shapeID == shapeID {
			return clipped
		}
	}
	return nil
}

// faceEdge and clippedEdge store temporary edge data while the index is being
// updated.
//
// While it would be possible to combine all the edge information into one
// structure, there are two good reasons for separating it:
//
//  - Memory usage. Separating the two means that we only need to
//    store one copy of the per-face data no matter how many times an edge is
//    subdivided, and it also lets us delay computing bounding boxes until
//    they are needed for processing each face (when the dataset spans
//    multiple faces).
//
//  - Performance. UpdateEdges is significantly faster on large polygons when
//    the data is separated, because it often only needs to access the data in
//    clippedEdge and this data is cached more successfully.

// faceEdge represents an edge that has been projected onto a given face,
type faceEdge struct {
	shapeID     int32    // The ID of shape that this edge belongs to
	edgeID      int      // Edge ID within that shape
	maxLevel    int      // Not desirable to subdivide this edge beyond this level
	hasInterior bool     // Belongs to a shape that has an interior
	a, b        r2.Point // The edge endpoints, clipped to a given face
	va, vb      Point    // The original Loop vertices of this edge.
}

// clippedEdge represents the portion of that edge that has been clipped to a given Cell.
type clippedEdge struct {
	faceEdge *faceEdge // The original unclipped edge
	bound    r2.Rect   // Bounding box for the clipped portion
}

// ShapeIndex indexes a set of Shapes, where a Shape is some collection of
// edges. A shape can be as simple as a single edge, or as complex as a set of loops.
// For Shapes that have interiors, the index makes it very fast to determine which
// Shape(s) contain a given point or region.
type ShapeIndex struct {
	// shapes maps all shapes to their index.
	shapes map[Shape]int32

	maxEdgesPerCell int

	// nextID tracks the next ID to hand out. IDs are not reused when shapes
	// are removed from the index.
	nextID int32
}

// NewShapeIndex creates a new ShapeIndex.
func NewShapeIndex() *ShapeIndex {
	return &ShapeIndex{
		maxEdgesPerCell: 10,
		shapes:          make(map[Shape]int32),
	}
}

// Add adds the given shape to the index and assign an ID to it.
func (s *ShapeIndex) Add(shape Shape) {
	s.shapes[shape] = s.nextID
	s.nextID++
}

// Remove removes the given shape from the index.
func (s *ShapeIndex) Remove(shape Shape) {
	delete(s.shapes, shape)
}

// Len reports the number of Shapes in this index.
func (s *ShapeIndex) Len() int {
	return len(s.shapes)
}

// Reset clears the contents of the index and resets it to its original state.
func (s *ShapeIndex) Reset() {
	s.shapes = make(map[Shape]int32)
	s.nextID = 0
}

// NumEdges returns the number of edges in this index.
func (s *ShapeIndex) NumEdges() int {
	numEdges := 0
	for shape := range s.shapes {
		numEdges += shape.NumEdges()
	}
	return numEdges
}
