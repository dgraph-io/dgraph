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
	"github.com/golang/geo/r1"
	"github.com/golang/geo/r2"
)

// PaddedCell represents a Cell whose (u,v)-range has been expanded on
// all sides by a given amount of "padding". Unlike Cell, its methods and
// representation are optimized for clipping edges against Cell boundaries
// to determine which cells are intersected by a given set of edges.
type PaddedCell struct {
	id          CellID
	padding     float64
	bound       r2.Rect
	middle      r2.Rect // A rect in (u, v)-space that belongs to all four children.
	iLo, jLo    int     // Minimum (i,j)-coordinates of this cell before padding
	orientation int     // Hilbert curve orientation of this cell.
	level       int
}

// PaddedCellFromCellID constructs a padded cell with the given padding.
func PaddedCellFromCellID(id CellID, padding float64) *PaddedCell {
	p := &PaddedCell{
		id:      id,
		padding: padding,
		middle:  r2.EmptyRect(),
	}

	// Fast path for constructing a top-level face (the most common case).
	if id.isFace() {
		limit := padding + 1
		p.bound = r2.Rect{r1.Interval{-limit, limit}, r1.Interval{-limit, limit}}
		p.middle = r2.Rect{r1.Interval{-padding, padding}, r1.Interval{-padding, padding}}
		p.orientation = id.Face() & 1
		return p
	}

	_, p.iLo, p.jLo, p.orientation = id.faceIJOrientation()
	p.level = id.Level()
	p.bound = ijLevelToBoundUV(p.iLo, p.jLo, p.level).ExpandedByMargin(padding)
	ijSize := sizeIJ(p.level)
	p.iLo &= -ijSize
	p.jLo &= -ijSize

	return p
}

// CellID returns the CellID this padded cell represents.
func (p PaddedCell) CellID() CellID {
	return p.id
}

// Padding returns the amount of padding on this cell.
func (p PaddedCell) Padding() float64 {
	return p.padding
}

// Level returns the level this cell is at.
func (p PaddedCell) Level() int {
	return p.level
}

// Center returns the center of this cell.
func (p PaddedCell) Center() Point {
	ijSize := sizeIJ(p.level)
	si := uint64(2*p.iLo + ijSize)
	ti := uint64(2*p.jLo + ijSize)
	return Point{faceSiTiToXYZ(p.id.Face(), si, ti).Normalize()}
}

// Middle returns the rectangle in the middle of this cell that belongs to
// all four of its children in (u,v)-space.
func (p *PaddedCell) Middle() r2.Rect {
	// We compute this field lazily because it is not needed the majority of the
	// time (i.e., for cells where the recursion terminates).
	if p.middle.IsEmpty() {
		ijSize := sizeIJ(p.level)
		u := stToUV(siTiToST(uint64(2*p.iLo + ijSize)))
		v := stToUV(siTiToST(uint64(2*p.jLo + ijSize)))
		p.middle = r2.Rect{
			r1.Interval{u - p.padding, u + p.padding},
			r1.Interval{v - p.padding, v + p.padding},
		}
	}
	return p.middle
}

// Bound returns the bounds for this cell in (u,v)-space including padding.
func (p PaddedCell) Bound() r2.Rect {
	return p.bound
}

// ChildIJ returns the (i,j) coordinates for the child cell at the given traversal
// position. The traversal position corresponds to the order in which child
// cells are visited by the Hilbert curve.
func (p PaddedCell) ChildIJ(pos int) (i, j int) {
	ij := posToIJ[p.orientation][pos]
	return ij >> 1, ij & 1
}

// TODO(roberts): The major differences from the C++ version are:
// PaddedCellFromParentIJ
// ShrinkToFit
// EntryVertex
// ExitVertex
