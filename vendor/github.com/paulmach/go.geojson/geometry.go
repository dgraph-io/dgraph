package geojson

import (
	"encoding/json"
	"errors"
	"fmt"
)

// A GeometryType serves to enumerate the different GeoJSON geometry types.
type GeometryType string

// The geometry types supported by GeoJSON 1.0
const (
	GeometryPoint           GeometryType = "Point"
	GeometryMultiPoint      GeometryType = "MultiPoint"
	GeometryLineString      GeometryType = "LineString"
	GeometryMultiLineString GeometryType = "MultiLineString"
	GeometryPolygon         GeometryType = "Polygon"
	GeometryMultiPolygon    GeometryType = "MultiPolygon"
	GeometryCollection      GeometryType = "GeometryCollection"
)

// A Geometry correlates to a GeoJSON geometry object.
type Geometry struct {
	Type            GeometryType `json:"type"`
	BoundingBox     []float64    `json:"bbox,omitempty"`
	Point           []float64
	MultiPoint      [][]float64
	LineString      [][]float64
	MultiLineString [][][]float64
	Polygon         [][][]float64
	MultiPolygon    [][][][]float64
	Geometries      []*Geometry
	CRS             map[string]interface{} `json:"crs,omitempty"` // Coordinate Reference System Objects are not currently supported
}

// NewPointGeometry creates and initializes a point geometry with the give coordinate.
func NewPointGeometry(coordinate []float64) *Geometry {
	return &Geometry{
		Type:  GeometryPoint,
		Point: coordinate,
	}
}

// NewMultiPointGeometry creates and initializes a multi-point geometry with the given coordinates.
func NewMultiPointGeometry(coordinates ...[]float64) *Geometry {
	return &Geometry{
		Type:       GeometryMultiPoint,
		MultiPoint: coordinates,
	}
}

// NewLineStringGeometry creates and initializes a line string geometry with the given coordinates.
func NewLineStringGeometry(coordinates [][]float64) *Geometry {
	return &Geometry{
		Type:       GeometryLineString,
		LineString: coordinates,
	}
}

// NewMultiLineStringGeometry creates and initializes a multi-line string geometry with the given lines.
func NewMultiLineStringGeometry(lines ...[][]float64) *Geometry {
	return &Geometry{
		Type:            GeometryMultiLineString,
		MultiLineString: lines,
	}
}

// NewPolygonGeometry creates and initializes a polygon geometry with the given polygon.
func NewPolygonGeometry(polygon [][][]float64) *Geometry {
	return &Geometry{
		Type:    GeometryPolygon,
		Polygon: polygon,
	}
}

// NewMultiPolygonGeometry creates and initializes a multi-polygon geometry with the given polygons.
func NewMultiPolygonGeometry(polygons ...[][][]float64) *Geometry {
	return &Geometry{
		Type:         GeometryMultiPolygon,
		MultiPolygon: polygons,
	}
}

// NewCollectionGeometry creates and initializes a geometry collection geometry with the given geometries.
func NewCollectionGeometry(geometries ...*Geometry) *Geometry {
	return &Geometry{
		Type:       GeometryCollection,
		Geometries: geometries,
	}
}

// MarshalJSON converts the geometry object into the correct JSON.
// This fulfills the json.Marshaler interface.
func (g *Geometry) MarshalJSON() ([]byte, error) {
	// defining a struct here lets us define the order of the JSON elements.
	type geometry struct {
		Type        GeometryType           `json:"type"`
		BoundingBox []float64              `json:"bbox,omitempty"`
		Coordinates interface{}            `json:"coordinates,omitempty"`
		Geometries  interface{}            `json:"geometries,omitempty"`
		CRS         map[string]interface{} `json:"crs,omitempty"`
	}

	geo := &geometry{
		Type: g.Type,
	}

	if g.BoundingBox != nil && len(g.BoundingBox) != 0 {
		geo.BoundingBox = g.BoundingBox
	}

	switch g.Type {
	case GeometryPoint:
		geo.Coordinates = g.Point
	case GeometryMultiPoint:
		geo.Coordinates = g.MultiPoint
	case GeometryLineString:
		geo.Coordinates = g.LineString
	case GeometryMultiLineString:
		geo.Coordinates = g.MultiLineString
	case GeometryPolygon:
		geo.Coordinates = g.Polygon
	case GeometryMultiPolygon:
		geo.Coordinates = g.MultiPolygon
	case GeometryCollection:
		geo.Geometries = g.Geometries
	}

	return json.Marshal(geo)
}

// UnmarshalGeometry decodes the data into a GeoJSON geometry.
// Alternately one can call json.Unmarshal(g) directly for the same result.
func UnmarshalGeometry(data []byte) (*Geometry, error) {
	g := &Geometry{}
	err := json.Unmarshal(data, g)
	if err != nil {
		return nil, err
	}

	return g, nil
}

// UnmarshalJSON decodes the data into a GeoJSON geometry.
// This fulfills the json.Unmarshaler interface.
func (g *Geometry) UnmarshalJSON(data []byte) error {
	var object map[string]interface{}
	err := json.Unmarshal(data, &object)
	if err != nil {
		return err
	}

	return decodeGeometry(g, object)
}

// Scan implements the sql.Scanner interface allowing
// geometry structs to be passed into rows.Scan(...interface{})
// The columns must be received as GeoJSON Geometry.
// When using PostGIS a spatial column would need to be wrapped in ST_AsGeoJSON.
func (g *Geometry) Scan(value interface{}) error {
	var data []byte

	switch value.(type) {
	case string:
		data = []byte(value.(string))
	case []byte:
		data = value.([]byte)
	default:
		return errors.New("unable to parse this type into geojson")
	}

	return g.UnmarshalJSON(data)
}

func decodeGeometry(g *Geometry, object map[string]interface{}) error {
	t, ok := object["type"]
	if !ok {
		return errors.New("type property not defined")
	}

	if s, ok := t.(string); ok {
		g.Type = GeometryType(s)
	} else {
		return errors.New("type property not string")
	}

	var err error
	switch g.Type {
	case GeometryPoint:
		g.Point, err = decodePosition(object["coordinates"])
	case GeometryMultiPoint:
		g.MultiPoint, err = decodePositionSet(object["coordinates"])
	case GeometryLineString:
		g.LineString, err = decodePositionSet(object["coordinates"])
	case GeometryMultiLineString:
		g.MultiLineString, err = decodePathSet(object["coordinates"])
	case GeometryPolygon:
		g.Polygon, err = decodePathSet(object["coordinates"])
	case GeometryMultiPolygon:
		g.MultiPolygon, err = decodePolygonSet(object["coordinates"])
	case GeometryCollection:
		g.Geometries, err = decodeGeometries(object["geometries"])
	}

	return err
}

func decodePosition(data interface{}) ([]float64, error) {
	coords, ok := data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("not a valid position, got %v", data)
	}

	result := make([]float64, 0, len(coords))
	for _, coord := range coords {
		if f, ok := coord.(float64); ok {
			result = append(result, f)
		} else {
			return nil, fmt.Errorf("not a valid coordinate, got %v", coord)
		}
	}

	return result, nil
}

func decodePositionSet(data interface{}) ([][]float64, error) {
	points, ok := data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("not a valid set of positions, got %v", data)
	}

	result := make([][]float64, 0, len(points))
	for _, point := range points {
		if p, err := decodePosition(point); err == nil {
			result = append(result, p)
		} else {
			return nil, err
		}
	}

	return result, nil
}

func decodePathSet(data interface{}) ([][][]float64, error) {
	sets, ok := data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("not a valid path, got %v", data)
	}

	result := make([][][]float64, 0, len(sets))

	for _, set := range sets {
		if s, err := decodePositionSet(set); err == nil {
			result = append(result, s)
		} else {
			return nil, err
		}
	}

	return result, nil
}

func decodePolygonSet(data interface{}) ([][][][]float64, error) {
	polygons, ok := data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("not a valid polygon, got %v", data)
	}

	result := make([][][][]float64, 0, len(polygons))
	for _, polygon := range polygons {
		if p, err := decodePathSet(polygon); err == nil {
			result = append(result, p)
		} else {
			return nil, err
		}
	}

	return result, nil
}

func decodeGeometries(data interface{}) ([]*Geometry, error) {
	if vs, ok := data.([]interface{}); ok {
		geometries := make([]*Geometry, 0, len(vs))
		for _, v := range vs {
			g := &Geometry{}

			vmap, ok := v.(map[string]interface{})
			if !ok {
				break
			}

			err := decodeGeometry(g, vmap)
			if err != nil {
				return nil, err
			}

			geometries = append(geometries, g)
		}

		if len(geometries) == len(vs) {
			return geometries, nil
		}
	}

	return nil, fmt.Errorf("not a valid set of geometries, got %v", data)
}

// IsPoint returns true with the geometry object is a Point type.
func (g *Geometry) IsPoint() bool {
	return g.Type == GeometryPoint
}

// IsMultiPoint returns true with the geometry object is a MultiPoint type.
func (g *Geometry) IsMultiPoint() bool {
	return g.Type == GeometryMultiPoint
}

// IsLineString returns true with the geometry object is a LineString type.
func (g *Geometry) IsLineString() bool {
	return g.Type == GeometryLineString
}

// IsMultiLineString returns true with the geometry object is a LineString type.
func (g *Geometry) IsMultiLineString() bool {
	return g.Type == GeometryMultiLineString
}

// IsPolygon returns true with the geometry object is a Polygon type.
func (g *Geometry) IsPolygon() bool {
	return g.Type == GeometryPolygon
}

// IsMultiPolygon returns true with the geometry object is a MultiPolygon type.
func (g *Geometry) IsMultiPolygon() bool {
	return g.Type == GeometryMultiPolygon
}

// IsCollection returns true with the geometry object is a GeometryCollection type.
func (g *Geometry) IsCollection() bool {
	return g.Type == GeometryCollection
}
