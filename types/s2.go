/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
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
	"fmt"

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/geo/s2"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func edgesCrossPoints(l *s2.Loop, pts []s2.Point) bool {
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

func intersects(l *s2.Loop, loop *s2.Loop) bool {
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
	if edgesCrossPoints(l, loop.Vertices()) {
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
		return intersects(l2, l1)
	}
	return intersects(l1, l2)
}

func convertToGeom(str string) (geom.T, error) {
	s := x.WhiteSpace.Replace(str)
	if len(s) < 5 { // [1,2]
		return nil, x.Errorf("Invalid coordinates")
	}
	var g geojson.Geometry
	var m json.RawMessage
	var err error
	if s[0:2] == "[[" {
		g.Type = "Polygon"
		err = m.UnmarshalJSON([]byte(fmt.Sprintf("[%s]", s)))
		if err != nil {
			return nil, x.Wrapf(err, "Invalid coordinates")
		}
		g.Coordinates = &m
		g1, err := g.Decode()
		if err != nil {
			return nil, x.Wrapf(err, "Invalid coordinates")
		}
		coords := g1.(*geom.Polygon).Coords()
		if coords[0][0][0] != coords[0][len(coords[0])-1][0] ||
			coords[0][0][1] != coords[0][len(coords[0])-1][1] {
			return nil, x.Errorf("Last coord not same as first")
		}
		return g1, nil
	}

	if s[0] == '[' {
		g.Type = "Point"
		err = m.UnmarshalJSON([]byte(s))
		if err != nil {
			return nil, x.Wrapf(err, "Invalid coordinates")
		}
		g.Coordinates = &m
		return g.Decode()
	}
	return nil, x.Errorf("invalid coordinates")
}
