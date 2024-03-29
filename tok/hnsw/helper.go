package hnsw

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/chewxy/math32"
	c "github.com/dgraph-io/dgraph/tok/constraints"
	"github.com/pkg/errors"
)

const (
	Euclidian            = "euclidian"
	Cosine               = "cosine"
	DotProd              = "dotproduct"
	plError              = "\nerror fetching posting list for data key: "
	dataError            = "\nerror fetching data for data key: "
	VecKeyword           = "__vector_"
	visitedVectorsLevel  = "visited_vectors_level_"
	distanceComputations = "vector_distance_computations"
	searchTime           = "vector_search_time"
	VecEntry             = "__vector_entry"
	VecDead              = "__vector_dead"
	VectorIndexMaxLevels = 5
	EfConstruction       = 16
	EfSearch             = 12
	numEdgesConst        = 2
	// ByteData indicates the key stores data.
	ByteData = byte(0x00)
	// DefaultPrefix is the prefix used for data, index and reverse keys so that relative
	DefaultPrefix = byte(0x00)
	// NsSeparator is the separator between the namespace and attribute.
	NsSeparator = "-"
)

type SearchResult struct {
	nnUids        []uint64
	traversalPath []uint64
	extraMetrics  map[string]uint64
}

func newSearchResult(nnUids []uint64, traversalPath []uint64,
	extraMetrics map[string]uint64) *SearchResult {
	return &SearchResult{
		nnUids:        nnUids,
		traversalPath: traversalPath,
		extraMetrics:  extraMetrics,
	}
}

func (s *SearchResult) GetNnUids() []uint64 {
	return s.nnUids
}

func (s *SearchResult) GetTraversalPath() []uint64 {
	return s.traversalPath
}

func (s *SearchResult) GetExtraMetrics() map[string]uint64 {
	return s.extraMetrics
}

func norm[T c.Float](v []T, floatBits int) T {
	vectorNorm, _ := dotProduct(v, v, floatBits)
	if floatBits == 32 {
		return T(math32.Sqrt(float32(vectorNorm)))
	}
	if floatBits == 64 {
		return T(math.Sqrt(float64(vectorNorm)))
	}
	panic("Invalid floatBits")
}

// This needs to implement signature of SimilarityType[T].distanceScore
// function, hence it takes in a floatBits parameter,
// but doesn't actually use it.
func dotProduct[T c.Float](a, b []T, floatBits int) (T, error) {
	var dotProduct T
	if len(a) != len(b) {
		err := errors.New("can not compute dot product on vectors of different lengths")
		return dotProduct, err
	}
	for i := range a {
		dotProduct += a[i] * b[i]
	}
	return dotProduct, nil
}

// This needs to implement signature of SimilarityType[T].distanceScore
// function, hence it takes in a floatBits parameter.
func cosineSimilarity[T c.Float](a, b []T, floatBits int) (T, error) {
	dotProd, err := dotProduct(a, b, floatBits)
	if err != nil {
		return 0, err
	}
	normA := norm[T](a, floatBits)
	normB := norm[T](b, floatBits)
	if normA == 0 || normB == 0 {
		err := errors.New("can not compute cosine similarity on zero vector")
		var empty T
		return empty, err
	}
	return dotProd / (normA * normB), nil
}

// This needs to implement signature of SimilarityType[T].distanceScore
// function, hence it takes in a floatBits parameter,
// but doesn't actually use it.
func euclidianDistanceSq[T c.Float](a, b []T, floatBits int) (T, error) {
	if len(a) != len(b) {
		return 0, errors.New("can not subtract vectors of different lengths")
	}
	var distSq T
	for i := range a {
		val := a[i] - b[i]
		distSq += val * val
	}
	return distSq, nil
}

func contains[T c.Float](slice []minPersistentHeapElement[T], uuid uint64) bool {
	for _, e := range slice {
		if e.index == uuid {
			return true
		}
	}
	return false
}

// Used for distance, since shorter distance is better
func insortPersistentHeapAscending[T c.Float](
	slice []minPersistentHeapElement[T],
	val minPersistentHeapElement[T]) []minPersistentHeapElement[T] {
	i := sort.Search(len(slice), func(i int) bool { return slice[i].value > val.value })
	var empty T
	slice = append(slice, *initPersistentHeapElement(empty, notAUid, false))
	copy(slice[i+1:], slice[i:])
	slice[i] = val
	return slice
}

// Used for cosine similarity, since higher similarity score is better
func insortPersistentHeapDescending[T c.Float](
	slice []minPersistentHeapElement[T],
	val minPersistentHeapElement[T]) []minPersistentHeapElement[T] {
	i := sort.Search(len(slice), func(i int) bool { return slice[i].value < val.value })
	var empty T
	slice = append(slice, *initPersistentHeapElement(empty, notAUid, false))
	copy(slice[i+1:], slice[i:])
	slice[i] = val
	return slice
}

func isBetterScoreForDistance[T c.Float](a, b T) bool {
	return a < b
}

func isBetterScoreForSimilarity[T c.Float](a, b T) bool {
	return a > b
}

func ParseEdges(s string) ([]uint64, error) {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return []uint64{}, nil
	}
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
	if strings.Contains(trimmed, ",") {
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

// TODO: Move SimilarityType to index package.
//
//	Remove "hnsw-isms".
type SimilarityType[T c.Float] struct {
	indexType     string
	distanceScore func(v, w []T, floatBits int) (T, error)
	insortHeap    func(slice []minPersistentHeapElement[T], val minPersistentHeapElement[T]) []minPersistentHeapElement[T]
	isBetterScore func(a, b T) bool
}

func GetSimType[T c.Float](indexType string, floatBits int) SimilarityType[T] {
	switch {
	case indexType == Euclidian:
		return SimilarityType[T]{indexType: Euclidian, distanceScore: euclidianDistanceSq[T],
			insortHeap: insortPersistentHeapAscending[T], isBetterScore: isBetterScoreForDistance[T]}
	case indexType == Cosine:
		return SimilarityType[T]{indexType: Cosine, distanceScore: cosineSimilarity[T],
			insortHeap: insortPersistentHeapDescending[T], isBetterScore: isBetterScoreForSimilarity[T]}
	case indexType == DotProd:
		return SimilarityType[T]{indexType: DotProd, distanceScore: dotProduct[T],
			insortHeap: insortPersistentHeapDescending[T], isBetterScore: isBetterScoreForSimilarity[T]}
	default:
		return SimilarityType[T]{indexType: Euclidian, distanceScore: euclidianDistanceSq[T],
			insortHeap: insortPersistentHeapAscending[T], isBetterScore: isBetterScoreForDistance[T]}
	}
}
