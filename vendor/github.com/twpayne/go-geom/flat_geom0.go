package geom

type geom0 struct {
	layout     Layout
	stride     int
	flatCoords []float64
	srid       int
}

func (g *geom0) Bounds() *Bounds {
	return NewBounds(g.layout).extendFlatCoords(g.flatCoords, 0, len(g.flatCoords), g.stride)
}

func (g *geom0) Coords() Coord {
	return inflate0(g.flatCoords, 0, len(g.flatCoords), g.stride)
}

func (g *geom0) Ends() []int {
	return nil
}

func (g *geom0) Endss() [][]int {
	return nil
}

func (g *geom0) FlatCoords() []float64 {
	return g.flatCoords
}

func (g *geom0) Layout() Layout {
	return g.layout
}

func (g *geom0) NumCoords() int {
	return 1
}

func (g *geom0) Reserve(n int) {
	if cap(g.flatCoords) < n*g.stride {
		fcs := make([]float64, len(g.flatCoords), n*g.stride)
		copy(fcs, g.flatCoords)
		g.flatCoords = fcs
	}
}

func (g *geom0) SRID() int {
	return g.srid
}

func (g *geom0) swap(g2 *geom0) {
	g.stride, g2.stride = g2.stride, g.stride
	g.layout, g2.layout = g2.layout, g.layout
	g.flatCoords, g2.flatCoords = g2.flatCoords, g.flatCoords
	g.srid, g2.srid = g2.srid, g.srid
}

func (g *geom0) setCoords(coords0 []float64) error {
	var err error
	g.flatCoords, err = deflate0(nil, coords0, g.stride)
	return err
}

func (g *geom0) Stride() int {
	return g.stride
}

func (g *geom0) verify() error {
	if g.stride != g.layout.Stride() {
		return errStrideLayoutMismatch
	}
	if g.stride == 0 {
		if len(g.flatCoords) != 0 {
			return errNonEmptyFlatCoords
		}
		return nil
	}
	if len(g.flatCoords) != g.stride {
		return errLengthStrideMismatch
	}
	return nil
}
