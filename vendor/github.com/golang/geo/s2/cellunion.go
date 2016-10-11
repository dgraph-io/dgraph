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
	"sort"
)

// A CellUnion is a collection of CellIDs.
//
// It is normalized if it is sorted, and does not contain redundancy.
// Specifically, it may not contain the same CellID twice, nor a CellID that is contained by another,
// nor the four sibling CellIDs that are children of a single higher level CellID.
type CellUnion []CellID

// Normalize normalizes the CellUnion.
func (cu *CellUnion) Normalize() {
	sort.Sort(byID(*cu))

	output := make([]CellID, 0, len(*cu)) // the list of accepted cells
	// Loop invariant: output is a sorted list of cells with no redundancy.
	for _, ci := range *cu {
		// The first two passes here either ignore this new candidate,
		// or remove previously accepted cells that are covered by this candidate.

		// Ignore this cell if it is contained by the previous one.
		// We only need to check the last accepted cell. The ordering of the
		// cells implies containment (but not the converse), and output has no redundancy,
		// so if this candidate is not contained by the last accepted cell
		// then it cannot be contained by any previously accepted cell.
		if len(output) > 0 && output[len(output)-1].Contains(ci) {
			continue
		}

		// Discard any previously accepted cells contained by this one.
		// This could be any contiguous trailing subsequence, but it can't be
		// a discontiguous subsequence because of the containment property of
		// sorted S2 cells mentioned above.
		j := len(output) - 1 // last index to keep
		for j >= 0 {
			if !ci.Contains(output[j]) {
				break
			}
			j--
		}
		output = output[:j+1]

		// See if the last three cells plus this one can be collapsed.
		// We loop because collapsing three accepted cells and adding a higher level cell
		// could cascade into previously accepted cells.
		for len(output) >= 3 {
			fin := output[len(output)-3:]

			// fast XOR test; a necessary but not sufficient condition
			if fin[0]^fin[1]^fin[2]^ci != 0 {
				break
			}

			// more expensive test; exact.
			// Compute the two bit mask for the encoded child position,
			// then see if they all agree.
			mask := CellID(ci.lsb() << 1)
			mask = ^(mask + mask<<1)
			should := ci & mask
			if (fin[0]&mask != should) || (fin[1]&mask != should) || (fin[2]&mask != should) || ci.isFace() {
				break
			}

			output = output[:len(output)-3]
			ci = ci.immediateParent() // checked !ci.isFace above
		}
		output = append(output, ci)
	}
	*cu = output
}

// IntersectsCellID reports whether this cell union intersects the given cell ID.
//
// This method assumes that the CellUnion has been normalized.
func (cu *CellUnion) IntersectsCellID(id CellID) bool {
	// Find index of array item that occurs directly after our probe cell:
	i := sort.Search(len(*cu), func(i int) bool { return id < (*cu)[i] })

	if i != len(*cu) && (*cu)[i].RangeMin() <= id.RangeMax() {
		return true
	}
	return i != 0 && (*cu)[i-1].RangeMax() >= id.RangeMin()
}

// ContainsCellID reports whether the cell union contains the given cell ID.
// Containment is defined with respect to regions, e.g. a cell contains its 4 children.
//
// This method assumes that the CellUnion has been normalized.
func (cu *CellUnion) ContainsCellID(id CellID) bool {
	// Find index of array item that occurs directly after our probe cell:
	i := sort.Search(len(*cu), func(i int) bool { return id < (*cu)[i] })

	if i != len(*cu) && (*cu)[i].RangeMin() <= id {
		return true
	}
	return i != 0 && (*cu)[i-1].RangeMax() >= id
}

type byID []CellID

func (cu byID) Len() int           { return len(cu) }
func (cu byID) Less(i, j int) bool { return cu[i] < cu[j] }
func (cu byID) Swap(i, j int)      { cu[i], cu[j] = cu[j], cu[i] }

// Denormalize replaces this CellUnion with an expanded version of the
// CellUnion where any cell whose level is less than minLevel or where
// (level - minLevel) is not a multiple of levelMod is replaced by its
// children, until either both of these conditions are satisfied or the
// maximum level is reached.
func (cu *CellUnion) Denormalize(minLevel, levelMod int) {
	var denorm CellUnion
	for _, id := range *cu {
		level := id.Level()
		newLevel := level
		if newLevel < minLevel {
			newLevel = minLevel
		}
		if levelMod > 1 {
			newLevel += (maxLevel - (newLevel - minLevel)) % levelMod
			if newLevel > maxLevel {
				newLevel = maxLevel
			}
		}
		if newLevel == level {
			denorm = append(denorm, id)
		} else {
			end := id.ChildEndAtLevel(newLevel)
			for ci := id.ChildBeginAtLevel(newLevel); ci != end; ci = ci.Next() {
				denorm = append(denorm, ci)
			}
		}
	}
	*cu = denorm
}

// RectBound returns a Rect that bounds this entity.
func (cu *CellUnion) RectBound() Rect {
	bound := EmptyRect()
	for _, c := range *cu {
		bound = bound.Union(CellFromCellID(c).RectBound())
	}
	return bound
}

// CapBound returns a Cap that bounds this entity.
func (cu *CellUnion) CapBound() Cap {
	if len(*cu) == 0 {
		return EmptyCap()
	}

	// Compute the approximate centroid of the region. This won't produce the
	// bounding cap of minimal area, but it should be close enough.
	var centroid Point

	for _, ci := range *cu {
		area := AvgAreaMetric.Value(ci.Level())
		centroid = Point{centroid.Add(ci.Point().Mul(area))}
	}

	if zero := (Point{}); centroid == zero {
		centroid = PointFromCoords(1, 0, 0)
	} else {
		centroid = Point{centroid.Normalize()}
	}

	// Use the centroid as the cap axis, and expand the cap angle so that it
	// contains the bounding caps of all the individual cells.  Note that it is
	// *not* sufficient to just bound all the cell vertices because the bounding
	// cap may be concave (i.e. cover more than one hemisphere).
	c := CapFromPoint(centroid)
	for _, ci := range *cu {
		c = c.AddCap(CellFromCellID(ci).CapBound())
	}

	return c
}

// ContainsCell reports whether this cell union contains the given cell.
func (cu *CellUnion) ContainsCell(c Cell) bool {
	return cu.ContainsCellID(c.id)
}

// IntersectsCell reports whether this cell union intersects the given cell.
func (cu *CellUnion) IntersectsCell(c Cell) bool {
	return cu.IntersectsCellID(c.id)
}

// LeafCellsCovered reports the number of leaf cells covered by this cell union.
// This will be no more than 6*2^60 for the whole sphere.
func (cu *CellUnion) LeafCellsCovered() int64 {
	var numLeaves int64
	for _, c := range *cu {
		numLeaves += 1 << uint64((maxLevel-int64(c.Level()))<<1)
	}
	return numLeaves
}

// BUG: Differences from C++:
//  Contains(CellUnion)/Intersects(CellUnion)
//  Union(CellUnion)/Intersection(CellUnion)/Difference(CellUnion)
//  Expand
//  IntersectsCellID
//  CapBound
//  ContainsPoint
//  AverageArea/ApproxArea/ExactArea
