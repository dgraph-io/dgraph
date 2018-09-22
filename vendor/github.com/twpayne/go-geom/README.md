# go-geom

[![Build Status](https://travis-ci.org/twpayne/go-geom.svg?branch=master)](https://travis-ci.org/twpayne/go-geom)
[![GoDoc](https://godoc.org/github.com/twpayne/go-geom?status.svg)](https://godoc.org/github.com/twpayne/go-geom)

Package geom implements efficient geometry types.

Encoding and decoding:

 * [GeoJSON](https://godoc.org/github.com/twpayne/go-geom/encoding/geojson)
 * [IGC](https://godoc.org/github.com/twpayne/go-geom/encoding/igc)
 * [KML](https://godoc.org/github.com/twpayne/go-geom/encoding/kml)
 * [WKB](https://godoc.org/github.com/twpayne/go-geom/encoding/wkb)
 * [EWKB](https://godoc.org/github.com/twpayne/go-geom/encoding/ewkb)
 * [WKT](https://godoc.org/github.com/twpayne/go-geom/encoding/wkt) (encoding only)
 * [WKB Hex](https://godoc.org/github.com/twpayne/go-geom/encoding/wkbhex)
 * [EWKB Hex](https://godoc.org/github.com/twpayne/go-geom/encoding/ewkbhex)

Geometry functions:

 * [XY](https://godoc.org/github.com/twpayne/go-geom/xy) 2D geometry functions
 * [XYZ](https://godoc.org/github.com/twpayne/go-geom/xyz) 3D geometry functions

Example:

```go
func ExampleNewPolygon() {
	unitSquare := NewPolygon(XY).MustSetCoords([][]Coord{
		{{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0, 0}},
	})
	fmt.Printf("unitSquare.Area() == %f", unitSquare.Area())
	// Output: unitSquare.Area() == 1.000000
}
```

[License](LICENSE)
