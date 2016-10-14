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
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkb"

	"github.com/dgraph-io/dgraph/types"
)

func TestQueryTokensPolygon(t *testing.T) {
	p, err := loadPolygon("zip.json")
	require.NoError(t, err)

	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	qtypes := []QueryType{QueryTypeWithin, QueryTypeIntersects}
	for _, qt := range qtypes {
		f := &Filter{Data: data, Type: qt}
		toks, qd, err := QueryTokens(f)
		require.NoError(t, err)

		require.Equal(t, len(toks), 18, "Expected 18 keys")
		require.NotNil(t, qd)
		require.Equal(t, qd.qtype, f.Type)
		require.NotNil(t, qd.loop)
		require.Nil(t, qd.pt)
		require.Nil(t, qd.cap)
	}
}

func TestQueryTokensPolygonError(t *testing.T) {
	p, err := loadPolygon("zip.json")
	require.NoError(t, err)

	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	qtypes := []QueryType{QueryTypeNear, QueryTypeContains}
	for _, qt := range qtypes {
		f := &Filter{Data: data, Type: qt}
		_, _, err := QueryTokens(f)
		require.Error(t, err)
	}
}

func TestQueryTokensPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	qtypes := []QueryType{QueryTypeWithin, QueryTypeIntersects, QueryTypeContains}
	for _, qt := range qtypes {
		f := &Filter{Data: data, Type: qt}
		toks, qd, err := QueryTokens(f)
		require.NoError(t, err)

		require.Equal(t, len(toks), MaxCellLevel-MinCellLevel+1)
		require.NotNil(t, qd)
		require.Equal(t, qd.qtype, f.Type)
		require.Nil(t, qd.loop)
		require.NotNil(t, qd.pt)
		require.Nil(t, qd.cap)
	}
}

func TestQueryTokensNear(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	f := &Filter{Data: data, Type: QueryTypeNear, MaxDistance: 1000}
	toks, qd, err := QueryTokens(f)
	require.NoError(t, err)

	require.Equal(t, len(toks), 15)
	require.NotNil(t, qd)
	require.Equal(t, qd.qtype, f.Type)
	require.Nil(t, qd.loop)
	require.Nil(t, qd.pt)
	require.NotNil(t, qd.cap)
}

func TestQueryTokensNearError(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)

	f := &Filter{Data: data, Type: QueryTypeNear}
	_, _, err = QueryTokens(f)
	require.Error(t, err) // no max distance
}

func TestMatchesFilterWithinPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)
	f := &Filter{Data: data, Type: QueryTypeWithin}
	_, qd, err := QueryTokens(f)
	require.NoError(t, err)

	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	// Same point
	require.True(t, qd.MatchesFilter(types.Geo{p2}))

	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	// Different point
	require.False(t, qd.MatchesFilter(types.Geo{p3}))

	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	// Points don't contain polys
	require.False(t, qd.MatchesFilter(types.Geo{poly}))
}

func TestMatchesFilterWithinPolygon(t *testing.T) {
	p := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)
	f := &Filter{Data: data, Type: QueryTypeWithin}
	_, qd, err := QueryTokens(f)
	require.NoError(t, err)

	// Poly contains point
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.True(t, qd.MatchesFilter(types.Geo{p2}))

	// Poly doesn't contain point
	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(types.Geo{p3}))

	// Poly within poly not supported
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.1, 37.1}, {-122.9, 37.1}, {-122.9, 37.9}, {-122.1, 37.9}, {-122.1, 37.1}},
	})
	// Poly containment not supported
	require.False(t, qd.MatchesFilter(types.Geo{poly}))
}

func TestMatchesFilterContainsPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)
	f := &Filter{Data: data, Type: QueryTypeContains}
	_, qd, err := QueryTokens(f)
	require.NoError(t, err)

	// Points aren't returned for contains queries
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(types.Geo{p2}))

	// Polygon contains
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	require.True(t, qd.MatchesFilter(types.Geo{poly}))

	// Polygon doesn't contains
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 36}, {-123, 36}, {-123, 37}, {-122, 37}, {-122, 36}},
	})
	require.False(t, qd.MatchesFilter(types.Geo{poly}))
}

func TestMatchesFilterIntersectsPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)
	f := &Filter{Data: data, Type: QueryTypeIntersects}
	_, qd, err := QueryTokens(f)
	require.NoError(t, err)

	// Same point
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.True(t, qd.MatchesFilter(types.Geo{p2}))

	// Different point
	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(types.Geo{p3}))

	// containing poly
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	require.True(t, qd.MatchesFilter(types.Geo{poly}))

	// Polygon doesn't contains
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 36}, {-123, 36}, {-123, 37}, {-122, 37}, {-122, 36}},
	})
	require.False(t, qd.MatchesFilter(types.Geo{poly}))
}

func TestMatchesFilterIntersectsPolygon(t *testing.T) {
	p := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)
	f := &Filter{Data: data, Type: QueryTypeIntersects}
	_, qd, err := QueryTokens(f)
	require.NoError(t, err)

	// Poly contains point
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.True(t, qd.MatchesFilter(types.Geo{p2}))

	// Poly doesn't contain point
	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(types.Geo{p3}))

	// Poly contains poly
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122.1, 37.1}, {-122.9, 37.1}, {-122.9, 37.9}, {-122.1, 37.9}, {-122.1, 37.1}},
	})
	require.True(t, qd.MatchesFilter(types.Geo{poly}))

	// Poly contained in poly
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-121, 36}, {-124, 36}, {-124, 39}, {-121, 39}, {-121, 36}},
	})
	require.True(t, qd.MatchesFilter(types.Geo{poly}))

	// Poly intersecting poly
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-121.5, 36.5}, {-122.5, 36.5}, {-122.5, 37.5}, {-121.5, 37.5}, {-121.5, 36.5}},
	})
	require.True(t, qd.MatchesFilter(types.Geo{poly}))

	// Poly not intersecting poly
	poly = geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-120, 35}, {-121, 35}, {-121, 36}, {-120, 36}, {-120, 35}},
	})
	require.False(t, qd.MatchesFilter(types.Geo{poly}))
}

func TestMatchesFilterNearPoint(t *testing.T) {
	p := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	data, err := wkb.Marshal(p, binary.LittleEndian)
	require.NoError(t, err)
	f := &Filter{Data: data, Type: QueryTypeNear, MaxDistance: 1000}
	_, qd, err := QueryTokens(f)
	require.NoError(t, err)

	// Same point
	p2 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.082506, 37.4249518})
	require.True(t, qd.MatchesFilter(types.Geo{p2}))

	// Close point
	p3 := geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-122.080668, 37.426753})
	require.True(t, qd.MatchesFilter(types.Geo{p3}))

	// Far point
	p3 = geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{-123.082506, 37.4249518})
	require.False(t, qd.MatchesFilter(types.Geo{p3}))

	// Polys aren't returned for near queries
	poly := geom.NewPolygon(geom.XY).MustSetCoords([][]geom.Coord{
		{{-122, 37}, {-123, 37}, {-123, 38}, {-122, 38}, {-122, 37}},
	})
	require.False(t, qd.MatchesFilter(types.Geo{poly}))
}
