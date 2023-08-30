package posting

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

func norm(v []float64) float64 {
	vectorNorm, _ := dotProduct(v, v)
	return math.Sqrt(vectorNorm)
}

func normSq(v []float64) float64 {
	normSq, _ := dotProduct(v, v)
	return normSq
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

func EuclidianDistance(a, b []float64) (float64, error) {
	subtractResult := make([]float64, len(a))
	err := vectorSubtract(a, b, subtractResult)
	return norm(subtractResult), err
}

func cosineSimilarity(a, b []float64) (float64, error) {
	dotProd, err := dotProduct(a, b)
	if err != nil {
		return 0, err
	}
	normA := norm(a)
	normB := norm(b)
	if normA == 0 || normB == 0 {
		err := errors.New("can not compute cosine similarity on zero vector")
		return 0, err
	}
	return dotProd / (normA * normB), nil
}

func euclidianDistanceSq(a, b []float64) (float64, error) {
	subtractResult := make([]float64, len(a))
	err := vectorSubtract(a, b, subtractResult)
	return normSq(subtractResult), err
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

func contains(slice []minBadgerHeapElement, uuid uint64) bool {
	for _, e := range slice {
		if e.index == uuid {
			return true
		}
	}
	return false
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

const (
	HnswEuclidian = "HNSW-Euclidian"
	HnswCosine    = "HNSW-Cosine"
	HnswDotProd   = "HNSW-DotProduct"
	plError       = "\nerror fetching posting list for data key: "
	dataError     = "\nerror fetching data for data key: "
	VecKeyword    = "__vector_"
	VecEntry      = "__vector_entry"
	VecDead       = "__vector_dead"
)

type SimilarityType struct {
	distanceScore func(v, w []float64) (float64, error)
	insortHeap    func(slice []minBadgerHeapElement, val minBadgerHeapElement) []minBadgerHeapElement
}

var euclidianSimilarityType = SimilarityType{distanceScore: euclidianDistanceSq, insortHeap: insortBadgerHeapAscending}
var cosineSimilarityType = SimilarityType{distanceScore: cosineSimilarity, insortHeap: insortBadgerHeapDescending}
var dotProductSimilarityType = SimilarityType{distanceScore: dotProduct, insortHeap: insortBadgerHeapDescending}

var simTypeMap = map[string]SimilarityType{
	HnswEuclidian: euclidianSimilarityType,
	HnswCosine:    cosineSimilarityType,
	HnswDotProd:   dotProductSimilarityType,
}

type CacheType interface {
	Get(key []byte) (*List, error)
	Ts() uint64
}

// implements CacheType interface
type TxnCache struct {
	txn     *Txn
	startTs uint64
}

func (tc *TxnCache) Get(key []byte) (*List, error) {
	return tc.txn.Get(key)
}

func (tc *TxnCache) Ts() uint64 {
	return tc.startTs
}

// implements cacheType interface
type queryCache struct {
	cache  *LocalCache
	readTs uint64
}

func (qc *queryCache) Get(key []byte) (*List, error) {
	return qc.cache.Get(key)
}

func (qc *queryCache) Ts() uint64 {
	return qc.readTs
}

func getDataFromKeyWithCacheType(keyString string, uid uint64, c CacheType) (types.Val, *List, error) {
	key := x.DataKey(keyString, uid)
	pl, err := c.Get(key)
	if err != nil {
		return types.Val{}, nil, errors.New(err.Error() + plError + keyString + " with uid" + strconv.FormatUint(uid, 10))
	}
	data, err := pl.Value(c.Ts())
	if err != nil {
		return types.Val{}, pl, errors.New(err.Error() + dataError + keyString + " with uid" + strconv.FormatUint(uid, 10))
	}
	return data, pl, nil
}

func getDataFromKeyWithTxn(keyString string, uid uint64, txn *Txn) (types.Val, *List, error) {
	key := x.DataKey(keyString, uid)
	pl, err := txn.Get(key)
	if err != nil {
		return types.Val{}, &List{}, errors.New(plError + keyString + " with uid" + strconv.FormatUint(uid, 10))
	}
	data, err := pl.Value(txn.StartTs)
	if err != nil {
		return types.Val{}, pl, errors.New(dataError + keyString + " with uid" + strconv.FormatUint(uid, 10))
	}
	return data, pl, nil
}

func getDataFromKeyWithCache(keyString string, uid uint64, cache *LocalCache, readTs uint64) (types.Val, *List, error) {
	key := x.DataKey(keyString, uid)
	pl, err := cache.Get(key)
	if err != nil {
		return types.Val{}, &List{}, errors.New(plError + keyString + " with uid" + strconv.FormatUint(uid, 10))
	}
	data, err := pl.Value(readTs)
	if err != nil {
		return types.Val{}, pl, errors.New(dataError + keyString + " with uid" + strconv.FormatUint(uid, 10))
	}
	return data, pl, nil
}

func newBadgerEdgeKeyValueEntry(ctx context.Context, plist *List, txn *Txn, pred string,
	level int, uuid uint64, edges []byte) error {
	edge := &pb.DirectedEdge{
		Entity:    uuid,
		Attr:      buildDataKeyPred(pred, VecKeyword, fmt.Sprint(level)),
		Value:     edges,
		ValueType: pb.Posting_ValType(0),
		Op:        pb.DirectedEdge_SET,
	}
	if err := plist.addMutation(ctx, txn, edge); err != nil {
		return err
	}
	return nil
}

func deadUUidInsert(ctx context.Context, c CacheType, plist *List, pred string, deadNodes []byte) error {
	txn := NewTxn(c.Ts())
	edge := &pb.DirectedEdge{
		Entity:    1,
		Attr:      buildDataKeyPred(pred, VecDead),
		Value:     deadNodes,
		ValueType: pb.Posting_ValType(0),
		Op:        pb.DirectedEdge_SET,
	}
	return plist.addMutation(ctx, txn, edge)
}

func entryUuidInsert(ctx context.Context, plist *List, txn *Txn, pred string, entryUuid []byte) error {
	edge := &pb.DirectedEdge{
		Entity:    1,
		Attr:      buildDataKeyPred(pred, VecEntry),
		Value:     entryUuid,
		ValueType: pb.Posting_ValType(7),
		Op:        pb.DirectedEdge_SET,
	}
	return plist.addMutationInternal(ctx, txn, edge)
}

func addStartNodeToAllLevels(ctx context.Context, pl *List, txn *Txn, pred string, maxLevels int, inUuid uint64) error {
	for i := 0; i < maxLevels; i++ {
		key := x.DataKey(buildDataKeyPred(pred, VecKeyword, fmt.Sprint(i)), inUuid)
		plL, err := txn.Get(key)
		if err != nil {
			return err
		}
		// creates empty at all levels only for entry node
		err = newBadgerEdgeKeyValueEntry(ctx, plL, txn, pred, i, inUuid, []byte{})
		if err != nil {
			return err
		}
	}
	inUuidByte := make([]byte, 8)
	binary.BigEndian.PutUint64(inUuidByte, inUuid)         // convert inUuid to bytes
	err := entryUuidInsert(ctx, pl, txn, pred, inUuidByte) // add inUuid as entry for this structure from now on
	if err != nil {
		return err
	}
	return nil
}

func buildDataKeyPred(strs ...string) string {
	total := ""
	for _, s := range strs {
		total += s
	}
	return total
}
