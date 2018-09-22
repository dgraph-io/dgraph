package geom

func inflate0(flatCoords []float64, offset, end, stride int) Coord {
	if offset+stride != end {
		panic("geom: stride mismatch")
	}
	c := make([]float64, stride)
	copy(c, flatCoords[offset:end])
	return c
}

func inflate1(flatCoords []float64, offset, end, stride int) []Coord {
	coords1 := make([]Coord, (end-offset)/stride)
	for i := range coords1 {
		coords1[i] = inflate0(flatCoords, offset, offset+stride, stride)
		offset += stride
	}
	return coords1
}

func inflate2(flatCoords []float64, offset int, ends []int, stride int) [][]Coord {
	coords2 := make([][]Coord, len(ends))
	for i := range coords2 {
		end := ends[i]
		coords2[i] = inflate1(flatCoords, offset, end, stride)
		offset = end
	}
	return coords2
}

func inflate3(flatCoords []float64, offset int, endss [][]int, stride int) [][][]Coord {
	coords3 := make([][][]Coord, len(endss))
	for i := range coords3 {
		ends := endss[i]
		coords3[i] = inflate2(flatCoords, offset, ends, stride)
		offset = ends[len(ends)-1]
	}
	return coords3
}
