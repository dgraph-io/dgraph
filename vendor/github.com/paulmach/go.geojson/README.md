go.geojson
==========

Go.geojson is a package for **encoding and decoding** [GeoJSON](http://geojson.org/) into Go structs.
Supports both the [json.Marshaler](http://golang.org/pkg/encoding/json/#Marshaler) and [json.Unmarshaler](http://golang.org/pkg/encoding/json/#Unmarshaler)
interfaces as well as [sql.Scanner](http://golang.org/pkg/database/sql/#Scanner) for directly scanning PostGIS query results.
The package also provides helper functions such as `UnmarshalFeatureCollection`, `UnmarshalFeature` and `UnmarshalGeometry`.

#### To install
	
	go get github.com/paulmach/go.geojson

#### To use, imports as package name `geojson`:

	import "github.com/paulmach/go.geojson"

<br />
[![Build Status](https://travis-ci.org/paulmach/go.geojson.png?branch=master)](https://travis-ci.org/paulmach/go.geojson)
&nbsp; &nbsp;
[![Coverage Status](https://coveralls.io/repos/paulmach/go.geojson/badge.png?branch=master)](https://coveralls.io/r/paulmach/go.geojson?branch=master)
&nbsp; &nbsp;
[![Godoc Reference](https://godoc.org/github.com/paulmach/go.geojson?status.png)](https://godoc.org/github.com/paulmach/go.geojson)

## Examples

* #### Unmarshalling  (JSON -> Go)

	go.geojson supports both the [json.Marshaler](http://golang.org/pkg/encoding/json/#Marshaler) and [json.Unmarshaler](http://golang.org/pkg/encoding/json/#Unmarshaler) interfaces as well as helper functions such as `UnmarshalFeatureCollection`, `UnmarshalFeature` and `UnmarshalGeometry`.

		// Feature Collection
		rawFeatureJSON := []byte(`
		  { "type": "FeatureCollection",
		    "features": [
		      { "type": "Feature",
		        "geometry": {"type": "Point", "coordinates": [102.0, 0.5]},
		        "properties": {"prop0": "value0"}
		      }
		    ]
		  }`)

		fc1, err := geojson.UnmarshalFeatureCollection(rawFeatureJSON)

		fc2 := geojson.NewFeatureCollection()
		err := json.Unmarshal(rawJSON, fc2)

		// Geometry
		rawGeometryJSON := []byte(`{"type": "Point", "coordinates": [102.0, 0.5]}`)
		g, err := geojson.UnmarshalGeometry(rawGeometryJSON)

		g.IsPoint() == true
		g.Point == []float64{102.0, 0.5}
	
* #### Marshalling (Go -> JSON)

		g := geojson.NewPointGeometry([]float64{1, 2})
		rawJSON, err := g.MarshalJSON()

		fc := geojson.NewFeatureCollection()
		fc.AddFeature(geojson.NewPointFeature([]float64{1,2}))
		rawJSON, err := fc.MarshalJSON()

* #### Scanning PostGIS query results

		row := db.QueryRow("SELECT ST_AsGeoJSON(the_geom) FROM postgis_table)

		var geometry *geojson.Geometry
		row.Scan(&geometry)

* #### Dealing with different Geometry types

	A geometry can be of several types, causing problems in a statically typed language.
	Thus there is a separate attribute on Geometry for each type. 
	See the [Geometry object](https://godoc.org/github.com/paulmach/go.geojson#Geometry) for more details.

		g := geojson.UnmarshalGeometry([]byte(`
			{
	          "type": "LineString",
	          "coordinates": [
	            [102.0, 0.0], [103.0, 1.0], [104.0, 0.0], [105.0, 1.0]
	          ]
	        }`))

		switch {
		case g.IsPoint():
			// do something with g.Point
		case g.IsLineString():
			// do something with g.LineString
		}

## Feature Properties

GeoJSON [Features](http://geojson.org/geojson-spec.html#feature-objects) can have properties of any type.
This can cause issues in a statically typed language such as Go.
So, included are some helper methods on the Feature object to ease the pain.

	// functions to do the casting for you
	func (f Feature) PropertyBool(key string) (bool, error) {
	func (f Feature) PropertyInt(key string) (int, error) {
	func (f Feature) PropertyFloat64(key string) (float64, error) {
	func (f Feature) PropertyString(key string) (string, error) {

	// functions that hide the error and let you define default
	func (f Feature) PropertyMustBool(key string, def ...bool) bool {
	func (f Feature) PropertyMustInt(key string, def ...int) int {
	func (f Feature) PropertyMustFloat64(key string, def ...float64) float64 {
	func (f Feature) PropertyMustString(key string, def ...string) string {

