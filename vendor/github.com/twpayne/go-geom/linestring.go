package geom

// A LineString represents a single, unbroken line, linearly interpreted
// between zero or more control points.
type LineString struct {
	geom1
}

// NewLineString returns a new LineString with layout l and no control points.
func NewLineString(l Layout) *LineString {
	return NewLineStringFlat(l, nil)
}

// NewLineStringFlat returns a new LineString with layout l and control points
// flatCoords.
func NewLineStringFlat(layout Layout, flatCoords []float64) *LineString {
	ls := new(LineString)
	ls.layout = layout
	ls.stride = layout.Stride()
	ls.flatCoords = flatCoords
	return ls
}

// Area returns the length of ls, i.e. zero.
func (ls *LineString) Area() float64 {
	return 0
}

// Clone returns a copy of ls that does not alias ls.
func (ls *LineString) Clone() *LineString {
	flatCoords := make([]float64, len(ls.flatCoords))
	copy(flatCoords, ls.flatCoords)
	return NewLineStringFlat(ls.layout, flatCoords)
}

// Empty returns false.
func (ls *LineString) Empty() bool {
	return false
}

// Interpolate returns the index and delta of val in dimension dim.
func (ls *LineString) Interpolate(val float64, dim int) (int, float64) {
	n := len(ls.flatCoords)
	if n == 0 {
		panic("geom: empty linestring")
	}
	if val <= ls.flatCoords[dim] {
		return 0, 0
	}
	if ls.flatCoords[n-ls.stride+dim] <= val {
		return (n - 1) / ls.stride, 0
	}
	low := 0
	high := n / ls.stride
	for low < high {
		mid := (low + high) / 2
		if val < ls.flatCoords[mid*ls.stride+dim] {
			high = mid
		} else {
			low = mid + 1
		}
	}
	low--
	val0 := ls.flatCoords[low*ls.stride+dim]
	if val == val0 {
		return low, 0
	}
	val1 := ls.flatCoords[(low+1)*ls.stride+dim]
	return low, (val - val0) / (val1 - val0)
}

// Length returns the length of ls.
func (ls *LineString) Length() float64 {
	return length1(ls.flatCoords, 0, len(ls.flatCoords), ls.stride)
}

// MustSetCoords is like SetCoords but it panics on any error.
func (ls *LineString) MustSetCoords(coords []Coord) *LineString {
	Must(ls.SetCoords(coords))
	return ls
}

// SetCoords sets the coordinates of ls.
func (ls *LineString) SetCoords(coords []Coord) (*LineString, error) {
	if err := ls.setCoords(coords); err != nil {
		return nil, err
	}
	return ls, nil
}

// SetSRID sets the SRID of ls.
func (ls *LineString) SetSRID(srid int) *LineString {
	ls.srid = srid
	return ls
}

// SubLineString returns a LineString from starts at index start and stops at
// index stop of ls. The returned LineString aliases ls.
func (ls *LineString) SubLineString(start, stop int) *LineString {
	return NewLineStringFlat(ls.layout, ls.flatCoords[start*ls.stride:stop*ls.stride])
}

// Swap swaps the values of ls and ls2.
func (ls *LineString) Swap(ls2 *LineString) {
	ls.geom1.swap(&ls2.geom1)
}
