package geom

// A MultiPolygon is a collection of Polygons.
type MultiPolygon struct {
	geom3
}

// NewMultiPolygon returns a new MultiPolygon with no Polygons.
func NewMultiPolygon(layout Layout) *MultiPolygon {
	return NewMultiPolygonFlat(layout, nil, nil)
}

// NewMultiPolygonFlat returns a new MultiPolygon with the given flat coordinates.
func NewMultiPolygonFlat(layout Layout, flatCoords []float64, endss [][]int) *MultiPolygon {
	mp := new(MultiPolygon)
	mp.layout = layout
	mp.stride = layout.Stride()
	mp.flatCoords = flatCoords
	mp.endss = endss
	return mp
}

// Area returns the sum of the area of the individual Polygons.
func (mp *MultiPolygon) Area() float64 {
	return doubleArea3(mp.flatCoords, 0, mp.endss, mp.stride) / 2
}

// Clone returns a deep copy.
func (mp *MultiPolygon) Clone() *MultiPolygon {
	flatCoords := make([]float64, len(mp.flatCoords))
	copy(flatCoords, mp.flatCoords)
	endss := make([][]int, len(mp.endss))
	for i, ends := range mp.endss {
		endss[i] = make([]int, len(ends))
		copy(endss[i], ends)
	}
	return NewMultiPolygonFlat(mp.layout, flatCoords, endss)
}

// Empty returns true if the collection is empty.
func (mp *MultiPolygon) Empty() bool {
	return mp.NumPolygons() == 0
}

// Length returns the sum of the perimeters of the Polygons.
func (mp *MultiPolygon) Length() float64 {
	return length3(mp.flatCoords, 0, mp.endss, mp.stride)
}

// MustSetCoords sets the coordinates and panics on any error.
func (mp *MultiPolygon) MustSetCoords(coords [][][]Coord) *MultiPolygon {
	Must(mp.SetCoords(coords))
	return mp
}

// NumPolygons returns the number of Polygons.
func (mp *MultiPolygon) NumPolygons() int {
	return len(mp.endss)
}

// Polygon returns the ith Polygon.
func (mp *MultiPolygon) Polygon(i int) *Polygon {
	offset := 0
	if i > 0 {
		ends := mp.endss[i-1]
		offset = ends[len(ends)-1]
	}
	ends := make([]int, len(mp.endss[i]))
	if offset == 0 {
		copy(ends, mp.endss[i])
	} else {
		for j, end := range mp.endss[i] {
			ends[j] = end - offset
		}
	}
	return NewPolygonFlat(mp.layout, mp.flatCoords[offset:mp.endss[i][len(mp.endss[i])-1]], ends)
}

// Push appends a Polygon.
func (mp *MultiPolygon) Push(p *Polygon) error {
	if p.layout != mp.layout {
		return ErrLayoutMismatch{Got: p.layout, Want: mp.layout}
	}
	offset := len(mp.flatCoords)
	ends := make([]int, len(p.ends))
	if offset == 0 {
		copy(ends, p.ends)
	} else {
		for i, end := range p.ends {
			ends[i] = end + offset
		}
	}
	mp.flatCoords = append(mp.flatCoords, p.flatCoords...)
	mp.endss = append(mp.endss, ends)
	return nil
}

// SetCoords sets the coordinates.
func (mp *MultiPolygon) SetCoords(coords [][][]Coord) (*MultiPolygon, error) {
	if err := mp.setCoords(coords); err != nil {
		return nil, err
	}
	return mp, nil
}

// SetSRID sets the SRID of mp.
func (mp *MultiPolygon) SetSRID(srid int) *MultiPolygon {
	mp.srid = srid
	return mp
}

// Swap swaps the values of mp and mp2.
func (mp *MultiPolygon) Swap(mp2 *MultiPolygon) {
	mp.geom3.swap(&mp2.geom3)
}
