package geom

// A Point represents a single point.
type Point struct {
	geom0
}

// NewPoint allocates a new Point with layout l and all values zero.
func NewPoint(l Layout) *Point {
	return NewPointFlat(l, make([]float64, l.Stride()))
}

// NewPointFlat allocates a new Point with layout l and flat coordinates flatCoords.
func NewPointFlat(l Layout, flatCoords []float64) *Point {
	p := new(Point)
	p.layout = l
	p.stride = l.Stride()
	p.flatCoords = flatCoords
	return p
}

// Area returns p's area, i.e. zero.
func (p *Point) Area() float64 {
	return 0
}

// Clone returns a copy of p that does not alias p.
func (p *Point) Clone() *Point {
	flatCoords := make([]float64, len(p.flatCoords))
	copy(flatCoords, p.flatCoords)
	return NewPointFlat(p.layout, flatCoords)
}

// Empty returns true if p contains no geometries, i.e. it returns false.
func (p *Point) Empty() bool {
	return false
}

// Length returns the length of p, i.e. zero.
func (p *Point) Length() float64 {
	return 0
}

// MustSetCoords is like SetCoords but panics on any error.
func (p *Point) MustSetCoords(coords Coord) *Point {
	Must(p.SetCoords(coords))
	return p
}

// SetCoords sets the coordinates of p.
func (p *Point) SetCoords(coords Coord) (*Point, error) {
	if err := p.setCoords(coords); err != nil {
		return nil, err
	}
	return p, nil
}

// SetSRID sets the SRID of p.
func (p *Point) SetSRID(srid int) *Point {
	p.srid = srid
	return p
}

// Swap swaps the values of p and p2.
func (p *Point) Swap(p2 *Point) {
	p.geom0.swap(&p2.geom0)
}

// X returns p's X-coordinate.
func (p *Point) X() float64 {
	return p.flatCoords[0]
}

// Y returns p's Y-coordinate.
func (p *Point) Y() float64 {
	return p.flatCoords[1]
}

// Z returns p's Z-coordinate, or zero if p has no Z-coordinate.
func (p *Point) Z() float64 {
	zIndex := p.layout.ZIndex()
	if zIndex == -1 {
		return 0
	}
	return p.flatCoords[zIndex]
}

// M returns p's M-coordinate, or zero if p has no M-coordinate.
func (p *Point) M() float64 {
	mIndex := p.layout.MIndex()
	if mIndex == -1 {
		return 0
	}
	return p.flatCoords[mIndex]
}
