package posting

import (
	"errors"
	"math"
	"sort"
)

func norm(v []float64) float64 {
	vectorNorm, _ := dotProduct(v, v)
	return math.Sqrt(vectorNorm)
}

func dotProduct(a, b []float64) (float64, error) {
	var dotProduct float64
	if len(a) != len(b) {
		err := errors.New("can not compute dot product on vectors of different lengths")
		return dotProduct, err
	}
	for i := range a {
		dotProduct += a[i] * b[i]
	}
	return dotProduct, nil
}

func euclidianDistance(a, b []float64) (float64, error) {
	subtractResult := make([]float64, len(a))
	err := vectorSubtract(a, b, subtractResult)
	return norm(subtractResult), err
}

func cosineSimilarity(a, b []float64) (float64, error) {
	dotProd, err := dotProduct(a, b)
	if err != nil {
		return 0, err
	}
	if norm(a) == 0 || norm(b) == 0 {
		err := errors.New("can not compute cosine similarity on zero vector")
		return 0, err
	}
	return dotProd / (norm(a) * norm(b)), nil
}

func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func vectorAdd(a, b, result []float64) error {
	if len(a) != len(b) {
		return errors.New("can not add vectors of different lengths")
	}
	if len(a) != len(result) {
		return errors.New("result and operand vectors must be same length")
	}
	for i := range a {
		result[i] = a[i] + b[i]
	}
	return nil
}

func vectorSubtract(a, b, result []float64) error {
	if len(a) != len(b) {
		return errors.New("can not subtract vectors of different lengths")
	}
	if len(a) != len(result) {
		return errors.New("result and operand vectors must be same length")
	}
	for i := range a {
		result[i] = a[i] - b[i]
	}
	return nil
}

// Used for distance, since shorter distance is better
func insortBadgerHeapAscending(slice []minBadgerHeapElement, val minBadgerHeapElement) []minBadgerHeapElement {
	i := sort.Search(len(slice), func(i int) bool { return slice[i].value > val.value })
	slice = append(slice, *initBadgerHeapElement(0.0, 0))
	copy(slice[i+1:], slice[i:])
	slice[i] = val
	return slice
}

// Used for cosine similarity, since higher similarity score is better
func insortBadgerHeapDescending(slice []minBadgerHeapElement, val minBadgerHeapElement) []minBadgerHeapElement {
	i := sort.Search(len(slice), func(i int) bool { return slice[i].value > val.value })
	slice = append(slice, *initBadgerHeapElement(0.0, 0))
	copy(slice[i+1:], slice[i:])
	slice[i] = val
	return slice
}
