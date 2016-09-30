// Package geojson implements GeoJSON encoding and decoding.
package geojson

import (
	"encoding/json"
	"fmt"

	"github.com/twpayne/go-geom"
)

// DefaultLayout is the default layout for empty geometries.
// FIXME This should be Codec-specific, not global
var DefaultLayout = geom.XY

// ErrDimensionalityTooLow is returned when the dimensionality is too low.
type ErrDimensionalityTooLow int

func (e ErrDimensionalityTooLow) Error() string {
	return fmt.Sprintf("geojson: dimensionality too low (%d)", int(e))
}

// ErrUnsupportedType is returned when the type is unsupported.
type ErrUnsupportedType string

func (e ErrUnsupportedType) Error() string {
	return fmt.Sprintf("geojson: unsupported type: %s", string(e))
}

// A Geometry is a geometry in GeoJSON format.
type Geometry struct {
	Type        string           `json:"type"`
	Coordinates *json.RawMessage `json:"coordinates"`
}

// A Feature is a GeoJSON Feature.
type Feature struct {
	ID         string
	Geometry   geom.T
	Properties map[string]interface{}
}

type geojsonFeature struct {
	Type       string                 `json:"type,omitempty"`
	ID         string                 `json:"id,omitempty"`
	Geometry   *Geometry              `json:"geometry,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// A FeatureCollection is a GeoJSON FeatureCollection.
type FeatureCollection struct {
	Features []*Feature
}

type geojsonFeatureCollection struct {
	Type     string     `json:"type,omitempty"`
	Features []*Feature `json:"features,omitempty"`
}

func guessLayout0(coords0 []float64) (geom.Layout, error) {
	switch n := len(coords0); n {
	case 0, 1:
		return geom.NoLayout, ErrDimensionalityTooLow(len(coords0))
	case 2:
		return geom.XY, nil
	case 3:
		return geom.XYZ, nil
	case 4:
		return geom.XYZM, nil
	default:
		return geom.Layout(n), nil
	}
}

func guessLayout1(coords1 []geom.Coord) (geom.Layout, error) {
	if len(coords1) == 0 {
		return DefaultLayout, nil
	}
	return guessLayout0(coords1[0])
}

func guessLayout2(coords2 [][]geom.Coord) (geom.Layout, error) {
	if len(coords2) == 0 {
		return DefaultLayout, nil
	}
	return guessLayout1(coords2[0])
}

func guessLayout3(coords3 [][][]geom.Coord) (geom.Layout, error) {
	if len(coords3) == 0 {
		return DefaultLayout, nil
	}
	return guessLayout2(coords3[0])
}

// Decode decodes g to a geometry.
func (g *Geometry) Decode() (geom.T, error) {
	switch g.Type {
	case "Point":
		var coords geom.Coord
		if err := json.Unmarshal(*g.Coordinates, &coords); err != nil {
			return nil, err
		}
		layout, err := guessLayout0(coords)
		if err != nil {
			return nil, err
		}
		return geom.NewPoint(layout).SetCoords(coords)
	case "LineString":
		var coords []geom.Coord
		if err := json.Unmarshal(*g.Coordinates, &coords); err != nil {
			return nil, err
		}
		layout, err := guessLayout1(coords)
		if err != nil {
			return nil, err
		}
		return geom.NewLineString(layout).SetCoords(coords)
	case "Polygon":
		var coords [][]geom.Coord
		if err := json.Unmarshal(*g.Coordinates, &coords); err != nil {
			return nil, err
		}
		layout, err := guessLayout2(coords)
		if err != nil {
			return nil, err
		}
		return geom.NewPolygon(layout).SetCoords(coords)
	case "MultiPoint":
		var coords []geom.Coord
		if err := json.Unmarshal(*g.Coordinates, &coords); err != nil {
			return nil, err
		}
		layout, err := guessLayout1(coords)
		if err != nil {
			return nil, err
		}
		return geom.NewMultiPoint(layout).SetCoords(coords)
	case "MultiLineString":
		var coords [][]geom.Coord
		if err := json.Unmarshal(*g.Coordinates, &coords); err != nil {
			return nil, err
		}
		layout, err := guessLayout2(coords)
		if err != nil {
			return nil, err
		}
		return geom.NewMultiLineString(layout).SetCoords(coords)
	case "MultiPolygon":
		var coords [][][]geom.Coord
		if err := json.Unmarshal(*g.Coordinates, &coords); err != nil {
			return nil, err
		}
		layout, err := guessLayout3(coords)
		if err != nil {
			return nil, err
		}
		return geom.NewMultiPolygon(layout).SetCoords(coords)
	default:
		return nil, ErrUnsupportedType(g.Type)
	}
}

// Encode encodes g as a GeoJSON geometry.
func Encode(g geom.T) (*Geometry, error) {

	switch g := g.(type) {
	case *geom.Point:
		var coords json.RawMessage
		coords, err := json.Marshal(g.Coords())
		if err != nil {
			return nil, err
		}
		return &Geometry{
			Type:        "Point",
			Coordinates: &coords,
		}, nil
	case *geom.LineString:
		var coords json.RawMessage

		coords, err := json.Marshal(g.Coords())
		if err != nil {
			return nil, err
		}
		return &Geometry{
			Type:        "LineString",
			Coordinates: &coords,
		}, nil
	case *geom.Polygon:
		var coords json.RawMessage
		coords, err := json.Marshal(g.Coords())
		if err != nil {
			return nil, err
		}
		return &Geometry{
			Type:        "Polygon",
			Coordinates: &coords,
		}, nil
	case *geom.MultiPoint:
		var coords json.RawMessage
		coords, err := json.Marshal(g.Coords())
		if err != nil {
			return nil, err
		}
		return &Geometry{
			Type:        "MultiPoint",
			Coordinates: &coords,
		}, nil
	case *geom.MultiLineString:
		var coords json.RawMessage
		coords, err := json.Marshal(g.Coords())
		if err != nil {
			return nil, err
		}
		return &Geometry{
			Type:        "MultiLineString",
			Coordinates: &coords,
		}, nil
	case *geom.MultiPolygon:
		var coords json.RawMessage
		coords, err := json.Marshal(g.Coords())
		if err != nil {
			return nil, err
		}
		return &Geometry{
			Type:        "MultiPolygon",
			Coordinates: &coords,
		}, nil
	default:
		return nil, geom.ErrUnsupportedType{Value: g}
	}
}

// Marshal marshals an arbitrary geometry to a []byte.
func Marshal(g geom.T) ([]byte, error) {
	geojson, err := Encode(g)
	if err != nil {
		return nil, err
	}
	return json.Marshal(geojson)
}

func unmarshalCoords0(data []byte) (geom.Layout, geom.Coord, error) {
	var coords geom.Coord
	if err := json.Unmarshal(data, &coords); err != nil {
		return geom.NoLayout, nil, err
	}
	layout, err := guessLayout0(coords)
	if err != nil {
		return geom.NoLayout, nil, err
	}
	return layout, coords, nil
}

func unmarshalCoords1(data []byte) (geom.Layout, []geom.Coord, error) {
	var coords []geom.Coord
	if err := json.Unmarshal(data, &coords); err != nil {
		return geom.NoLayout, nil, err
	}
	layout, err := guessLayout1(coords)
	if err != nil {
		return geom.NoLayout, nil, err
	}
	return layout, coords, nil
}

func unmarshalCoords2(data []byte) (geom.Layout, [][]geom.Coord, error) {
	var coords [][]geom.Coord
	if err := json.Unmarshal(data, &coords); err != nil {
		return geom.NoLayout, nil, err
	}
	layout, err := guessLayout2(coords)
	if err != nil {
		return geom.NoLayout, nil, err
	}
	return layout, coords, nil
}

func unmarshalCoords3(data []byte) (geom.Layout, [][][]geom.Coord, error) {
	var coords [][][]geom.Coord
	if err := json.Unmarshal(data, &coords); err != nil {
		return geom.NoLayout, nil, err
	}
	layout, err := guessLayout3(coords)
	if err != nil {
		return geom.NoLayout, nil, err
	}
	return layout, coords, nil
}

// Unmarshal unmarshalls a []byte to an arbitrary geometry.
func Unmarshal(data []byte, g *geom.T) error {
	gg := &Geometry{}
	if err := json.Unmarshal(data, gg); err != nil {
		return err
	}
	switch gg.Type {
	case "Point":
		layout, coords, err := unmarshalCoords0(*gg.Coordinates)
		if err != nil {
			return err
		}
		*g = geom.NewPoint(layout).MustSetCoords(coords)
		return nil
	case "LineString":
		layout, coords, err := unmarshalCoords1(*gg.Coordinates)
		if err != nil {
			return err
		}
		*g = geom.NewLineString(layout).MustSetCoords(coords)
		return nil
	case "Polygon":
		layout, coords, err := unmarshalCoords2(*gg.Coordinates)
		if err != nil {
			return err
		}
		*g = geom.NewPolygon(layout).MustSetCoords(coords)
		return nil
	case "MultiPoint":
		layout, coords, err := unmarshalCoords1(*gg.Coordinates)
		if err != nil {
			return err
		}
		*g = geom.NewMultiPoint(layout).MustSetCoords(coords)
		return nil
	case "MultiLineString":
		layout, coords, err := unmarshalCoords2(*gg.Coordinates)
		if err != nil {
			return err
		}
		*g = geom.NewMultiLineString(layout).MustSetCoords(coords)
		return nil
	case "MultiPolygon":
		layout, coords, err := unmarshalCoords3(*gg.Coordinates)
		if err != nil {
			return err
		}
		*g = geom.NewMultiPolygon(layout).MustSetCoords(coords)
		return nil
	default:
		return ErrUnsupportedType(gg.Type)
	}
}

// MarshalJSON implements json.Marshaler.MarshalJSON.
func (f *Feature) MarshalJSON() ([]byte, error) {
	geometry, err := Encode(f.Geometry)
	if err != nil {
		return nil, err
	}
	return json.Marshal(&geojsonFeature{
		ID:         f.ID,
		Type:       "Feature",
		Geometry:   geometry,
		Properties: f.Properties,
	})
}

// UnmarshalJSON implements json.Unmarshaler.UnmarshalJSON.
func (f *Feature) UnmarshalJSON(data []byte) error {
	var gf geojsonFeature
	if err := json.Unmarshal(data, &gf); err != nil {
		return err
	}
	if gf.Type != "Feature" {
		return ErrUnsupportedType(gf.Type)
	}
	f.ID = gf.ID
	var err error
	f.Geometry, err = gf.Geometry.Decode()
	if err != nil {
		return err
	}
	f.Properties = gf.Properties
	return nil
}

// MarshalJSON implements json.Marshaler.MarshalJSON.
func (fc *FeatureCollection) MarshalJSON() ([]byte, error) {
	return json.Marshal(&geojsonFeatureCollection{
		Type:     "FeatureCollection",
		Features: fc.Features,
	})
}
