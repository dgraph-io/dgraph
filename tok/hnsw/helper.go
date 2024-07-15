package hnsw

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"github.com/chewxy/math32"
	c "github.com/dgraph-io/dgraph/tok/constraints"
	"github.com/dgraph-io/dgraph/tok/index"
	"github.com/getsentry/sentry-go"
	"github.com/pkg/errors"
	"github.com/viterin/vek"
	"github.com/viterin/vek/vek32"
)

const (
	Euclidian            = "euclidian"
	Cosine               = "cosine"
	DotProd              = "dotproduct"
	plError              = "\nerror fetching posting list for data key: "
	dataError            = "\nerror fetching data for data key: "
	EmptyHNSWTreeError   = "HNSW tree has no elements"
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

var k int

// This needs to implement signature of SimilarityType[T].distanceScore
// function, hence it takes in a floatBits parameter,
// but doesn't actually use it.
func dotProduct[T c.Float](a, b []T, floatBits int) (T, error) {
	k += 1
	if k%100000 == 0 {
		fmt.Println("dot:", k)
	}

	if len(a) != len(b) {
		err := errors.New("can not compute dot product on vectors of different lengths")
		return T(0), err
	}

	if floatBits == 32 {
		var a1, b1 []float32
		a1 = *(*[]float32)(unsafe.Pointer(&a))
		b1 = *(*[]float32)(unsafe.Pointer(&b))
		return T(vek32.Dot(a1, b1)), nil
	} else if floatBits == 64 {
		var a1, b1 []float64
		a1 = *(*[]float64)(unsafe.Pointer(&a))
		b1 = *(*[]float64)(unsafe.Pointer(&b))
		return T(vek.Dot(a1, b1)), nil
	}

	panic("invalid")
}

// This needs to implement signature of SimilarityType[T].distanceScore
// function, hence it takes in a floatBits parameter.
func cosineSimilarity[T c.Float](a, b []T, floatBits int) (T, error) {
	if len(a) != len(b) {
		err := errors.New("can not compute dot product on vectors of different lengths")
		return T(0), err
	}
	if floatBits == 32 {
		var a1, b1 []float32
		a1 = *(*[]float32)(unsafe.Pointer(&a))
		b1 = *(*[]float32)(unsafe.Pointer(&b))
		return T(vek32.CosineSimilarity(a1, b1)), nil
	} else if floatBits == 64 {
		var a1, b1 []float64
		a1 = *(*[]float64)(unsafe.Pointer(&a))
		b1 = *(*[]float64)(unsafe.Pointer(&b))
		return T(vek.CosineSimilarity(a1, b1)), nil
	}
	panic("Invalid")
	//dotProd, err := dotProduct(a, b, floatBits)
	//if err != nil {
	//	return 0, err
	//}
	//normA := norm[T](a, floatBits)
	//normB := norm[T](b, floatBits)
	//if normA == 0 || normB == 0 {
	//	err := errors.New("can not compute cosine similarity on zero vector")
	//	var empty T
	//	return empty, err
	//}
	//return dotProd / (normA * normB), nil
}

// This needs to implement signature of SimilarityType[T].distanceScore
// function, hence it takes in a floatBits parameter,
// but doesn't actually use it.
func euclidianDistanceSq[T c.Float](a, b []T, floatBits int) (T, error) {
	if len(a) != len(b) {
		return 0, errors.New("can not subtract vectors of different lengths")
	}
	if floatBits == 32 {
		var a1, b1 []float32
		a1 = *(*[]float32)(unsafe.Pointer(&a))
		b1 = *(*[]float32)(unsafe.Pointer(&b))
		return T(vek32.Distance(a1, b1)), nil
	} else if floatBits == 64 {
		var a1, b1 []float64
		a1 = *(*[]float64)(unsafe.Pointer(&a))
		b1 = *(*[]float64)(unsafe.Pointer(&b))
		return T(vek.Distance(a1, b1)), nil
	}
	panic("Invalid")
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

// implements CacheType interface
type TxnCache struct {
	txn     index.Txn
	startTs uint64
}

func (tc *TxnCache) Get(key []byte) (rval index.Value, rerr error) {
	return tc.txn.Get(key)
}

func (tc *TxnCache) Ts() uint64 {
	return tc.startTs
}

func (tc *TxnCache) Find(prefix []byte, filter func([]byte) bool) (uint64, error) {
	return tc.txn.Find(prefix, filter)
}

func NewTxnCache(txn index.Txn, startTs uint64) *TxnCache {
	return &TxnCache{
		txn:     txn,
		startTs: startTs,
	}
}

// implements index.CacheType interface
type QueryCache struct {
	cache  index.LocalCache
	readTs uint64
}

func (qc *QueryCache) Find(prefix []byte, filter func([]byte) bool) (uint64, error) {
	return qc.cache.Find(prefix, filter)
}

func (qc *QueryCache) Get(key []byte) (rval index.Value, rerr error) {
	return qc.cache.Get(key)
}

func (qc *QueryCache) Ts() uint64 {
	return qc.readTs
}

func NewQueryCache(cache index.LocalCache, readTs uint64) *QueryCache {
	return &QueryCache{
		cache:  cache,
		readTs: readTs,
	}
}

// getDataFromKeyWithCacheType(keyString, uid, c) looks up data in c
// associated with keyString and uid.
func getDataFromKeyWithCacheType(keyString string, uid uint64, c index.CacheType) (index.Value, error) {
	key := DataKey(keyString, uid)
	data, err := c.Get(key)
	if err != nil {
		return nil, errors.New(err.Error() + plError + keyString + " with uid" + strconv.FormatUint(uid, 10))
	}
	return data, nil
}

// populateEdgeDataFromStore(keyString, uid, c, edgeData)
// will fill edgeData with the contents of the neighboring edges for
// a given DataKey by looking into the given cache (which may result
// in a call to the underlying persistent storage).
// If data is found for the key, this returns true, otherwise, it
// returns false. If the data was found (and there were no errors),
// it populates edgeData with the found contents.
func populateEdgeDataFromKeyWithCacheType(
	keyString string,
	uid uint64,
	c index.CacheType,
	edgeData *[][]uint64) (bool, error) {
	data, err := getDataFromKeyWithCacheType(keyString, uid, c)
	// Note that "dataError" errors are treated as just not having
	// found the data -- no harm, no foul, as it is probably a
	// dead reference that we can ignore.
	if err != nil && !strings.Contains(err.Error(), dataError) {
		return false, err
	}
	if data == nil {
		return false, nil
	}
	err = decodeUint64MatrixUnsafe(data.([]byte), edgeData)
	return true, err
}

// entryUuidInsert adds the entry uuid to the given key
func entryUuidInsert(
	ctx context.Context,
	key []byte,
	txn index.Txn,
	predEntryKey string,
	entryUuid []byte) (*index.KeyValue, error) {
	edge := &index.KeyValue{
		Entity: 1,
		Attr:   predEntryKey,
		Value:  entryUuid,
	}
	err := txn.AddMutationWithLockHeld(ctx, key, edge)
	return edge, err
}

func ConcatStrings(strs ...string) string {
	total := ""
	for _, s := range strs {
		total += s
	}
	return total
}

func getInsertLayer(maxLevels int) int {
	// multFactor is a multiplicative factor used to normalize the distribution
	var level int
	randFloat := rand.Float64()
	for i := 0; i < maxLevels; i++ {
		// calculate level based on section 3.1 here
		if randFloat < math.Pow(1.0/float64(5), float64(maxLevels-1-i)) {
			level = i
			break
		}
	}
	return level
}

// adds the data corresponding to a uid to the given vec variable in the form of []T
// this does not allocate memory for vec, so it must be allocated before calling this function
func (ph *persistentHNSW[T]) getVecFromUid(uid uint64, c index.CacheType, vec *[]T) error {
	data, err := getDataFromKeyWithCacheType(ph.pred, uid, c)
	if err != nil {
		if strings.Contains(err.Error(), plError) {
			// no vector. Return empty array of floats
			index.BytesAsFloatArray([]byte{}, vec, ph.floatBits)
			return errors.New("Nil vector returned")
		}
		return err
	}
	if data != nil {
		index.BytesAsFloatArray(data.([]byte), vec, ph.floatBits)
		return nil

	} else {
		index.BytesAsFloatArray([]byte{}, vec, ph.floatBits)
		return errors.New("Nil vector returned")
	}
}

// chooses whether to create the entry and start nodes based on if it already
// exists, and if it hasnt been created yet, it adds the startNode to all
// levels.
func (ph *persistentHNSW[T]) createEntryAndStartNodes(
	ctx context.Context,
	c *TxnCache,
	inUuid uint64,
	vec *[]T) (uint64, []*index.KeyValue, error) {
	txn := c.txn
	edges := []*index.KeyValue{}
	entryKey := DataKey(ph.vecEntryKey, 1) // 0-profile_vector_entry
	txn.LockKey(entryKey)
	defer txn.UnlockKey(entryKey)
	data, _ := txn.GetWithLockHeld(entryKey)

	create_edges := func(inUuid uint64) (uint64, []*index.KeyValue, error) {
		startEdges, err := ph.addStartNodeToAllLevels(ctx, entryKey, txn, inUuid)
		if err != nil {
			return 0, []*index.KeyValue{}, err
		}
		// return entry node at all levels
		edges = append(edges, startEdges...)
		return 0, edges, nil
	}

	if data == nil {
		// no entries in vector index yet b/c no entry exists, so put in all levels
		return create_edges(inUuid)
	}

	entry := BytesToUint64(data.([]byte)) // convert entry Uuid returned from Get to uint64
	err := ph.getVecFromUid(entry, c, vec)
	if err != nil || len(*vec) == 0 {
		// The entry vector has been deleted. We have to create a new entry vector.
		entry, err := ph.calculateNewEntryVec(ctx, c, vec)
		if err != nil {
			// No other node exists, go with the new node that has come
			return create_edges(inUuid)
		}
		return create_edges(entry)
	}

	return entry, edges, nil
}

func encodeUint64MatrixUnsafe(matrix [][]uint64) []byte {
	if len(matrix) == 0 {
		return nil
	}

	// Calculate the total size
	var totalSize uint64
	for _, row := range matrix {
		totalSize += uint64(len(row))*uint64(unsafe.Sizeof(uint64(0))) + uint64(unsafe.Sizeof(uint64(0)))
	}
	totalSize += uint64(unsafe.Sizeof(uint64(0)))

	// Create a byte slice with the appropriate size
	data := make([]byte, totalSize)

	offset := 0
	// Write number of rows
	rows := uint64(len(matrix))
	copy(data[offset:offset+8], (*[8]byte)(unsafe.Pointer(&rows))[:])
	offset += 8

	// Write each row's length and data
	for _, row := range matrix {
		rowLen := uint64(len(row))
		copy(data[offset:offset+8], (*[8]byte)(unsafe.Pointer(&rowLen))[:])
		offset += 8
		for _, value := range row {
			copy(data[offset:offset+8], (*[8]byte)(unsafe.Pointer(&value))[:])
			offset += 8
		}
	}

	return data
}

func decodeUint64MatrixUnsafe(data []byte, matrix *[][]uint64) error {
	if len(data) == 0 {
		return nil
	}

	offset := 0
	// Read number of rows
	var rows uint64
	rows = *(*uint64)(unsafe.Pointer(&data[offset]))
	offset += 8

	*matrix = make([][]uint64, rows)

	for i := 0; i < int(rows); i++ {
		// Read row length
		var rowLen uint64
		rowLen = *(*uint64)(unsafe.Pointer(&data[offset]))
		offset += 8

		(*matrix)[i] = make([]uint64, rowLen)
		for j := 0; j < int(rowLen); j++ {
			(*matrix)[i][j] = *(*uint64)(unsafe.Pointer(&data[offset]))
			offset += 8
		}
	}

	return nil
}

// adds empty layers to all levels
func (ph *persistentHNSW[T]) addStartNodeToAllLevels(
	ctx context.Context,
	entryKey []byte,
	txn index.Txn,
	inUuid uint64) ([]*index.KeyValue, error) {
	edges := []*index.KeyValue{}
	key := DataKey(ph.vecKey, inUuid)
	emptyEdgesBytes := encodeUint64MatrixUnsafe(make([][]uint64, ph.maxLevels))
	// creates empty at all levels only for entry node
	edge, err := ph.newPersistentEdgeKeyValueEntry(ctx, key, txn, inUuid, emptyEdgesBytes)
	if err != nil {
		return []*index.KeyValue{}, err
	}
	edges = append(edges, edge)
	inUuidByte := Uint64ToBytes(inUuid)
	// add inUuid as entry for this structure from now on
	edge, err = entryUuidInsert(ctx, entryKey, txn, ph.vecEntryKey, inUuidByte)
	if err != nil {
		return []*index.KeyValue{}, err
	}
	edges = append(edges, edge)
	return edges, nil
}

// creates a new edge with the given uuid and edges. Lock must be held before calling this function
func (ph *persistentHNSW[T]) newPersistentEdgeKeyValueEntry(ctx context.Context, key []byte,
	txn index.Txn, uuid uint64, edges []byte) (*index.KeyValue, error) {
	txn.LockKey(key)
	defer txn.UnlockKey(key)
	edge := &index.KeyValue{
		Entity: uuid,
		Attr:   ph.vecKey,
		Value:  edges,
	}
	if err := txn.AddMutationWithLockHeld(ctx, key, edge); err != nil {
		return nil, err
	}
	return edge, nil
}

// addNeighbors adds the neighbors of the given uuid to the given level.
// It returns the edge created and the error if any.
func (ph *persistentHNSW[T]) addNeighbors(ctx context.Context, tc *TxnCache,
	uuid uint64, allLayerNeighbors [][]uint64) (*index.KeyValue, error) {

	txn := tc.txn
	keyPred := ph.vecKey
	key := DataKey(keyPred, uuid)
	txn.LockKey(key)
	defer txn.UnlockKey(key)
	var nnEdgesErr error
	var allLayerEdges [][]uint64
	var ok bool
	allLayerEdges, ok = ph.nodeAllEdges[uuid]
	if !ok {
		data, _ := txn.GetWithLockHeld(key)
		if data == nil {
			allLayerEdges = allLayerNeighbors
		} else {
			// all edges of nearest neighbor
			err := decodeUint64MatrixUnsafe(data.([]byte), &allLayerEdges)
			if err != nil {
				return nil, err
			}
		}
	}
	for level := 0; level < ph.maxLevels; level++ {
		allLayerEdges[level], nnEdgesErr = ph.removeDeadNodes(allLayerEdges[level], tc)
		if nnEdgesErr != nil {
			return nil, nnEdgesErr
		}
		// This adds at most efConstruction number of edges for each layer for this node
		allLayerEdges[level] = append(allLayerEdges[level], allLayerNeighbors[level]...)
	}

	// on every modification of the layer edges, add it to in mem map so you dont have to always be reading
	// from persistent storage
	ph.nodeAllEdges[uuid] = allLayerEdges
	inboundEdgesBytes := encodeUint64MatrixUnsafe(allLayerEdges)

	edge := &index.KeyValue{
		Entity: uuid,
		Attr:   ph.vecKey,
		Value:  inboundEdgesBytes,
	}
	if err := txn.AddMutationWithLockHeld(ctx, key, edge); err != nil {
		return nil, err
	}
	return edge, nil
}

// removeDeadNodes(nnEdges, tc) removes dead nodes from nnEdges and returns the new nnEdges
func (ph *persistentHNSW[T]) removeDeadNodes(nnEdges []uint64, tc *TxnCache) ([]uint64, error) {
	// TODO add a path to delete deadNodes
	if ph.deadNodes == nil {
		data, err := getDataFromKeyWithCacheType(ph.vecDead, 1, tc)
		if err != nil && err.Error() == plError {
			return []uint64{}, err
		}

		var deadNodes []uint64
		if data != nil { // if dead nodes exist, convert to []uint64
			deadNodes, err = ParseEdges(string(data.([]byte)))
			if err != nil {
				return []uint64{}, err
			}
		}

		ph.deadNodes = make(map[uint64]struct{})
		for _, n := range deadNodes {
			ph.deadNodes[n] = struct{}{}
		}
	}
	if len(ph.deadNodes) == 0 {
		return nnEdges, nil
	}

	var diff []uint64
	for _, s := range nnEdges {
		if _, ok := ph.deadNodes[s]; !ok {
			diff = append(diff, s)
			continue
		}
	}
	return diff, nil
}

func Uint64ToBytes(key uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, key)
	return b
}

func BytesToUint64(bytes []byte) uint64 {
	return binary.BigEndian.Uint64(bytes)
}

func isEqual[T c.Float](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i, val := range a {
		if val != b[i] {
			return false
		}
	}
	return true
}

// DataKey generates a data key with the given attribute and UID.
// The structure of a data key is as follows:
//
// byte 0: key type prefix (set to DefaultPrefix or ByteSplit if part of a multi-part list)
// byte 1-2: length of attr
// next len(attr) bytes: value of attr
// next byte: data type prefix (set to ByteData)
// next eight bytes: value of uid
// next eight bytes (optional): if the key corresponds to a split list, the startUid of
// the split stored in this key and the first byte will be sets to ByteSplit.
func DataKey(attr string, uid uint64) []byte {
	extra := 1 + 8 // ByteData + UID
	buf, prefixLen := generateKey(DefaultPrefix, attr, extra)

	rest := buf[prefixLen:]
	rest[0] = ByteData

	rest = rest[1:]
	binary.BigEndian.PutUint64(rest, uid)
	return buf
}

// genKey creates the key and writes the initial bytes (type byte, length of attribute,
// and the attribute itself). It leaves the rest of the key empty for further processing
// if necessary. It also returns next index from where further processing should be done.
func generateKey(typeByte byte, attr string, extra int) ([]byte, int) {
	// Separate namespace and attribute from attr and write namespace in the first 8 bytes of key.
	namespace, attr := ParseNamespaceBytes(attr)
	prefixLen := 1 + 8 + 2 + len(attr) // byteType + ns + len(pred) + pred
	buf := make([]byte, prefixLen+extra)
	buf[0] = typeByte
	AssertTrue(copy(buf[1:], namespace) == 8)
	rest := buf[9:]

	writeAttr(rest, attr)
	return buf, prefixLen
}

func ParseNamespaceBytes(attr string) ([]byte, string) {
	splits := strings.SplitN(attr, NsSeparator, 2)
	ns := make([]byte, 8)
	binary.BigEndian.PutUint64(ns, strToUint(splits[0]))
	return ns, splits[1]
}

// AssertTrue asserts that b is true. Otherwise, it would log fatal.
func AssertTrue(b bool) {
	if !b {
		log.Fatalf("%+v", errors.Errorf("Assert failed"))
	}
}

func writeAttr(buf []byte, attr string) []byte {
	AssertTrue(len(attr) < math.MaxUint16)
	binary.BigEndian.PutUint16(buf[:2], uint16(len(attr)))

	rest := buf[2:]
	AssertTrue(len(attr) == copy(rest, attr))

	return rest[len(attr):]
}

// For consistency, use base16 to encode/decode the namespace.
func strToUint(s string) uint64 {
	ns, err := strconv.ParseUint(s, 16, 64)
	Check(err)
	return ns
}

// Check logs fatal if err != nil.
func Check(err error) {
	if err != nil {
		err = errors.Wrap(err, "")
		CaptureSentryException(err)
		log.Fatalf("%+v", err)
	}
}

// CaptureSentryException sends the error report to Sentry.
func CaptureSentryException(err error) {
	if err != nil {
		sentry.CaptureException(err)
	}
}
