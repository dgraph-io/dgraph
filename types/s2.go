/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/geo/s2"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

// Added functionality missing in s2

// Make s2.Loop implement s2.Region
type loopRegion struct {
	*s2.Loop
}

func (l loopRegion) ContainsCell(c s2.Cell) bool {
	// Quick check if the cell is in the bounding box
	if !l.RectBound().ContainsCell(c) {
		return false
	}

	// Quick check to see if the first vertex is in the loop.
	if !l.ContainsPoint(c.Vertex(0)) {
		return false
	}

	// At this stage one vertex is in the loop, now we check that the edges of the cell do not cross
	// the loop.
	return !l.edgesCross(c)
}

// returns true if the edges of the cell cross the edges of the loop
func (l loopRegion) edgesCross(c s2.Cell) bool {
	return l.edgesCrossPoints([]s2.Point{c.Vertex(0), c.Vertex(1), c.Vertex(2), c.Vertex(3)})
}

func (l loopRegion) edgesCrossPoints(pts []s2.Point) bool {
	n := len(pts)
	for i := 0; i < n; i++ {
		crosser := s2.NewChainEdgeCrosser(pts[i], pts[(i+1)%n], l.Vertex(0))
		for i := 1; i <= l.NumEdges(); i++ { // add vertex 0 twice as it is a closed loop
			if crosser.EdgeOrVertexChainCrossing(l.Vertex(i)) {
				return true
			}
		}
	}
	return false
}

func (l loopRegion) IntersectsCell(c s2.Cell) bool {
	// Quick check if the cell intersects the bounding box
	if !l.RectBound().IntersectsCell(c) {
		return false
	}

	// Quick check to see if the first vertex is in the loop.
	if l.ContainsPoint(c.Vertex(0)) {
		return true
	}

	// At this stage the one vertex is outside the loop. We check if any of the edges of the cell
	// cross the loop.
	if l.edgesCross(c) {
		return true
	}

	// At this stage we know that there is one point of the cell outside the loop and the boundaries
	// do not interesect. The only way for the cell to intersect with the loop is if it contains the
	// the loop. We test this by checking if an arbitrary vertex of the loop is inside the cell.
	if c.RectBound().Contains(l.RectBound()) {
		return c.ContainsPoint(l.Vertex(0))
	}
	return false
}

func (l loopRegion) intersects(loop *s2.Loop) bool {
	// Quick check if the bounding boxes intersect
	if !l.RectBound().Intersects(loop.RectBound()) {
		return false
	}

	// Quick check to see if the first vertex is in the loop.
	if l.ContainsPoint(loop.Vertex(0)) {
		return true
	}

	// At this stage the one vertex is outside the loop. We check if any of the edges of the cell
	// cross the loop.
	if l.edgesCrossPoints(loop.Vertices()) {
		return true
	}

	// At this stage we know that there is one point of the loop is outside us and the boundaries do
	// not interesect. The only way for the loops to intersect is if it contains us.  We test this
	// by checking if an arbitrary vertex is inside the loop.
	if loop.RectBound().Contains(l.RectBound()) {
		return loop.ContainsPoint(l.Vertex(0))
	}
	return false
}

// Intersects returns true if the two loops intersect.
func Intersects(l1 *s2.Loop, l2 *s2.Loop) bool {
	if l2.NumEdges() > l1.NumEdges() {
		// Use the larger loop for edge indexing.
		return loopRegion{l2}.intersects(l1)
	}
	return loopRegion{l1}.intersects(l2)
}

func ConvertToGeoJson(str string) (geom.T, error) {
	s := strings.Replace(str, " ", "", -1)
	if len(s) < 5 {
		return nil, x.Errorf("Invalid coordinates")
	}
	var g geojson.Geometry
	var m json.RawMessage
	var err error

	if s[0] == '[' && s[1] != '[' {
		g.Type = "Point"
		err = m.UnmarshalJSON([]byte(s))
		if err != nil {
			return nil, x.Errorf("Invalid coordinates")
		}
		g.Coordinates = &m
	} else if s[0] == '[' && s[1] == '[' {
		g.Type = "Polygon"
		err = m.UnmarshalJSON([]byte("[" + s + "]"))
		if err != nil {
			return nil, x.Errorf("Invalid coordinates")
		}
		g.Coordinates = &m
		g1, err := g.Decode()
		if err != nil {
			return nil, err
		}
		coords := g1.(*geom.Polygon).Coords()
		if !reflect.DeepEqual(coords[0][0], coords[0][len(coords[0])-1]) {
			return nil, x.Errorf("Last coord not same as first")
		}
	} else {
		return nil, x.Errorf("invalid coordinates")
	}
	return g.Decode()
}
