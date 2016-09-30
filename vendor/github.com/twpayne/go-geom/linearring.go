package geom

// A LinearRing is a linear ring.
type LinearRing struct {
	geom1
}

// NewLinearRing returns a new LinearRing with no coordinates.
func NewLinearRing(layout Layout) *LinearRing {
	return NewLinearRingFlat(layout, nil)
}

// NewLinearRingFlat returns a new LinearRing with the given flat coordinates.
func NewLinearRingFlat(layout Layout, flatCoords []float64) *LinearRing {
	lr := new(LinearRing)
	lr.layout = layout
	lr.stride = layout.Stride()
	lr.flatCoords = flatCoords
	return lr
}

// Area returns the the area.
func (lr *LinearRing) Area() float64 {
	return doubleArea1(lr.flatCoords, 0, len(lr.flatCoords), lr.stride) / 2
}

// Clone returns a deep copy.
func (lr *LinearRing) Clone() *LinearRing {
	flatCoords := make([]float64, len(lr.flatCoords))
	copy(flatCoords, lr.flatCoords)
	return NewLinearRingFlat(lr.layout, flatCoords)
}

// Empty returns false.
func (lr *LinearRing) Empty() bool {
	return false
}

// Length returns the length of the perimeter.
func (lr *LinearRing) Length() float64 {
	return length1(lr.flatCoords, 0, len(lr.flatCoords), lr.stride)
}

// MustSetCoords sets the coordinates and panics if there is any error.
func (lr *LinearRing) MustSetCoords(coords []Coord) *LinearRing {
	Must(lr.SetCoords(coords))
	return lr
}

// SetCoords sets the coordinates.
func (lr *LinearRing) SetCoords(coords []Coord) (*LinearRing, error) {
	if err := lr.setCoords(coords); err != nil {
		return nil, err
	}
	return lr, nil
}

// SetSRID sets the SRID of lr.
func (lr *LinearRing) SetSRID(srid int) *LinearRing {
	lr.srid = srid
	return lr
}

// Swap swaps the values of lr and lr2.
func (lr *LinearRing) Swap(lr2 *LinearRing) {
	lr.geom1.swap(&lr2.geom1)
}
