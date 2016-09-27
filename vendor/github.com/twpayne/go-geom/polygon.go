package geom

// A Polygon represents a polygon as a collection of LinearRings. The first
// LinearRing is the outer boundary. Subsequent LinearRings are inner
// boundaries (holes).
type Polygon struct {
	geom2
}

// NewPolygon returns a new, empty, Polygon.
func NewPolygon(layout Layout) *Polygon {
	return NewPolygonFlat(layout, nil, nil)
}

// NewPolygonFlat returns a new Polygon with the given flat coordinates.
func NewPolygonFlat(layout Layout, flatCoords []float64, ends []int) *Polygon {
	p := new(Polygon)
	p.layout = layout
	p.stride = layout.Stride()
	p.flatCoords = flatCoords
	p.ends = ends
	return p
}

// Area returns the area.
func (p *Polygon) Area() float64 {
	return doubleArea2(p.flatCoords, 0, p.ends, p.stride) / 2
}

// Clone returns a deep copy.
func (p *Polygon) Clone() *Polygon {
	flatCoords := make([]float64, len(p.flatCoords))
	copy(flatCoords, p.flatCoords)
	ends := make([]int, len(p.ends))
	copy(ends, p.ends)
	return NewPolygonFlat(p.layout, flatCoords, ends)
}

// Empty returns false.
func (p *Polygon) Empty() bool {
	return false
}

// Length returns the perimter.
func (p *Polygon) Length() float64 {
	return length2(p.flatCoords, 0, p.ends, p.stride)
}

// LinearRing returns the ith LinearRing.
func (p *Polygon) LinearRing(i int) *LinearRing {
	offset := 0
	if i > 0 {
		offset = p.ends[i-1]
	}
	return NewLinearRingFlat(p.layout, p.flatCoords[offset:p.ends[i]])
}

// MustSetCoords sets the coordinates and panics on any error.
func (p *Polygon) MustSetCoords(coords [][]Coord) *Polygon {
	Must(p.SetCoords(coords))
	return p
}

// NumLinearRings returns the number of LinearRings.
func (p *Polygon) NumLinearRings() int {
	return len(p.ends)
}

// Push appends a LinearRing.
func (p *Polygon) Push(lr *LinearRing) error {
	if lr.layout != p.layout {
		return ErrLayoutMismatch{Got: lr.layout, Want: p.layout}
	}
	p.flatCoords = append(p.flatCoords, lr.flatCoords...)
	p.ends = append(p.ends, len(p.flatCoords))
	return nil
}

// SetCoords sets the coordinates.
func (p *Polygon) SetCoords(coords [][]Coord) (*Polygon, error) {
	if err := p.setCoords(coords); err != nil {
		return nil, err
	}
	return p, nil
}

// SetSRID sets the SRID of p.
func (p *Polygon) SetSRID(srid int) *Polygon {
	p.srid = srid
	return p
}

// Swap swaps the values of p and p2.
func (p *Polygon) Swap(p2 *Polygon) {
	p.geom2.swap(&p2.geom2)
}
