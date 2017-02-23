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
		[][]geom.Coord{[]geom.Coord{geom.Coord{1.123, 2.543}, geom.Coord{-3.23, 4.123}, geom.Coord{4.43, -6.123}, geom.Coord{1.123, 2.543}}},
		b.(*geom.Polygon).Coords())
	x, err := geojson.Marshal(b)
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
