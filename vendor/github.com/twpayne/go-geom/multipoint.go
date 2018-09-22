package geom

// A MultiPoint is a collection of Points.
type MultiPoint struct {
	geom1
}

// NewMultiPoint returns a new, empty, MultiPoint.
func NewMultiPoint(layout Layout) *MultiPoint {
	return NewMultiPointFlat(layout, nil)
}

// NewMultiPointFlat returns a new MultiPoint with the given flat coordinates.
func NewMultiPointFlat(layout Layout, flatCoords []float64) *MultiPoint {
	mp := new(MultiPoint)
	mp.layout = layout
	mp.stride = layout.Stride()
	mp.flatCoords = flatCoords
	return mp
}

// Area returns zero.
func (mp *MultiPoint) Area() float64 {
	return 0
}

// Clone returns a deep copy.
func (mp *MultiPoint) Clone() *MultiPoint {
	flatCoords := make([]float64, len(mp.flatCoords))
	copy(flatCoords, mp.flatCoords)
	return NewMultiPointFlat(mp.layout, flatCoords)
}

// Empty returns true if the collection is empty.
func (mp *MultiPoint) Empty() bool {
	return mp.NumPoints() == 0
}

// Length returns zero.
func (mp *MultiPoint) Length() float64 {
	return 0
}

// MustSetCoords sets the coordinates and panics on any error.
func (mp *MultiPoint) MustSetCoords(coords []Coord) *MultiPoint {
	Must(mp.SetCoords(coords))
	return mp
}

// SetCoords sets the coordinates.
func (mp *MultiPoint) SetCoords(coords []Coord) (*MultiPoint, error) {
	if err := mp.setCoords(coords); err != nil {
		return nil, err
	}
	return mp, nil
}

// SetSRID sets the SRID of mp.
func (mp *MultiPoint) SetSRID(srid int) *MultiPoint {
	mp.srid = srid
	return mp
}

// NumPoints returns the number of Points.
func (mp *MultiPoint) NumPoints() int {
	return mp.NumCoords()
}

// Point returns the ith Point.
func (mp *MultiPoint) Point(i int) *Point {
	return NewPointFlat(mp.layout, mp.Coord(i))
}

// Push appends a point.
func (mp *MultiPoint) Push(p *Point) error {
	if p.layout != mp.layout {
		return ErrLayoutMismatch{Got: p.layout, Want: mp.layout}
	}
	mp.flatCoords = append(mp.flatCoords, p.flatCoords...)
	return nil
}

// Swap swaps the values of mp and mp2.
func (mp *MultiPoint) Swap(mp2 *MultiPoint) {
	mp.geom1.swap(&mp2.geom1)
}
