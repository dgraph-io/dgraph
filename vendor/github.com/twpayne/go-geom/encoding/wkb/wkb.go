// Package wkb implements Well Known Binary encoding and decoding.
package wkb

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkbcommon"
)

var (
	// XDR is big endian.
	XDR = wkbcommon.XDR
	// NDR is little endian.
	NDR = wkbcommon.NDR
)

const (
	wkbXYID   = 0
	wkbXYZID  = 1000
	wkbXYMID  = 2000
	wkbXYZMID = 3000
)

// Read reads an arbitrary geometry from r.
func Read(r io.Reader) (geom.T, error) {

	var wkbByteOrder, err = wkbcommon.ReadByte(r)
	if err != nil {
		return nil, err
	}
	var byteOrder binary.ByteOrder
	switch wkbByteOrder {
	case wkbcommon.XDRID:
		byteOrder = XDR
	case wkbcommon.NDRID:
		byteOrder = NDR
	default:
		return nil, wkbcommon.ErrUnknownByteOrder(wkbByteOrder)
	}

	wkbGeometryType, err := wkbcommon.ReadUInt32(r, byteOrder)
	if err != nil {
		return nil, err
	}
	t := wkbcommon.Type(wkbGeometryType)

	layout := geom.NoLayout
	switch 1000 * (t / 1000) {
	case wkbXYID:
		layout = geom.XY
	case wkbXYZID:
		layout = geom.XYZ
	case wkbXYMID:
		layout = geom.XYM
	case wkbXYZMID:
		layout = geom.XYZM
	default:
		return nil, wkbcommon.ErrUnknownType(t)
	}

	switch t % 1000 {
	case wkbcommon.PointID:
		flatCoords, err := wkbcommon.ReadFlatCoords0(r, byteOrder, layout.Stride())
		if err != nil {
			return nil, err
		}
		return geom.NewPointFlat(layout, flatCoords), nil
	case wkbcommon.LineStringID:
		flatCoords, err := wkbcommon.ReadFlatCoords1(r, byteOrder, layout.Stride())
		if err != nil {
			return nil, err
		}
		return geom.NewLineStringFlat(layout, flatCoords), nil
	case wkbcommon.PolygonID:
		flatCoords, ends, err := wkbcommon.ReadFlatCoords2(r, byteOrder, layout.Stride())
		if err != nil {
			return nil, err
		}
		return geom.NewPolygonFlat(layout, flatCoords, ends), nil
	case wkbcommon.MultiPointID:
		n, err := wkbcommon.ReadUInt32(r, byteOrder)
		if err != nil {
			return nil, err
		}
		if n > wkbcommon.MaxGeometryElements[1] {
			return nil, wkbcommon.ErrGeometryTooLarge{Level: 1, N: n, Limit: wkbcommon.MaxGeometryElements[1]}
		}
		mp := geom.NewMultiPoint(layout)
		for i := uint32(0); i < n; i++ {
			g, err := Read(r)
			if err != nil {
				return nil, err
			}
			p, ok := g.(*geom.Point)
			if !ok {
				return nil, wkbcommon.ErrUnexpectedType{Got: g, Want: &geom.Point{}}
			}
			if err = mp.Push(p); err != nil {
				return nil, err
			}
		}
		return mp, nil
	case wkbcommon.MultiLineStringID:
		n, err := wkbcommon.ReadUInt32(r, byteOrder)
		if err != nil {
			return nil, err
		}
		if n > wkbcommon.MaxGeometryElements[2] {
			return nil, wkbcommon.ErrGeometryTooLarge{Level: 2, N: n, Limit: wkbcommon.MaxGeometryElements[2]}
		}
		mls := geom.NewMultiLineString(layout)
		for i := uint32(0); i < n; i++ {
			g, err := Read(r)
			if err != nil {
				return nil, err
			}
			p, ok := g.(*geom.LineString)
			if !ok {
				return nil, wkbcommon.ErrUnexpectedType{Got: g, Want: &geom.LineString{}}
			}
			if err = mls.Push(p); err != nil {
				return nil, err
			}
		}
		return mls, nil
	case wkbcommon.MultiPolygonID:
		n, err := wkbcommon.ReadUInt32(r, byteOrder)
		if err != nil {
			return nil, err
		}
		if n > wkbcommon.MaxGeometryElements[3] {
			return nil, wkbcommon.ErrGeometryTooLarge{Level: 3, N: n, Limit: wkbcommon.MaxGeometryElements[3]}
		}
		mp := geom.NewMultiPolygon(layout)
		for i := uint32(0); i < n; i++ {
			g, err := Read(r)
			if err != nil {
				return nil, err
			}
			p, ok := g.(*geom.Polygon)
			if !ok {
				return nil, wkbcommon.ErrUnexpectedType{Got: g, Want: &geom.Polygon{}}
			}
			if err = mp.Push(p); err != nil {
				return nil, err
			}
		}
		return mp, nil
	default:
		return nil, wkbcommon.ErrUnsupportedType(wkbGeometryType)
	}

}

// Unmarshal unmrshals an arbitrary geometry from a []byte.
func Unmarshal(data []byte) (geom.T, error) {
	return Read(bytes.NewBuffer(data))
}

// Write writes an arbitrary geometry to w.
func Write(w io.Writer, byteOrder binary.ByteOrder, g geom.T) error {

	var wkbByteOrder byte
	switch byteOrder {
	case XDR:
		wkbByteOrder = wkbcommon.XDRID
	case NDR:
		wkbByteOrder = wkbcommon.NDRID
	default:
		return wkbcommon.ErrUnsupportedByteOrder{}
	}
	if err := wkbcommon.WriteByte(w, wkbByteOrder); err != nil {
		return err
	}

	var wkbGeometryType uint32
	switch g.(type) {
	case *geom.Point:
		wkbGeometryType = wkbcommon.PointID
	case *geom.LineString:
		wkbGeometryType = wkbcommon.LineStringID
	case *geom.Polygon:
		wkbGeometryType = wkbcommon.PolygonID
	case *geom.MultiPoint:
		wkbGeometryType = wkbcommon.MultiPointID
	case *geom.MultiLineString:
		wkbGeometryType = wkbcommon.MultiLineStringID
	case *geom.MultiPolygon:
		wkbGeometryType = wkbcommon.MultiPolygonID
	default:
		return geom.ErrUnsupportedType{Value: g}
	}
	switch g.Layout() {
	case geom.XY:
		wkbGeometryType += wkbXYID
	case geom.XYZ:
		wkbGeometryType += wkbXYZID
	case geom.XYM:
		wkbGeometryType += wkbXYMID
	case geom.XYZM:
		wkbGeometryType += wkbXYZMID
	default:
		return geom.ErrUnsupportedLayout(g.Layout())
	}
	if err := wkbcommon.WriteUInt32(w, byteOrder, wkbGeometryType); err != nil {
		return err
	}

	switch g.(type) {
	case *geom.Point:
		return wkbcommon.WriteFlatCoords0(w, byteOrder, g.FlatCoords())
	case *geom.LineString:
		return wkbcommon.WriteFlatCoords1(w, byteOrder, g.FlatCoords(), g.Stride())
	case *geom.Polygon:
		return wkbcommon.WriteFlatCoords2(w, byteOrder, g.FlatCoords(), g.Ends(), g.Stride())
	case *geom.MultiPoint:
		mp := g.(*geom.MultiPoint)
		n := mp.NumPoints()
		if err := wkbcommon.WriteUInt32(w, byteOrder, uint32(n)); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if err := Write(w, byteOrder, mp.Point(i)); err != nil {
				return err
			}
		}
		return nil
	case *geom.MultiLineString:
		mls := g.(*geom.MultiLineString)
		n := mls.NumLineStrings()
		if err := wkbcommon.WriteUInt32(w, byteOrder, uint32(n)); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if err := Write(w, byteOrder, mls.LineString(i)); err != nil {
				return err
			}
		}
		return nil
	case *geom.MultiPolygon:
		mp := g.(*geom.MultiPolygon)
		n := mp.NumPolygons()
		if err := wkbcommon.WriteUInt32(w, byteOrder, uint32(n)); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if err := Write(w, byteOrder, mp.Polygon(i)); err != nil {
				return err
			}
		}
		return nil
	default:
		return geom.ErrUnsupportedType{Value: g}
	}

}

// Marshal marshals an arbitrary geometry to a []byte.
func Marshal(g geom.T, byteOrder binary.ByteOrder) ([]byte, error) {
	w := bytes.NewBuffer(nil)
	if err := Write(w, byteOrder, g); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}
