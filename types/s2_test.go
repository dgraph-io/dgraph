package types

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func TestConvertToGeoJson_Point(t *testing.T) {
	s := `[1, 2] `
	b, err := ConvertToGeoJson(s)
	require.NoError(t, err)
	require.Equal(t, geom.Coord{1, 2}, b.(*geom.Point).Coords())
}

func TestConvertToGeoJson_Poly(t *testing.T) {
	s := `[[1, 2], [3, 4], [4, 6], [1, 2]]`
	b, err := ConvertToGeoJson(s)
	require.NoError(t, err)
	require.Equal(t,
		[][]geom.Coord{[]geom.Coord{geom.Coord{1, 2}, geom.Coord{3, 4}, geom.Coord{4, 6}, geom.Coord{1, 2}}},
		b.(*geom.Polygon).Coords())
	x, err := geojson.Marshal(b)
	fmt.Println(string(x))
}

func TestConvertToGeoJson_PolyError1(t *testing.T) {
	s := `[[1, 2], [3, 4], [4, 6]]`
	_, err := ConvertToGeoJson(s)
	require.Error(t, err)
}

func TestConvertToGeoJson_PolyError2(t *testing.T) {
	s := `[[1, 2], [3, 4], [4, 6], [1, 2], []]`
	_, err := ConvertToGeoJson(s)
	require.Error(t, err)
}
