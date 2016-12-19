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
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkb"
)

func formData(t *testing.T, str string) string {
	p, err := loadPolygon(str)
	require.NoError(t, err)

	d, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	gd := ValueForType(StringID)
	src := ValueForType(GeoID)
	src.Value = []byte(d)
	err = Convert(src, &gd)
	require.NoError(t, err)
	gb := gd.Value.(string)
	return string(gb)
}

func formDataPoint(t *testing.T, p *geom.Point) string {
	d, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	gd := ValueForType(StringID)
	src := ValueForType(GeoID)
	src.Value = []byte(d)
	err = Convert(src, &gd)
	require.NoError(t, err)
	gb := gd.Value.(string)

	return string(gb)
}
func formDataPolygon(t *testing.T, p *geom.Polygon) string {
	d, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	gd := ValueForType(StringID)
	src := ValueForType(GeoID)
	src.Value = []byte(d)
	err = Convert(src, &gd)
	require.NoError(t, err)
	gb := gd.Value.(string)

	return string(gb)
}

func TestQueryTokensPolygon(t *testing.T) {
	data := formData(t, "testdata/zip.json")

	qtypes := []QueryType{QueryTypeWithin, QueryTypeIntersects}
	for _, qt := range qtypes {
		toks, qd, err := queryTokens(qt, data, 0.0)
		require.NoError(t, err)

		if qt == QueryTypeWithin {
			require.Len(t, toks, 18)
		} else {
			require.Len(t, toks, 65)
		}
		require.NotNil(t, qd)
		require.Equal(t, qd.qtype, qt)
		require.NotNil(t, qd.loop)
		require.Nil(t, qd.pt)
		require.Nil(t, qd.cap)
	}
}

func TestQueryTokensPolygonError(t *testing.T) {
	data := formData(t, "testdata/zip.json")
	qtypes := []QueryType{QueryTypeNear, QueryTypeContains}
	for _, qt := range qtypes {
		_, _, err := queryTokens(qt, data, 0.0)
		require.Error(t, err)
	}
}

func TestQueryTokensPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data := formDataPoint(t, p)

	qtypes := []QueryType{QueryTypeWithin, QueryTypeIntersects, QueryTypeContains}
	for _, qt := range qtypes {
		toks, qd, err := queryTokens(qt, data, 0.0)
		require.NoError(t, err)

		if qt == QueryTypeWithin {
			require.Len(t, toks, 1)
		} else if qt == QueryTypeContains {
			require.Len(t, toks, MaxCellLevel-MinCellLevel+1)
		} else {
			require.Len(t, toks, MaxCellLevel-MinCellLevel+2)
		}
		require.NotNil(t, qd)
		require.Equal(t, qd.qtype, qt)
		require.Nil(t, qd.loop)
		require.NotNil(t, qd.pt)
		require.Nil(t, qd.cap)
	}
}

func TestQueryTokensNear(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data := formDataPoint(t, p)

	toks, qd, err := queryTokens(QueryTypeNear, data, 1000.0)
	require.NoError(t, err)

	require.Equal(t, len(toks), 15)
	require.NotNil(t, qd)
	require.Equal(t, qd.qtype, QueryTypeNear)
	require.Nil(t, qd.loop)
	require.Nil(t, qd.pt)
	require.NotNil(t, qd.cap)
}

func TestQueryTokensNearError(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data := formDataPoint(t, p)

	_, _, err := queryTokens(QueryTypeNear, data, 0.0)
	require.Error(t, err) // no max distance
}

func TestMatchesFilterWithinPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data := formDataPoint(t, p)
	_, qd, err := queryTokens(QueryTypeWithin, data, 0.0)
	require.NoError(t, err)

	// Poly contains point
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.True(t, qd.MatchesFilter(p2))

	// Poly doesn't contain point
	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(p3))

	// Poly within poly not supported
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.1, 37.1}, {-122.9, 37.1}, {-122.9, 37.9}, {-122.1, 37.9}, {-122.1, 37.1}},
	})
	// Poly containment not supported
	require.False(t, qd.MatchesFilter(poly))
}

func TestMatchesFilterContainsPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data := formDataPoint(t, p)
	_, qd, err := queryTokens(QueryTypeContains, data, 0.0)
	require.NoError(t, err)

	// Points aren't returned for contains queries
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(p2))

	// Polygon contains
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	require.True(t, qd.MatchesFilter(poly))

	// Polygon doesn't contains
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 36}, {-123, 36}, {-123, 37}, {-122, 37}, {-122, 36}},
	})
	require.False(t, qd.MatchesFilter(poly))
}

func TestMatchesFilterIntersectsPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data := formDataPoint(t, p)

	_, qd, err := queryTokens(QueryTypeIntersects, data, 0.0)
	require.NoError(t, err)

	// Same point
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.True(t, qd.MatchesFilter(p2))

	// Different point
	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(p3))

	// containing poly
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	require.True(t, qd.MatchesFilter(poly))

	// Polygon doesn't contains
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 36}, {-123, 36}, {-123, 37}, {-122, 37}, {-122, 36}},
	})
	require.False(t, qd.MatchesFilter(poly))
}

func TestMatchesFilterIntersectsPolygon(t *testing.T) {
	p := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	data := formDataPolygon(t, p)
	_, qd, err := queryTokens(QueryTypeIntersects, data, 0.0)
	require.NoError(t, err)

	// Poly contains point
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.True(t, qd.MatchesFilter(p2))

	// Poly doesn't contain point
	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(p3))

	// Poly contains poly
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.1, 37.1}, {-122.9, 37.1}, {-122.9, 37.9}, {-122.1, 37.9}, {-122.1, 37.1}},
	})
	require.True(t, qd.MatchesFilter(poly))

	// Poly contained in poly
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-121, 36}, {-124, 36}, {-124, 39}, {-121, 39}, {-121, 36}},
	})
	require.True(t, qd.MatchesFilter(poly))

	// Poly intersecting poly
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-121.5, 36.5}, {-122.5, 36.5}, {-122.5, 37.5}, {-121.5, 37.5}, {-121.5, 36.5}},
	})
	require.True(t, qd.MatchesFilter(poly))

	// Poly not intersecting poly
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-120, 35}, {-121, 35}, {-121, 36}, {-120, 36}, {-120, 35}},
	})
	require.False(t, qd.MatchesFilter(poly))
}

func TestMatchesFilterNearPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data := formDataPoint(t, p)
	_, qd, err := queryTokens(QueryTypeNear, data, 1000.0)
	require.NoError(t, err)

	// Same point
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.True(t, qd.MatchesFilter(p2))

	// Close point
	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.080668, 37.426753})
	require.True(t, qd.MatchesFilter(p3))

	// Far point
	p3 = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(p3))

	// Polys aren't returned for near queries
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	require.False(t, qd.MatchesFilter(poly))
}
