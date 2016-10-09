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

package worker

import (
	"log"

	"github.com/golang/geo/s2"
	"github.com/twpayne/go-geom"

	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// QueryKeys represents the list of keys to be used when querying
type QueryKeys struct {
	keys  []string  // The index keys
	pt    *s2.Point // If not nil, the input data was a point
	loop  *s2.Loop  // If not nil, the input data was a polygon
	cap   *s2.Cap   // If not nil, the cap to be used for a near query
	query int8      // The geo query being performed.
}

// Length is the number of keys in the list
func (q QueryKeys) Length() int {
	return len(q.keys)
}

// Key returns the ith key for the given attribute
func (q QueryKeys) Key(i int, attr string) []byte {
	return []byte(q.keys[i])
}

// PostFilter returns a function to filter the uids after reading them from the index.
func (q QueryKeys) PostFilter(attr string) func(u uint64) bool {
	switch q.query {
	case task.GeoQueryWithin:
		return func(u uint64) bool {
			return isWithin(attr, u, q.pt, q.loop, nil)
		}
	case task.GeoQueryContains:
		return func(u uint64) bool {
			return contains(attr, u, q.pt, q.loop)
		}
	case task.GeoQueryIntersects:
		return func(u uint64) bool {
			return intersects(attr, u, q.pt, q.loop)
		}
	case task.GeoQueryNear:
		// for a point to be near it should be within the loop defined by distance from the origin
		return func(u uint64) bool {
			return isWithin(attr, u, nil, nil, q.cap)
		}
	}
	return nil
}

// newQueryKeys creates a QueryKeys object for the given filter.
func newQueryKeys(f *task.GeoFilter) (*QueryKeys, error) {
	// Try to parse the data as geo type.
	v, err := types.GeoType.Unmarshaler.FromBinary(f.DataBytes())
	if err != nil {
		return nil, err
	}
	g, ok := v.(types.Geo)
	if !ok {
		log.Fatalf("Unexpected type from the unmarshaler.")
	}

	keys, err := geo.IndexKeysFromGeo(g)
	if err != nil {
		return nil, err
	}

	switch v := g.T.(type) {
	case *geom.Point:
		p := geo.PointFromPoint(v)
		if f.Query() == task.GeoQueryNear {
			return nearQueryKeys(p, f.MaxDistance())
		}
		return &QueryKeys{keys: keys, pt: &p, query: f.Query()}, nil

	case *geom.Polygon:
		if f.Query() == task.GeoQueryNear {
			return nil, x.Errorf("Cannot use a polygon in a near query")
		}
		l, err := geo.LoopFromPolygon(v)
		if err != nil {
			return nil, err
		}
		return &QueryKeys{keys: keys, loop: l, query: f.Query()}, nil
	default:
		return nil, x.Errorf("Cannot query using a geometry of type %T", v)
	}
}

// nearQueryKeys creates a QueryKeys object for a near query.
func nearQueryKeys(pt s2.Point, d uint32) (*QueryKeys, error) {
	if d <= 0 {
		return nil, x.Errorf("Invalid max distance specified for a near query")
	}
	a := geo.EarthAngle(float64(d))
	c := s2.CapFromCenterAngle(pt, a)
	keys := geo.IndexKeysForCap(c)
	return &QueryKeys{keys: keys, cap: &c, query: task.GeoQueryNear}, nil
}

// returns true if the geometry represented by uid/attr is within the given loop or point
func isWithin(attr string, uid uint64, pt *s2.Point, loop *s2.Loop, cap *s2.Cap) bool {
	x.Assertf(pt != nil || loop != nil || cap != nil, "At least a point, loop or cap should be defined.")
	if pt != nil {
		// Nothing is inside a point.
		return false
	}
	g, err := parseValue(attr, uid)
	if err != nil {
		return false
	}

	gpt, ok := g.T.(*geom.Point)
	if !ok {
		// We will only consider points for within queries.
		return false
	}

	s2pt := geo.PointFromPoint(gpt)
	if loop != nil {
		return loop.ContainsPoint(s2pt)
	}
	return cap.ContainsPoint(s2pt)
}

// returns true if the geometry represented by uid/attr contains the given loop or point
func contains(attr string, uid uint64, pt *s2.Point, loop *s2.Loop) bool {
	x.Assertf(pt != nil || loop != nil, "At least a point or loop should be defined.")
	if loop != nil {
		// We don't support polygons containing polygons yet.
		return false
	}
	g, err := parseValue(attr, uid)
	if err != nil {
		return false
	}

	poly, ok := g.T.(*geom.Polygon)
	if !ok {
		// We will only consider polygons for contains queries.
		return false
	}

	s2loop, err := geo.LoopFromPolygon(poly)
	if err != nil {
		return false
	}
	return s2loop.ContainsPoint(*pt)
}

// returns true if the geometry represented by uid/attr intersects the given loop or point
func intersects(attr string, uid uint64, pt *s2.Point, loop *s2.Loop) bool {
	x.Assertf(pt != nil || loop != nil, "At least a point or loop should be defined.")
	g, err := parseValue(attr, uid)
	if err != nil {
		return false
	}
	switch v := g.T.(type) {
	case *geom.Point:
		p := geo.PointFromPoint(v)
		if pt != nil {
			// Points only intersect if they are the same. (We allow for small rounding errors)
			return pt.ApproxEqual(p)
		}
		// else loop is not nil
		return loop.ContainsPoint(p)

	case *geom.Polygon:
		l, err := geo.LoopFromPolygon(v)
		if err != nil {
			return false
		}
		if pt != nil {
			return loop.ContainsPoint(*pt)
		}
		// else loop is not nil
		return geo.Intersects(l, loop)
	default:
		// A type that we don't know how to handle.
		return false
	}
}

func parseValue(attr string, uid uint64) (types.Geo, error) {
	store := ws.dataStore
	key := posting.Key(uid, attr)
	pl, decr := posting.GetOrCreate(key, store)
	defer decr()
	val, _, err := pl.Value()
	if err != nil {
		return types.Geo{nil}, err
	}
	g, err := types.GeoType.Unmarshaler.FromBinary(val)
	if err != nil {
		return types.Geo{nil}, err
	}
	return g.(types.Geo), nil
}
