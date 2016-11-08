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

package geo

import (
	"github.com/golang/geo/s2"
	"github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// QueryType indicates the type of geo query.
type QueryType byte

const (
	// QueryTypeWithin finds all points that are within the given geometry
	QueryTypeWithin QueryType = iota
	// QueryTypeContains finds all polygons that contain the given point
	QueryTypeContains
	// QueryTypeIntersects finds all objects that intersect the given geometry
	QueryTypeIntersects
	// QueryTypeNear finds all points that are within the given distance from the given point.
	QueryTypeNear
)

/*
// Filter describes the geo query.
type Filter struct {
	Type        QueryType // The type of the query
	Data        []byte    // The geometry in binary form which is the parameter for the query
	MaxDistance float64   // MaxDistance for near queries
}
*/

// QueryData is internal data used by the geo query filter to additionally filter the geometries.
type QueryData struct {
	pt    *s2.Point // If not nil, the input data was a point
	loop  *s2.Loop  // If not nil, the input data was a polygon
	cap   *s2.Cap   // If not nil, the cap to be used for a near query
	qtype QueryType
}

// QueryTokens returns the tokens to be used to look up the geo index for a given filter.
func QueryTokens(data []byte, qt QueryType, maxDistance float64) ([]string, *QueryData, error) {
	// Try to parse the data as geo type.
	var g types.Geo
	err := g.UnmarshalBinary(data)
	if err != nil {
		return nil, nil, err
	}
	var l *s2.Loop
	var pt *s2.Point

	switch v := g.T.(type) {
	case *geom.Point:
		p := pointFromPoint(v)
		pt = &p

	case *geom.Polygon:
		l, err = loopFromPolygon(v)
		if err != nil {
			return nil, nil, err
		}

	default:
		return nil, nil, x.Errorf("Cannot query using a geometry of type %T", v)
	}

	x.AssertTruef(l != nil || pt != nil, "We should have a point or a loop.")

	parents, cover, err := indexCells(g)
	if err != nil {
		return nil, nil, err
	}

	switch qt {
	case QueryTypeWithin:
		// For a within query we only need to look at the objects whose parents match our cover.
		// So we take our cover and prefix with the parentPrefix to look in the index.
		toks := toTokens(cover, parentPrefix)
		return toks, &QueryData{pt: pt, loop: l, qtype: qt}, nil

	case QueryTypeContains:
		if l != nil {
			return nil, nil, x.Errorf("Cannot use a polygon in a contains query")
		}
		// For a contains query, we only need to look at the objects whose cover matches our
		// parents. So we take our parents and prefix with the coverPrefix to look in the index.
		return toTokens(parents, coverPrefix), &QueryData{pt: pt, qtype: qt}, nil

	case QueryTypeNear:
		if l != nil {
			return nil, nil, x.Errorf("Cannot use a polygon in a near query")
		}
		return nearQueryKeys(*pt, maxDistance)

	case QueryTypeIntersects:
		// An intersects query is essentially the union of contains and within. So we look at all
		// the objects whose parents match our cover as well as all the objects whose cover matches
		// our parents.
		toks := parentCoverTokens(parents, cover)
		return toks, &QueryData{pt: pt, loop: l, qtype: qt}, nil

	default:
		return nil, nil, x.Errorf("Unknown query type")
	}
}

// nearQueryKeys creates a QueryKeys object for a near query.
func nearQueryKeys(pt s2.Point, d float64) ([]string, *QueryData, error) {
	if d <= 0 {
		return nil, nil, x.Errorf("Invalid max distance specified for a near query")
	}
	a := EarthAngle(d)
	c := s2.CapFromCenterAngle(pt, a)
	cu := indexCellsForCap(c)
	// A near query is similar to within, where we are looking for points within the cap. So we need
	// all objects whose parents match the cover of the cap.
	return toTokens(cu, parentPrefix), &QueryData{cap: &c, qtype: QueryTypeNear}, nil
}

// MatchesFilter applies the query filter to a geo value
func (q QueryData) MatchesFilter(g types.Geo) bool {
	switch q.qtype {
	case QueryTypeWithin:
		return q.isWithin(g)
	case QueryTypeContains:
		return q.contains(g)
	case QueryTypeIntersects:
		return q.intersects(g)
	case QueryTypeNear:
		if q.cap == nil {
			return false
		}
		return q.isWithin(g)
	}
	return false
}

// returns true if the geometry represented by g is within the given loop or cap
func (q QueryData) isWithin(g types.Geo) bool {
	x.AssertTruef(q.pt != nil || q.loop != nil || q.cap != nil, "At least a point, loop or cap should be defined.")
	gpt, ok := g.T.(*geom.Point)
	if !ok {
		// We will only consider points for within queries.
		return false
	}

	s2pt := pointFromPoint(gpt)
	if q.pt != nil {
		return q.pt.ApproxEqual(s2pt)
	}

	if q.loop != nil {
		return q.loop.ContainsPoint(s2pt)
	}
	return q.cap.ContainsPoint(s2pt)
}

// returns true if the geometry represented by uid/attr contains the given point
func (q QueryData) contains(g types.Geo) bool {
	x.AssertTruef(q.pt != nil || q.loop != nil, "At least a point or loop should be defined.")
	if q.loop != nil {
		// We don't support polygons containing polygons yet.
		return false
	}

	poly, ok := g.T.(*geom.Polygon)
	if !ok {
		// We will only consider polygons for contains queries.
		return false
	}

	s2loop, err := loopFromPolygon(poly)
	if err != nil {
		return false
	}
	return s2loop.ContainsPoint(*q.pt)
}

// returns true if the geometry represented by uid/attr intersects the given loop or point
func (q QueryData) intersects(g types.Geo) bool {
	x.AssertTruef(q.pt != nil || q.loop != nil, "At least a point or loop should be defined.")
	switch v := g.T.(type) {
	case *geom.Point:
		p := pointFromPoint(v)
		if q.pt != nil {
			// Points only intersect if they are the same. (We allow for small rounding errors)
			return q.pt.ApproxEqual(p)
		}
		// else loop is not nil
		return q.loop.ContainsPoint(p)

	case *geom.Polygon:
		l, err := loopFromPolygon(v)
		if err != nil {
			return false
		}
		if q.pt != nil {
			return l.ContainsPoint(*q.pt)
		}
		// else loop is not nil
		return Intersects(l, q.loop)
	default:
		// A type that we don't know how to handle.
		return false
	}
}
