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
	"bytes"
	"strconv"
	"strings"

	"github.com/golang/geo/s2"
	geom "github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	"github.com/twpayne/go-geom/xy"
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

// GeoQueryData is pb.data used by the geo query filter to additionally filter the geometries.
type GeoQueryData struct {
	pt    *s2.Point  // If not nil, the input data was a point
	loops []*s2.Loop // If not empty, the input data was a polygon/multipolygon or it was a near query.
	qtype QueryType
}

// IsGeoFunc returns if a function is of geo type.
func IsGeoFunc(str string) bool {
	switch str {
	case "near", "contains", "within", "intersects":
		return true
	}

	return false
}

// GetGeoTokens returns the corresponding index keys based on the type
// of function.
func GetGeoTokens(srcFunc *pb.SrcFunction) ([]string, *GeoQueryData, error) {
	x.AssertTruef(len(srcFunc.Name) > 0, "Invalid function")
	funcName := strings.ToLower(srcFunc.Name)
	switch funcName {
	case "near":
		if len(srcFunc.Args) != 2 {
			return nil, nil, errors.Errorf("near function requires 2 arguments, but got %d",
				len(srcFunc.Args))
		}
		maxDist, err := strconv.ParseFloat(srcFunc.Args[1], 64)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "Error while converting distance to float")
		}
		if maxDist < 0 {
			return nil, nil, errors.Errorf("Distance cannot be negative")
		}
		g, err := convertToGeom(srcFunc.Args[0])
		if err != nil {
			return nil, nil, err
		}
		return queryTokensGeo(QueryTypeNear, g, maxDist)
	case "within":
		if len(srcFunc.Args) != 1 {
			return nil, nil, errors.Errorf("within function requires 1 arguments, but got %d",
				len(srcFunc.Args))
		}
		g, err := convertToGeom(srcFunc.Args[0])
		if err != nil {
			return nil, nil, err
		}
		return queryTokensGeo(QueryTypeWithin, g, 0.0)
	case "contains":
		if len(srcFunc.Args) != 1 {
			return nil, nil, errors.Errorf("contains function requires 1 arguments, but got %d",
				len(srcFunc.Args))
		}
		g, err := convertToGeom(srcFunc.Args[0])
		if err != nil {
			return nil, nil, err
		}
		return queryTokensGeo(QueryTypeContains, g, 0.0)
	case "intersects":
		if len(srcFunc.Args) != 1 {
			return nil, nil, errors.Errorf("intersects function requires 1 arguments, but got %d",
				len(srcFunc.Args))
		}
		g, err := convertToGeom(srcFunc.Args[0])
		if err != nil {
			return nil, nil, err
		}
		return queryTokensGeo(QueryTypeIntersects, g, 0.0)
	default:
		return nil, nil, errors.Errorf("Invalid geo function")
	}
}

// queryTokensGeo returns the tokens to be used to look up the geo index for a given filter.
// qt is the type of Geo query - near/intersects/contains/within
// g is the geom.T representation of the input. It could be a point/polygon/multipolygon.
// maxDistance is distance in metres, only used for near query.
func queryTokensGeo(qt QueryType, g geom.T, maxDistance float64) ([]string, *GeoQueryData, error) {
	var loops []*s2.Loop
	var pt *s2.Point
	var err error
	switch v := g.(type) {
	case *geom.Point:
		// Get s2 point from geom.Point.
		p := pointFromPoint(v)
		pt = &p

		if qt == QueryTypeNear {
			// We use the point and make a loop with radius maxDistance. Then we can use this for
			// the rest of the query.
			if maxDistance <= 0 {
				return nil, nil, errors.Errorf("Invalid max distance specified for a near query")
			}
			a := EarthAngle(maxDistance)
			l := s2.RegularLoop(*pt, a, 100)
			loops = append(loops, l)
		}
	case *geom.Polygon:
		l, err := loopFromPolygon(v)
		if err != nil {
			return nil, nil, err
		}
		loops = append(loops, l)

	case *geom.MultiPolygon:
		// We get a loop for each polygon.
		for i := 0; i < v.NumPolygons(); i++ {
			l, err := loopFromPolygon(v.Polygon(i))
			if err != nil {
				return nil, nil, err
			}
			loops = append(loops, l)
		}

	default:
		return nil, nil, errors.Errorf("Cannot query using a geometry of type %T", v)
	}

	x.AssertTruef(len(loops) > 0 || pt != nil, "We should have a point or a loop.")

	var cover, parents s2.CellUnion
	if qt == QueryTypeNear {
		if len(loops) == 0 {
			return nil, nil, errors.Errorf("Internal error while processing near query.")
		}
		cover = coverLoop(loops[0], MinCellLevel, MaxCellLevel, MaxCells)
		parents = getParentCells(cover, MinCellLevel)
	} else {
		parents, cover, err = indexCells(g)
		if err != nil {
			return nil, nil, err
		}
	}

	switch qt {
	case QueryTypeWithin:
		// For a within query we only need to look at the objects whose parents match our cover.
		// So we take our cover and prefix with the parentPrefix to look in the index.
		if len(loops) == 0 {
			return nil, nil, errors.Errorf("Require a polygon for within query")
		}
		toks := createTokens(cover, parentPrefix)
		return toks, &GeoQueryData{loops: loops, qtype: qt}, nil

	case QueryTypeContains:
		// For a contains query, we only need to look at the objects whose cover matches our
		// parents. So we take our parents and prefix with the coverPrefix to look in the index.
		return createTokens(parents, coverPrefix), &GeoQueryData{pt: pt, loops: loops, qtype: qt}, nil

	case QueryTypeNear:
		if pt == nil {
			return []string{}, nil, errors.Errorf("Require a point for a within query.")
		}
		// A near query is the same as the intersects query. We form a loop with the given point and
		// the radius and then see what all does it intersect with.
		toks := parentCoverTokens(parents, cover)
		return toks, &GeoQueryData{loops: loops, qtype: QueryTypeIntersects}, nil

	case QueryTypeIntersects:
		// An intersects query is as the name suggests all the entities which intersect with the
		// given region. So we look at all the objects whose parents match our cover as well as
		// all the objects whose cover matches our parents.
		if len(loops) == 0 {
			return nil, nil, errors.Errorf("Require a polygon for intersects query")
		}
		toks := parentCoverTokens(parents, cover)
		return toks, &GeoQueryData{loops: loops, qtype: qt}, nil

	default:
		return nil, nil, errors.Errorf("Unknown query type")
	}
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
		return q.intersects(g)
	}
	return false
}

func loopWithinMultiloops(l *s2.Loop, loops []*s2.Loop) bool {
	for _, s2loop := range loops {
		if Contains(s2loop, l) {
			return true
		}
	}
	return false
}

// returns true if the geometry represented by g is within the given loop
func (q GeoQueryData) isWithin(g geom.T) bool {
	x.AssertTruef(q.pt != nil || len(q.loops) > 0, "At least a point, loop should be defined.")
	switch geometry := g.(type) {
	case *geom.Point:
		s2pt := pointFromPoint(geometry)
		if q.pt != nil {
			return q.pt.ApproxEqual(s2pt)
		}

		if len(q.loops) > 0 {
			for _, l := range q.loops {
				if l.ContainsPoint(s2pt) {
					return true
				}
			}
			return false
		}
	case *geom.Polygon:
		s2loop, err := loopFromPolygon(geometry)
		if err != nil {
			return false
		}
		if len(q.loops) > 0 {
			for _, l := range q.loops {
				if Contains(l, s2loop) {
					return true
				}
			}
			return false
		}
	case *geom.MultiPolygon:
		// We check each polygon in the multipolygon should be within some loop of q.loops.
		if len(q.loops) > 0 {
			for i := 0; i < geometry.NumPolygons(); i++ {
				s2loop, err := loopFromPolygon(geometry.Polygon(i))
				if err != nil {
					return false
				}
				if !loopWithinMultiloops(s2loop, q.loops) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func multiPolygonContainsLoop(g *geom.MultiPolygon, l *s2.Loop) bool {
	for i := 0; i < g.NumPolygons(); i++ {
		p := g.Polygon(i)
		s2loop, err := loopFromPolygon(p)
		if err != nil {
			return false
		}
		if Contains(s2loop, l) {
			return true
		}
	}
	return false
}

// returns true if the geometry represented by g contains the given point/polygon.
// g is the geom.T representation of the value which is the stored in the DB.
func (q GeoQueryData) contains(g geom.T) bool {
	x.AssertTruef(q.pt != nil || len(q.loops) > 0, "At least a point or loop should be defined.")
	switch v := g.(type) {
	case *geom.Polygon:
		if q.pt != nil {
			return polygonContainsCoord(v, q.pt)
		}

		s2loop, err := loopFromPolygon(v)
		if err != nil {
			return false
		}

		// Input could be a multipolygon, in which q.loops would have more than 1 loop. Each loop
		// in the query should be part of the s2loop.
		for _, l := range q.loops {
			if !Contains(s2loop, l) {
				return false
			}
		}
		return true
	case *geom.MultiPolygon:
		if q.pt != nil {
			for j := 0; j < v.NumPolygons(); j++ {
				if polygonContainsCoord(v.Polygon(j), q.pt) {
					return true
				}
			}

			return false
		}

		if len(q.loops) > 0 {
			// All the loops that are part of the query should be part of some loop of v.
			for _, l := range q.loops {
				if !multiPolygonContainsLoop(v, l) {
					return false
				}
			}
			return true
		}

		return false
	default:
		// We will only consider polygons for contains queries.
		return false
	}
}

func polygonContainsCoord(v *geom.Polygon, pt *s2.Point) bool {
	for i := 0; i < v.NumLinearRings(); i++ {
		r := v.LinearRing(i)
		ll := s2.LatLngFromPoint(*pt)
		p := []float64{ll.Lng.Degrees(), ll.Lat.Degrees()}
		if xy.IsPointInRing(r.Layout(), p, r.FlatCoords()) {
			return true
		}
	}

	return false
}

// returns true if the geometry represented by uid/attr intersects the given loop or point
func (q GeoQueryData) intersects(g geom.T) bool {
	x.AssertTruef(len(q.loops) > 0, "Loop should be defined for intersects.")
	switch v := g.(type) {
	case *geom.Point:
		p := pointFromPoint(v)
		// else loop is not nil
		for _, l := range q.loops {
			if l.ContainsPoint(p) {
				return true
			}
		}
		return false

	case *geom.Polygon:
		l, err := loopFromPolygon(v)
		if err != nil {
			return false
		}
		for _, loop := range q.loops {
			if Intersects(l, loop) {
				return true
			}
		}
		return false
	case *geom.MultiPolygon:
		// We must compare all polygons in g with those in the query.
		for i := 0; i < v.NumPolygons(); i++ {
			l, err := loopFromPolygon(v.Polygon(i))
			if err != nil {
				return false
			}
			for _, loop := range q.loops {
				if Intersects(l, loop) {
					return true
				}
			}
		}
		return false
	default:
		// A type that we don't know how to handle.
		return false
	}
}

// MatchGeo matches values and GeoQueryData and ensures that the value actually
// matches the query criteria.
func MatchGeo(value *pb.TaskValue, q *GeoQueryData) bool {
	valBytes := value.Val
	if bytes.Equal(valBytes, nil) {
		return false
	}
	vType := value.ValType
	if TypeID(vType) != GeoID {
		return false
	}
	src := ValueForType(BinaryID)
	src.Value = valBytes
	gc, err := Convert(src, GeoID)
	if err != nil {
		return false
	}
	g := gc.Value.(geom.T)
	return q.MatchesFilter(g)
}
