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
	"bytes"
	"strconv"
	"strings"

	"github.com/golang/geo/s2"
	"github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/task"
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

// GeoQueryData is internal data used by the geo query filter to additionally filter the geometries.
type GeoQueryData struct {
	pt    *s2.Point // If not nil, the input data was a point
	loop  *s2.Loop  // If not nil, the input data was a polygon
	cap   *s2.Cap   // If not nil, the cap to be used for a near query
	qtype QueryType
}

// IsGeoFunc returns if a function is of geo type.
func IsGeoFunc(str string) bool {
	switch str {
	case "near":
		return true
	case "contains":
		return true
	case "within":
		return true
	case "intersects":
		return true
	}

	return false
}

// GetGeoTokens returns the corresponding index keys based on the type
// of function.
func GetGeoTokens(funcArgs []string) ([]string, *GeoQueryData, error) {
	x.AssertTruef(len(funcArgs) > 1, "Invalid function")
	funcName := strings.ToLower(funcArgs[0])
	switch funcName {
	case "near":
		if len(funcArgs) != 3 {
			return nil, nil, x.Errorf("near function requires 3 arguments, but got %d",
				len(funcArgs))
		}
		maxDist, err := strconv.ParseFloat(funcArgs[2], 64)
		if err != nil {
			return nil, nil, x.Wrapf(err, "Error while converting distance to float")
		}
		if maxDist < 0 {
			return nil, nil, x.Errorf("Distance cannot be negative")
		}
		g, err := convertToGeom(funcArgs[1])
		if err != nil {
			return nil, nil, err
		}
		return queryTokensGeo(QueryTypeNear, g, maxDist)
	case "within":
		if len(funcArgs) != 2 {
			return nil, nil, x.Errorf("within function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		g, err := convertToGeom(funcArgs[1])
		if err != nil {
			return nil, nil, err
		}
		return queryTokensGeo(QueryTypeWithin, g, 0.0)
	case "contains":
		if len(funcArgs) != 2 {
			return nil, nil, x.Errorf("contains function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		g, err := convertToGeom(funcArgs[1])
		if err != nil {
			return nil, nil, err
		}
		return queryTokensGeo(QueryTypeContains, g, 0.0)
	case "intersects":
		if len(funcArgs) != 2 {
			return nil, nil, x.Errorf("intersects function requires 2 arguments, but got %d",
				len(funcArgs))
		}
		g, err := convertToGeom(funcArgs[1])
		if err != nil {
			return nil, nil, err
		}
		return queryTokensGeo(QueryTypeIntersects, g, 0.0)
	default:
		return nil, nil, x.Errorf("Invalid geo function")
	}
}

// queryTokensGeo returns the tokens to be used to look up the geo index for a given filter.
func queryTokensGeo(qt QueryType, g geom.T, maxDistance float64) ([]string, *GeoQueryData, error) {
	var l *s2.Loop
	var pt *s2.Point
	var err error
	switch v := g.(type) {
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
		if l == nil {
			return nil, nil, x.Errorf("Require a polygon for within query")
		}
		toks := createTokens(cover, parentPrefix)
		return toks, &GeoQueryData{loop: l, qtype: qt}, nil

	case QueryTypeContains:
		// For a contains query, we only need to look at the objects whose cover matches our
		// parents. So we take our parents and prefix with the coverPrefix to look in the index.
		return createTokens(parents, coverPrefix), &GeoQueryData{pt: pt, loop: l, qtype: qt}, nil

	case QueryTypeNear:
		if l != nil {
			return nil, nil, x.Errorf("Cannot use a polygon in a near query")
		}
		return nearQueryKeys(*pt, maxDistance)

	case QueryTypeIntersects:
		// An intersects query is as the name suggests all the entities which intersect with the
		// given region. So we look at all the objects whose parents match our cover as well as
		// all the objects whose cover matches our parents.
		if l == nil {
			return nil, nil, x.Errorf("Require a polygon for intersects query")
		}
		toks := parentCoverTokens(parents, cover)
		return toks, &GeoQueryData{loop: l, qtype: qt}, nil

	default:
		return nil, nil, x.Errorf("Unknown query type")
	}
}

// nearQueryKeys creates a QueryKeys object for a near query.
func nearQueryKeys(pt s2.Point, d float64) ([]string, *GeoQueryData, error) {
	if d <= 0 {
		return nil, nil, x.Errorf("Invalid max distance specified for a near query")
	}
	a := EarthAngle(d)
	c := s2.CapFromCenterAngle(pt, a)
	cu := indexCellsForCap(c)
	// A near query is similar to within, where we are looking for points within the cap. So we need
	// all objects whose parents match the cover of the cap.
	return createTokens(cu, parentPrefix), &GeoQueryData{cap: &c, qtype: QueryTypeNear}, nil
}

// MatchesFilter applies the query filter to a geo value
func (q GeoQueryData) MatchesFilter(g geom.T) bool {
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

// WithinPolygon returns true if g1 is within g2 approximaltely.
// Note that this is very far from accurate within function and is
// a temporary fix.
// TODO(Ashwin): Improve this to make it more accurate.
func WithinPolygon(g1 *s2.Loop, g2 *s2.Loop) bool {
	for _, point := range g1.Vertices() {
		if !g2.ContainsPoint(point) {
			return false
		}
	}
	return true
}

// TODO(Ashwin): Improve this to make it more accurate.
func WithinCapPolygon(g1 *s2.Loop, g2 *s2.Cap) bool {
	for _, point := range g1.Vertices() {
		if !g2.ContainsPoint(point) {
			return false
		}
	}
	return true
}

// returns true if the geometry represented by g is within the given loop or cap
func (q GeoQueryData) isWithin(g geom.T) bool {
	x.AssertTruef(q.pt != nil || q.loop != nil || q.cap != nil, "At least a point, loop or cap should be defined.")
	gpoly, ok := g.(*geom.Polygon)
	if ok {
		// We will only consider points for within queries.
		if !ok {
			return false
		}
		s2loop, err := loopFromPolygon(gpoly)
		if err != nil {
			return false
		}
		if q.loop != nil {
			return WithinPolygon(s2loop, q.loop)
		}
		if q.cap != nil {
			return WithinCapPolygon(s2loop, q.cap)
		}
	}

	gpt, ok := g.(*geom.Point)
	if ok {
		s2pt := pointFromPoint(gpt)
		if q.pt != nil {
			return q.pt.ApproxEqual(s2pt)
		}

		if q.loop != nil {
			return q.loop.ContainsPoint(s2pt)
		}
		return q.cap.ContainsPoint(s2pt)
	}
	return false
}

// returns true if the geometry represented by uid/attr contains the given point
func (q GeoQueryData) contains(g geom.T) bool {
	x.AssertTruef(q.pt != nil || q.loop != nil, "At least a point or loop should be defined.")

	poly, ok := g.(*geom.Polygon)
	if !ok {
		// We will only consider polygons for contains queries.
		return false
	}

	s2loop, err := loopFromPolygon(poly)
	if err != nil {
		return false
	}
	// If its a loop check if it lies within other loop. Else Check the point.
	if q.loop != nil {
		// We don't support polygons containing polygons yet.
		return WithinPolygon(q.loop, s2loop)
	}
	return s2loop.ContainsPoint(*q.pt)
}

// returns true if the geometry represented by uid/attr intersects the given loop or point
func (q GeoQueryData) intersects(g geom.T) bool {
	x.AssertTruef(q.loop != nil, "Loop should be defined for intersects.")
	switch v := g.(type) {
	case *geom.Point:
		p := pointFromPoint(v)
		// else loop is not nil
		return q.loop.ContainsPoint(p)

	case *geom.Polygon:
		l, err := loopFromPolygon(v)
		if err != nil {
			return false
		}
		// else loop is not nil
		return Intersects(l, q.loop)
	default:
		// A type that we don't know how to handle.
		return false
	}
}

// FilterGeoUids filters the uids based on the corresponding values and GeoQueryData.
func FilterGeoUids(uids *task.List, values []*task.Value, q *GeoQueryData) *task.List {
	x.AssertTruef(len(values) == algo.ListLen(uids), "lengths not matching")
	o := new(task.List)
	var out algo.WriteIterator
	out.Init(o)
	var it algo.ListIterator
	it.Init(uids)
	for i := -1; it.Valid(); it.Next() {
		i++
		valBytes := values[i].Val
		if bytes.Equal(valBytes, nil) {
			continue
		}
		vType := values[i].ValType
		if TypeID(vType) != GeoID {
			continue
		}
		src := ValueForType(BinaryID)
		src.Value = valBytes
		gc, err := Convert(src, GeoID)
		if err != nil {
			continue
		}
		g := gc.Value.(geom.T)

		if !q.MatchesFilter(g) {
			continue
		}

		// we matched the geo filter, add the uid to the list
		out.Append(it.Val())
	}
	out.End()
	return o
}
