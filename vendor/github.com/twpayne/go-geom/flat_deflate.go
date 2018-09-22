package geom

func deflate0(flatCoords []float64, c Coord, stride int) ([]float64, error) {
	if len(c) != stride {
		return nil, ErrStrideMismatch{Got: len(c), Want: stride}
	}
	flatCoords = append(flatCoords, c...)
	return flatCoords, nil
}

func deflate1(flatCoords []float64, coords1 []Coord, stride int) ([]float64, error) {
	for _, c := range coords1 {
		var err error
		flatCoords, err = deflate0(flatCoords, c, stride)
		if err != nil {
			return nil, err
		}
	}
	return flatCoords, nil
}

func deflate2(flatCoords []float64, ends []int, coords2 [][]Coord, stride int) ([]float64, []int, error) {
	for _, coords1 := range coords2 {
		var err error
		flatCoords, err = deflate1(flatCoords, coords1, stride)
		if err != nil {
			return nil, nil, err
		}
		ends = append(ends, len(flatCoords))
	}
	return flatCoords, ends, nil
}

func deflate3(flatCoords []float64, endss [][]int, coords3 [][][]Coord, stride int) ([]float64, [][]int, error) {
	for _, coords2 := range coords3 {
		var err error
		var ends []int
		flatCoords, ends, err = deflate2(flatCoords, ends, coords2, stride)
		if err != nil {
			return nil, nil, err
		}
		endss = append(endss, ends)
	}
	return flatCoords, endss, nil
}
