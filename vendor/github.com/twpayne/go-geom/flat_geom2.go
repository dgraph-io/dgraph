package geom

type geom2 struct {
	geom1
	ends []int
}

func (g *geom2) Coords() [][]Coord {
	return inflate2(g.flatCoords, 0, g.ends, g.stride)
}

func (g *geom2) Ends() []int {
	return g.ends
}

func (g *geom2) setCoords(coords2 [][]Coord) error {
	var err error
	g.flatCoords, g.ends, err = deflate2(nil, nil, coords2, g.stride)
	return err
}

func (g *geom2) swap(g2 *geom2) {
	g.stride, g2.stride = g2.stride, g.stride
	g.layout, g2.layout = g2.layout, g.layout
	g.flatCoords, g2.flatCoords = g2.flatCoords, g.flatCoords
	g.ends, g2.ends = g2.ends, g.ends
}

func (g *geom2) verify() error {
	if g.stride != g.layout.Stride() {
		return errStrideLayoutMismatch
	}
	if g.stride == 0 {
		if len(g.flatCoords) != 0 {
			return errNonEmptyFlatCoords
		}
		if len(g.ends) != 0 {
			return errNonEmptyEnds
		}
		return nil
	}
	if len(g.flatCoords)%g.stride != 0 {
		return errLengthStrideMismatch
	}
	offset := 0
	for _, end := range g.ends {
		if end%g.stride != 0 {
			return errMisalignedEnd
		}
		if end < offset {
			return errOutOfOrderEnd
		}
		offset = end
	}
	if offset != len(g.flatCoords) {
		return errIncorrectEnd
	}
	return nil
}
