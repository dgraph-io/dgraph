package geom

// A MultiLineString is a collection of LineStrings.
type MultiLineString struct {
	geom2
}

// NewMultiLineString returns a new MultiLineString with no LineStrings.
func NewMultiLineString(layout Layout) *MultiLineString {
	return NewMultiLineStringFlat(layout, nil, nil)
}

// NewMultiLineStringFlat returns a new MultiLineString with the given flat coordinates.
func NewMultiLineStringFlat(layout Layout, flatCoords []float64, ends []int) *MultiLineString {
	mls := new(MultiLineString)
	mls.layout = layout
	mls.stride = layout.Stride()
	mls.flatCoords = flatCoords
	mls.ends = ends
	return mls
}

// Area returns 0.
func (mls *MultiLineString) Area() float64 {
	return 0
}

// Clone returns a deep copy.
func (mls *MultiLineString) Clone() *MultiLineString {
	flatCoords := make([]float64, len(mls.flatCoords))
	copy(flatCoords, mls.flatCoords)
	ends := make([]int, len(mls.ends))
	copy(ends, mls.ends)
	return NewMultiLineStringFlat(mls.layout, flatCoords, ends)
}

// Empty returns true if the collection is empty.
func (mls *MultiLineString) Empty() bool {
	return mls.NumLineStrings() == 0
}

// Length returns the sum of the length of the LineStrings.
func (mls *MultiLineString) Length() float64 {
	return length2(mls.flatCoords, 0, mls.ends, mls.stride)
}

// LineString returns the ith LineString.
func (mls *MultiLineString) LineString(i int) *LineString {
	offset := 0
	if i > 0 {
		offset = mls.ends[i-1]
	}
	return NewLineStringFlat(mls.layout, mls.flatCoords[offset:mls.ends[i]])
}

// MustSetCoords sets the coordinates and panics on any error.
func (mls *MultiLineString) MustSetCoords(coords [][]Coord) *MultiLineString {
	Must(mls.SetCoords(coords))
	return mls
}

// NumLineStrings returns the number of LineStrings.
func (mls *MultiLineString) NumLineStrings() int {
	return len(mls.ends)
}

// Push appends a LineString.
func (mls *MultiLineString) Push(ls *LineString) error {
	if ls.layout != mls.layout {
		return ErrLayoutMismatch{Got: ls.layout, Want: mls.layout}
	}
	mls.flatCoords = append(mls.flatCoords, ls.flatCoords...)
	mls.ends = append(mls.ends, len(mls.flatCoords))
	return nil
}

// SetCoords sets the coordinates.
func (mls *MultiLineString) SetCoords(coords [][]Coord) (*MultiLineString, error) {
	if err := mls.setCoords(coords); err != nil {
		return nil, err
	}
	return mls, nil
}

// SetSRID sets the SRID of mls.
func (mls *MultiLineString) SetSRID(srid int) *MultiLineString {
	mls.srid = srid
	return mls
}

// Swap swaps the values of mls and mls2.
func (mls *MultiLineString) Swap(mls2 *MultiLineString) {
	mls.geom2.swap(&mls2.geom2)
}
