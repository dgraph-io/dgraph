/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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

	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/geo/s2"
	"github.com/pkg/errors"
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

func findVertex(a *s2.Loop, p s2.Point) int {
	pts := a.Vertices()
	for i := 0; i < len(pts); i++ {
		if pts[i].ApproxEqual(p) {
			return i
		}
	}
	return -1
}

// Contains checks whether loop A contains loop B.
func Contains(a *s2.Loop, b *s2.Loop) bool {
	// For this loop A to contains the given loop B, all of the following must
	// be true:
	//
	//  (1) There are no edge crossings between A and B except at vertices.
	//
	//  (2) At every vertex that is shared between A and B, the local edge
	//      ordering implies that A contains B.
	//
	//  (3) If there are no shared vertices, then A must contain a vertex of B
	//      and B must not contain a vertex of A.  (An arbitrary vertex may be
	//      chosen in each case.)
	//
	// The second part of (3) is necessary to detect the case of two loops whose
	// union is the entire sphere, i.e. two loops that contains each other's
	// boundaries but not each other's interiors.

	if !a.RectBound().Contains(b.RectBound()) {
		return false
	}

	// Unless there are shared vertices, we need to check whether A contains a
	// vertex of B.  Since shared vertices are rare, it is more efficient to do
	// this test up front as a quick rejection test.
	if !a.ContainsPoint(b.Vertex(0)) && findVertex(a, b.Vertex(0)) < 0 {
		return false
	}

	// Now check whether there are any edge crossings.
	if edgesCrossPoints(a, b.Vertices()) {
		return false
	}

	// At this point we know that the boundaries of A and B do not intersect,
	// and that A contains a vertex of B.  However we still need to check for
	// the case mentioned above, where (A union B) is the entire sphere.
	// Normally this check is very cheap due to the bounding box precondition.
	if a.RectBound().Union(b.RectBound()).IsFull() {
		if b.ContainsPoint(a.Vertex(0)) && findVertex(b, a.Vertex(0)) < 0 {
			return false
		}
	}
	return true

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
	// validate would ensure that we have a closed loop for all the polygons. We don't support open
	// loop polygons.
	closed := func(p *geom.Polygon) error {
		coords := p.Coords()
		if len(coords) == 0 {
			return errors.Errorf("Got empty polygon.")
		}
		// Check that first ring is closed.
		c := coords[0]
		l := len(c)
		if c[0][0] == c[l-1][0] && c[0][1] == c[l-1][1] {
			return nil
		}
		return errors.Errorf("Last coord not same as first")
	}

	validate := func(g geom.T) (geom.T, error) {
		switch v := g.(type) {
		case *geom.MultiPolygon:
			for i := 0; i < v.NumPolygons(); i++ {
				if err := closed(v.Polygon(i)); err != nil {
					return nil, err
				}
			}
		case *geom.Polygon:
			if err := closed(v); err != nil {
				return nil, err
			}
		}
		return g, nil
	}

	var g geojson.Geometry
	if err := json.Unmarshal([]byte(str), &g); err == nil {
		t, err := g.Decode()
		if err != nil {
			return nil, err
		}
		return validate(t)
	}

	s := x.WhiteSpace.Replace(str)
	if len(s) < 5 { // [1,2]
		return nil, errors.Errorf("Invalid coordinates")
	}
	var m json.RawMessage
	var err error

	if s[0:4] == "[[[[" {
		g.Type = "MultiPolygon"
		err = m.UnmarshalJSON([]byte(s))
		if err != nil {
			return nil, errors.Wrapf(err, "Invalid coordinates")
		}
		g.Coordinates = &m
		g1, err := g.Decode()
		if err != nil {
			return nil, errors.Wrapf(err, "Invalid coordinates")
		}
		return validate(g1)
	}

	if s[0:3] == "[[[" {
		g.Type = "Polygon"
		err = m.UnmarshalJSON([]byte(s))
		if err != nil {
			return nil, errors.Wrapf(err, "Invalid coordinates")
		}
		g.Coordinates = &m
		g1, err := g.Decode()
		if err != nil {
			return nil, errors.Wrapf(err, "Invalid coordinates")
		}
		return validate(g1)
	}

	if s[0] == '[' {
		g.Type = "Point"
		err = m.UnmarshalJSON([]byte(s))
		if err != nil {
			return nil, errors.Wrapf(err, "Invalid coordinates")
		}
		g.Coordinates = &m
		return g.Decode()
	}
	return nil, errors.Errorf("Invalid coordinates")
}
