package wkb

import (
	"fmt"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkbcommon"
)

// ErrExpectedByteSlice is returned when a []byte is expected.
type ErrExpectedByteSlice struct {
	Value interface{}
}

func (e ErrExpectedByteSlice) Error() string {
	return fmt.Sprintf("wkb: want []byte, got %T", e.Value)
}

// A Point is a WKB-encoded Point.
type Point struct {
	geom.Point
}

// A LineString is a WKB-encoded LineString.
type LineString struct {
	geom.LineString
}

// A Polygon is a WKB-encoded Polygon.
type Polygon struct {
	geom.Polygon
}

// A MultiPoint is a WKB-encoded MultiPoint.
type MultiPoint struct {
	geom.MultiPoint
}

// A MultiLineString is a WKB-encoded MultiLineString.
type MultiLineString struct {
	geom.MultiLineString
}

// A MultiPolygon is a WKB-encoded MultiPolygon.
type MultiPolygon struct {
	geom.MultiPolygon
}

// Scan scans from a []byte.
func (p *Point) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return ErrExpectedByteSlice{Value: src}
	}
	got, err := Unmarshal(b)
	if err != nil {
		return err
	}
	p1, ok := got.(*geom.Point)
	if !ok {
		return wkbcommon.ErrUnexpectedType{Got: p1, Want: p}
	}
	p.Swap(p1)
	return nil
}

// Scan scans from a []byte.
func (ls *LineString) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return ErrExpectedByteSlice{Value: src}
	}
	got, err := Unmarshal(b)
	if err != nil {
		return err
	}
	p1, ok := got.(*geom.LineString)
	if !ok {
		return wkbcommon.ErrUnexpectedType{Got: p1, Want: ls}
	}
	ls.Swap(p1)
	return nil
}

// Scan scans from a []byte.
func (p *Polygon) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return ErrExpectedByteSlice{Value: src}
	}
	got, err := Unmarshal(b)
	if err != nil {
		return err
	}
	p1, ok := got.(*geom.Polygon)
	if !ok {
		return wkbcommon.ErrUnexpectedType{Got: p1, Want: p}
	}
	p.Swap(p1)
	return nil
}

// Scan scans from a []byte.
func (mp *MultiPoint) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return ErrExpectedByteSlice{Value: src}
	}
	got, err := Unmarshal(b)
	if err != nil {
		return err
	}
	mp1, ok := got.(*geom.MultiPoint)
	if !ok {
		return wkbcommon.ErrUnexpectedType{Got: mp1, Want: mp}
	}
	mp.Swap(mp1)
	return nil
}

// Scan scans from a []byte.
func (mls *MultiLineString) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return ErrExpectedByteSlice{Value: src}
	}
	got, err := Unmarshal(b)
	if err != nil {
		return err
	}
	mls1, ok := got.(*geom.MultiLineString)
	if !ok {
		return wkbcommon.ErrUnexpectedType{Got: mls1, Want: mls}
	}
	mls.Swap(mls1)
	return nil
}

// Scan scans from a []byte.
func (mp *MultiPolygon) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return ErrExpectedByteSlice{Value: src}
	}
	got, err := Unmarshal(b)
	if err != nil {
		return err
	}
	mp1, ok := got.(*geom.MultiPolygon)
	if !ok {
		return wkbcommon.ErrUnexpectedType{Got: mp1, Want: mp}
	}
	mp.Swap(mp1)
	return nil
}
