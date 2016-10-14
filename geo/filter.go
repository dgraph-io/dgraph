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

type QueryType byte

const (
	QueryTypeWithin QueryType = iota
	QueryTypeContains
	QueryTypeIntersects
	QueryTypeNear
)

type Filter struct {
	Type        QueryType
	Data        []byte
	MaxDistance float64
}

// QueryKeys represents the list of keys to be used when querying
type QueryData struct {
	pt    *s2.Point // If not nil, the input data was a point
	loop  *s2.Loop  // If not nil, the input data was a polygon
	cap   *s2.Cap   // If not nil, the cap to be used for a near query
	qtype QueryType
}

// QueryTokens returns the tokens to be used to look up the geo index for a given filter.
func QueryTokens(f *Filter) ([][]byte, *QueryData, error) {
	// Try to parse the data as geo type.
	var g types.Geo
	err := g.UnmarshalBinary(f.Data)
	if err != nil {
		return nil, nil, err
	}

	cu, err := indexCells(g)
	if err != nil {
		return nil, nil, err
	}

	switch v := g.T.(type) {
	case *geom.Point:
		p := pointFromPoint(v)
		if f.Type == QueryTypeNear {
			return nearQueryKeys(p, f.MaxDistance)
		}
		return ToTokens(cu), &QueryData{pt: &p, qtype: f.Type}, nil

	case *geom.Polygon:
		if f.Type == QueryTypeNear || f.Type == QueryTypeContains {
			return nil, nil, x.Errorf("Cannot use a polygon in a near or contains query")
		}
		l, err := loopFromPolygon(v)
		if err != nil {
			return nil, nil, err
		}
		return ToTokens(cu), &QueryData{loop: l, qtype: f.Type}, nil
	default:
		return nil, nil, x.Errorf("Cannot query using a geometry of type %T", v)
	}
}

// nearQueryKeys creates a QueryKeys object for a near query.
func nearQueryKeys(pt s2.Point, d float64) ([][]byte, *QueryData, error) {
	if d <= 0 {
		return nil, nil, x.Errorf("Invalid max distance specified for a near query")
	}
	a := EarthAngle(d)
	c := s2.CapFromCenterAngle(pt, a)
	cu := indexCellsForCap(c)
	return ToTokens(cu), &QueryData{cap: &c, qtype: QueryTypeNear}, nil
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
	x.Assertf(q.pt != nil || q.loop != nil || q.cap != nil, "At least a point, loop or cap should be defined.")
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
	x.Assertf(q.pt != nil || q.loop != nil, "At least a point or loop should be defined.")
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
	x.Assertf(q.pt != nil || q.loop != nil, "At least a point or loop should be defined.")
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
