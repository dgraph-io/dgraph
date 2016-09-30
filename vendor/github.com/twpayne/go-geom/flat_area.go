package geom

func doubleArea1(flatCoords []float64, offset, end, stride int) float64 {
	var doubleArea float64
	for i := offset + stride; i < end; i += stride {
		doubleArea += (flatCoords[i+1] - flatCoords[i+1-stride]) * (flatCoords[i] + flatCoords[i-stride])
	}
	return doubleArea
}

func doubleArea2(flatCoords []float64, offset int, ends []int, stride int) float64 {
	var doubleArea float64
	for i, end := range ends {
		da := doubleArea1(flatCoords, offset, end, stride)
		if i == 0 {
			doubleArea = da
		} else {
			doubleArea -= da
		}
		offset = end
	}
	return doubleArea
}

func doubleArea3(flatCoords []float64, offset int, endss [][]int, stride int) float64 {
	var doubleArea float64
	for _, ends := range endss {
		doubleArea += doubleArea2(flatCoords, offset, ends, stride)
		offset = ends[len(ends)-1]
	}
	return doubleArea
}
