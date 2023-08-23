package posting

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
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

func ParseEdges(s string) ([]uint64, error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return []uint64{}, nil
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	trimmedPre := strings.TrimPrefix(s, "[")
	if len(trimmedPre) == len(s) {
		return nil, cannotConvertToUintSlice(s)
	}
	trimmed := strings.TrimRight(trimmedPre, "]")
	if len(trimmed) == len(trimmedPre) {
		return nil, cannotConvertToUintSlice(s)
	}
	if len(trimmed) == 0 {
		return []uint64{}, nil
	}
	if strings.Index(trimmed, ",") != -1 {
		// Splitting based on comma-separation.
		values := strings.Split(trimmed, ",")
		result := make([]uint64, len(values))
		for i := 0; i < len(values); i++ {
			trimmedVal := strings.TrimSpace(values[i])
			val, err := strconv.ParseUint(trimmedVal, 10, 64)
			if err != nil {
				return nil, cannotConvertToUintSlice(s)
			}
			result[i] = val
		}
		return result, nil
	}
	values := strings.Split(trimmed, " ")
	result := make([]uint64, 0, len(values))
	for i := 0; i < len(values); i++ {
		if len(values[i]) == 0 {
			// skip if we have an empty string. This can naturally
			// occur if input s was "[1.0     2.0]"
			// notice the extra whitespace in separation!
			continue
		}
		if len(values[i]) > 0 {
			val, err := strconv.ParseUint(values[i], 10, 64)
			if err != nil {
				return nil, cannotConvertToUintSlice(s)
			}
			result = append(result, val)
		}
	}
	return result, nil
}

func cannotConvertToUintSlice(s string) error {
	return errors.Errorf("Cannot convert %s to uint slice", s)
}

func diff(a []uint64, b []uint64) []uint64 {
	// Turn b into a map
	m := make(map[uint64]bool, len(b))
	for _, s := range b {
		m[s] = false
	}
	// Append values from the longest slice that don't exist in the map
	var diff []uint64
	for _, s := range a {
		if _, ok := m[s]; !ok {
			diff = append(diff, s)
			continue
		}
		m[s] = true
	}
	return diff
}
