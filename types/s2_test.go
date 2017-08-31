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
	"testing"

	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func TestConvertToGeoJson_Point(t *testing.T) {
	s := `[1.0, 2.0] `
	b, err := convertToGeom(s)
	require.NoError(t, err)
	require.Equal(t, geom.Coord{1, 2}, b.(*geom.Point).Coords())
}

func TestConvertToGeoJson_Poly(t *testing.T) {
	s := `[[[1.123, 2.543], [-3.23, 4.123], [4.43, -6.123], [1.123, 2.543]]]`
	b, err := convertToGeom(s)
	require.NoError(t, err)
	require.Equal(t,
		[][]geom.Coord{{{1.123, 2.543}, {-3.23, 4.123}, {4.43, -6.123}, {1.123, 2.543}}},
		b.(*geom.Polygon).Coords())
	_, err = geojson.Marshal(b)
	require.NoError(t, err)
}

func TestConvertToGeoJson_PolyError1(t *testing.T) {
	s := `[[1.76, -2.234], [3.543, 4.534], [4.54, 6.213]]`
	_, err := convertToGeom(s)
	require.Error(t, err)
}

func TestConvertToGeoJson_PolyError2(t *testing.T) {
	s := `[[1.253, 2.2343], [-4.563, 6.1231], []]`
	_, err := convertToGeom(s)
	require.Error(t, err)
}
