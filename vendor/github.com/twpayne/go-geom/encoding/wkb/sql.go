package wkb

import (
	"bytes"
	"database/sql/driver"
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

// A Point is a WKB-encoded Point that implements the sql.Scanner and
// driver.Valuer interfaces.
type Point struct {
	*geom.Point
}

// A LineString is a WKB-encoded LineString that implements the sql.Scanner and
// driver.Valuer interfaces.
type LineString struct {
	*geom.LineString
}

// A Polygon is a WKB-encoded Polygon that implements the sql.Scanner and
// driver.Valuer interfaces.
type Polygon struct {
	*geom.Polygon
}

// A MultiPoint is a WKB-encoded MultiPoint that implements the sql.Scanner and
// driver.Valuer interfaces.
type MultiPoint struct {
	*geom.MultiPoint
}

// A MultiLineString is a WKB-encoded MultiLineString that implements the
// sql.Scanner and driver.Valuer interfaces.
type MultiLineString struct {
	*geom.MultiLineString
}

// A MultiPolygon is a WKB-encoded MultiPolygon that implements the sql.Scanner
// and driver.Valuer interfaces.
type MultiPolygon struct {
	*geom.MultiPolygon
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
	p.Point = p1
	return nil
}

// Value returns the WKB encoding of p.
func (p *Point) Value() (driver.Value, error) {
	return value(p.Point)
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
	ls1, ok := got.(*geom.LineString)
	if !ok {
		return wkbcommon.ErrUnexpectedType{Got: ls1, Want: ls}
	}
	ls.LineString = ls1
	return nil
}

// Value returns the WKB encoding of ls.
func (ls *LineString) Value() (driver.Value, error) {
	return value(ls.LineString)
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
	p.Polygon = p1
	return nil
}

// Value returns the WKB encoding of p.
func (p *Polygon) Value() (driver.Value, error) {
	return value(p.Polygon)
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
	mp.MultiPoint = mp1
	return nil
}

// Value returns the WKB encoding of mp.
func (mp *MultiPoint) Value() (driver.Value, error) {
	return value(mp.MultiPoint)
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
	mls.MultiLineString = mls1
	return nil
}

// Value returns the WKB encoding of mls.
func (mls *MultiLineString) Value() (driver.Value, error) {
	return value(mls.MultiLineString)
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
	mp.MultiPolygon = mp1
	return nil
}

// Value returns the WKB encoding of mp.
func (mp *MultiPolygon) Value() (driver.Value, error) {
	return value(mp.MultiPolygon)
}

func value(g geom.T) (driver.Value, error) {
	b := &bytes.Buffer{}
	if err := Write(b, NDR, g); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
