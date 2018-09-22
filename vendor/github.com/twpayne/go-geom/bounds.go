package geom

// FIXME(twpayne) creating a Bounds with layout XYM and then extending it with
// a XYZ geometry will not work.

import (
	"math"
)

// A Bounds represents a multi-dimensional bounding box.
type Bounds struct {
	layout Layout
	min    Coord
	max    Coord
}

// NewBounds creates a new Bounds.
func NewBounds(layout Layout) *Bounds {
	stride := layout.Stride()
	min, max := make(Coord, stride), make(Coord, stride)
	for i := 0; i < stride; i++ {
		min[i], max[i] = math.Inf(1), math.Inf(-1)
	}
	return &Bounds{
		layout: layout,
		min:    min,
		max:    max,
	}
}

// Extend extends b to include geometry g.
func (b *Bounds) Extend(g T) *Bounds {
	b.extendStride(g.Layout().Stride())
	b.extendFlatCoords(g.FlatCoords(), 0, len(g.FlatCoords()), g.Stride())
	return b
}

// IsEmpty returns true if b is empty.
func (b *Bounds) IsEmpty() bool {
	for i, stride := 0, b.layout.Stride(); i < stride; i++ {
		if b.max[i] < b.min[i] {
			return true
		}
	}
	return false
}

// Layout returns b's layout.
func (b *Bounds) Layout() Layout {
	return b.layout
}

// Max returns the maximum value in dimension dim.
func (b *Bounds) Max(dim int) float64 {
	return b.max[dim]
}

// Min returns the minimum value in dimension dim.
func (b *Bounds) Min(dim int) float64 {
	return b.min[dim]
}

// Overlaps returns true if b overlaps b2 in layout.
func (b *Bounds) Overlaps(layout Layout, b2 *Bounds) bool {
	for i, stride := 0, layout.Stride(); i < stride; i++ {
		if b.min[i] > b2.max[i] || b.max[i] < b2.min[i] {
			return false
		}
	}
	return true
}

// Set sets the minimum and maximum values. args must be an even number of
// values: the first half are the minimum values for each dimension and the
// second half are the maximum values for each dimension.
func (b *Bounds) Set(args ...float64) *Bounds {
	if len(args)&1 != 0 {
		panic("geom: even number of arguments required")
	}
	stride := len(args) / 2
	b.extendStride(stride)
	for i := 0; i < stride; i++ {
		b.min[i], b.max[i] = args[i], args[i+stride]
	}
	return b
}

// SetCoords sets the minimum and maximum values of the Bounds.
func (b *Bounds) SetCoords(min, max Coord) *Bounds {
	b.min = Coord(make([]float64, b.layout.Stride()))
	b.max = Coord(make([]float64, b.layout.Stride()))

	for i := 0; i < b.layout.Stride(); i++ {
		b.min[i] = math.Min(min[i], max[i])
		b.max[i] = math.Max(min[i], max[i])
	}

	return b
}

// OverlapsPoint determines if the bounding box overlaps the point (point is within or on the border of the bounds)
func (b *Bounds) OverlapsPoint(layout Layout, point Coord) bool {
	for i, stride := 0, layout.Stride(); i < stride; i++ {
		if b.min[i] > point[i] || b.max[i] < point[i] {
			return false
		}
	}
	return true
}

func (b *Bounds) extendFlatCoords(flatCoords []float64, offset, end, stride int) *Bounds {
	b.extendStride(stride)
	for i := offset; i < end; i += stride {
		for j := 0; j < stride; j++ {
			b.min[j] = math.Min(b.min[j], flatCoords[i+j])
			b.max[j] = math.Max(b.max[j], flatCoords[i+j])
		}
	}
	return b
}

func (b *Bounds) extendStride(stride int) {
	for b.layout.Stride() < stride {
		b.min = append(b.min, math.Inf(1))
		b.max = append(b.max, math.Inf(-1))
		b.layout++
	}
}
