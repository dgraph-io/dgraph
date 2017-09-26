/*
Package geojson is a library for encoding and decoding GeoJSON into Go structs.
Supports both the json.Marshaler and json.Unmarshaler interfaces as well as helper functions
such as `UnmarshalFeatureCollection`, `UnmarshalFeature` and `UnmarshalGeometry`.
*/
package geojson

import (
	"encoding/json"
)

// A FeatureCollection correlates to a GeoJSON feature collection.
type FeatureCollection struct {
	Type        string                 `json:"type"`
	BoundingBox []float64              `json:"bbox,omitempty"`
	Features    []*Feature             `json:"features"`
	CRS         map[string]interface{} `json:"crs,omitempty"` // Coordinate Reference System Objects are not currently supported
}

// NewFeatureCollection creates and initializes a new feature collection.
func NewFeatureCollection() *FeatureCollection {
	return &FeatureCollection{
		Type:     "FeatureCollection",
		Features: make([]*Feature, 0),
	}
}

// AddFeature appends a feature to the collection.
func (fc *FeatureCollection) AddFeature(feature *Feature) *FeatureCollection {
	fc.Features = append(fc.Features, feature)
	return fc
}

// MarshalJSON converts the feature collection object into the proper JSON.
// It will handle the encoding of all the child features and geometries.
// Alternately one can call json.Marshal(fc) directly for the same result.
func (fc *FeatureCollection) MarshalJSON() ([]byte, error) {
	fc.Type = "FeatureCollection"
	if fc.Features == nil {
		fc.Features = make([]*Feature, 0) // GeoJSON requires the feature attribute to be at least []
	}
	return json.Marshal(*fc)
}

// UnmarshalFeatureCollection decodes the data into a GeoJSON feature collection.
// Alternately one can call json.Unmarshal(fc) directly for the same result.
func UnmarshalFeatureCollection(data []byte) (*FeatureCollection, error) {
	fc := &FeatureCollection{}
	err := json.Unmarshal(data, fc)
	if err != nil {
		return nil, err
	}

	return fc, nil
}
