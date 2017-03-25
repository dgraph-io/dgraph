/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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
	s := `[[1.123, 2.543], [-3.23, 4.123], [4.43, -6.123], [1.123, 2.543]]`
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
