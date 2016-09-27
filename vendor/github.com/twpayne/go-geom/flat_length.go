package geom

import (
	"math"
)

func length1(flatCoords []float64, offset, end, stride int) float64 {
	var length float64
	for i := offset + stride; i < end; i += stride {
		dx := flatCoords[i] - flatCoords[i-stride]
		dy := flatCoords[i+1] - flatCoords[i+1-stride]
		length += math.Sqrt(dx*dx + dy*dy)
	}
	return length
}

func length2(flatCoords []float64, offset int, ends []int, stride int) float64 {
	var length float64
	for _, end := range ends {
		length += length1(flatCoords, offset, end, stride)
		offset = end
	}
	return length
}

func length3(flatCoords []float64, offset int, endss [][]int, stride int) float64 {
	var length float64
	for _, ends := range endss {
		length += length2(flatCoords, offset, ends, stride)
		offset = ends[len(ends)-1]
	}
	return length
}
