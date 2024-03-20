package hnsw

import (
	"context"
	"fmt"
	"strings"
	"time"

	c "github.com/dgraph-io/dgraph/tok/constraints"
	"github.com/dgraph-io/dgraph/tok/index"
	opt "github.com/dgraph-io/dgraph/tok/options"
	"github.com/pkg/errors"
)

type persistentHNSW[T c.Float] struct {
	maxLevels      int
	efConstruction int
	efSearch       int
	pred           string
	vecEntryKey    string
	vecKey         string
	vecDead        string
	simType        SimilarityType[T]
	floatBits      int
	// nodeAllEdges[65443][1][3] indicates the 3rd neighbor in the first
	// layer for uuid 65443. The result will be a neighboring uuid.
	nodeAllEdges map[uint64][][]uint64
	visitedUids  []uint64
}

func (ph *persistentHNSW[T]) applyOptions(o opt.Options) error {
	if o.Specifies(ExponentOpt) {
		// Adjust defaults based on exponent.
		exponent, _, _ := opt.GetOpt(o, ExponentOpt, 3)

		if !o.Specifies(MaxLevelsOpt) {
			o.SetOpt(MaxLevelsOpt, exponent)
		}

		if !o.Specifies(EfConstructionOpt) {
			o.SetOpt(EfConstructionOpt, 6*exponent)
		}

		if !o.Specifies(EfSearchOpt) {
			o.SetOpt(EfConstructionOpt, 9*exponent)
		}
	}

	var err error
	ph.maxLevels, _, err = opt.GetOpt(o, MaxLevelsOpt, 3)
	if err != nil {
		return err
	}
	ph.efConstruction, _, err = opt.GetOpt(o, EfConstructionOpt, 18)
	if err != nil {
		return err
	}
	ph.efSearch, _, err = opt.GetOpt(o, EfSearchOpt, 27)
	if err != nil {
		return err
	}
	simType, foundSimType := opt.GetInterfaceOpt(o, MetricOpt)
	if foundSimType {
		okSimType, ok := simType.(SimilarityType[T])
		if !ok {
			return fmt.Errorf("cannot cast %T to SimilarityType", simType)
		}
		ph.simType = okSimType
	} else {
		ph.simType = SimilarityType[T]{indexType: Euclidian, distanceScore: euclidianDistanceSq[T],
			insortHeap: insortPersistentHeapAscending[T], isBetterScore: isBetterScoreForDistance[T]}
	}
	return nil
}

func (ph *persistentHNSW[T]) emptyFinalResultWithError(e error) (
	*index.SearchPathResult, error) {
	return index.NewSearchPathResult(), e
}

func (ph *persistentHNSW[T]) emptySearchResultWithError(e error) (*searchLayerResult[T], error) {
	return newLayerResult[T](0), e
}

// fillNeighborEdges(uuid, c, edges) will "fill" edges with the neighbors for
// all levels associated with given uuid and CacheType.
// It returns true when we were able to find the node (either in cache or
// in persistent store) and false otherwise.
// (Of course, it may also return an error if a problem was encountered).
func (ph *persistentHNSW[T]) fillNeighborEdges(uuid uint64, c index.CacheType, edges *[][]uint64) (bool, error) {
	var ok bool
	*edges, ok = ph.nodeAllEdges[uuid]
	if ok {
		return true, nil
	}

	ok, err := populateEdgeDataFromKeyWithCacheType(ph.vecKey, uuid, c, edges)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	// add this to in mem storage of uid -> edges
	ph.nodeAllEdges[uuid] = *edges
	return true, nil
}

// searchPersistentLayer searches a layer of the hnsw graph for the nearest
// neighbors of the query vector and returns the traversal path and the nearest
// neighbors
func (ph *persistentHNSW[T]) searchPersistentLayer(
	c index.CacheType,
	level int,
	entry uint64,
	startVec, query []T,
	entryIsFilteredOut bool,
	expectedNeighbors int,
	filter index.SearchFilter[T]) (*searchLayerResult[T], error) {
	r := newLayerResult[T](level)

	bestDist, err := ph.simType.distanceScore(startVec, query, ph.floatBits)
	r.markFirstDistanceComputation()
	if err != nil {
		return ph.emptySearchResultWithError(err)
	}
	best := minPersistentHeapElement[T]{
		value:       bestDist,
		index:       entry,
		filteredOut: entryIsFilteredOut,
	}
	r.setFirstPathNode(best)
	//create set using map to append to on future visited nodes
	ph.visitedUids = append(ph.visitedUids, best.index)
	candidateHeap := *buildPersistentHeapByInit([]minPersistentHeapElement[T]{best})
	for candidateHeap.Len() != 0 {
		currCandidate := candidateHeap.Pop().(minPersistentHeapElement[T])
		if r.numNeighbors() < expectedNeighbors &&
			ph.simType.isBetterScore(r.lastNeighborScore(), currCandidate.value) {
			// If the "worst score" in our neighbors list is deemed to have
			// a better score than the current candidate -- and if we have at
			// least our expected number of nearest results -- we discontinue
			// the search.
			// Note that while this is faithful to the published
			// HNSW algorithms insofar as we stop when we reach a local
			// minimum, it leaves something to be desired in terms of
			// guarantees of getting best results.
			break
		}
		var allLayerEdges [][]uint64

		found, err := ph.fillNeighborEdges(currCandidate.index, c, &allLayerEdges)
		if err != nil {
			return ph.emptySearchResultWithError(err)
		}
		if !found {
			continue
		}
		currLayerEdges := allLayerEdges[level]
		currLayerEdges = diff(currLayerEdges, ph.visitedUids)
		var eVec []T
		for i := range currLayerEdges {
			// iterate over candidate's neighbors distances to get
			// best ones
			_ = ph.getVecFromUid(currLayerEdges[i], c, &eVec)
			// intentionally ignoring error -- we catch it
			// indirectly via eVec == nil check.
			if len(eVec) == 0 {
				continue
			}
			currDist, err := ph.simType.distanceScore(eVec, query, ph.floatBits)
			ph.visitedUids = append(ph.visitedUids, currLayerEdges[i])
			r.incrementDistanceComputations()
			if err != nil {
				return ph.emptySearchResultWithError(err)
			}
			filteredOut := !filter(query, eVec, currLayerEdges[i])
			currElement := initPersistentHeapElement(
				currDist, currLayerEdges[i], filteredOut)
			nodeVisited := r.nodeVisited(*currElement)
			if !nodeVisited {
				r.addToVisited(*currElement)

				// If we have not yet found k candidates, we can consider
				// any candidate. Otherwise, only consider those that
				// are better than our current k nearest neighbors.
				// Note that the "numNeighbors" function is a bit tricky:
				// If we previously added to the heap M elements that should
				// be filtered out, we ignore M elements in the numNeighbors
				// check! In this way, we can make sure to allow in up to
				// expectedNeighbors "unfiltered" elements.
				if ph.simType.isBetterScore(currDist, r.lastNeighborScore()) ||
					r.numNeighbors() < expectedNeighbors {
					candidateHeap.Push(*currElement)
					r.addPathNode(*currElement, ph.simType, expectedNeighbors)
				}
			}
		}
	}
	return r, nil
}

// Search searches the hnsw graph for the nearest neighbors of the query vector
// and returns the traversal path and the nearest neighbors
func (ph *persistentHNSW[T]) Search(ctx context.Context, c index.CacheType, query []T,
	maxResults int, filter index.SearchFilter[T]) (nnUids []uint64, err error) {
	r, err := ph.SearchWithPath(ctx, c, query, maxResults, filter)
	return r.Neighbors, err
}

// Search searches the hnsw graph for the nearest neighbors of the query uid
// and returns the traversal path and the nearest neighbors
func (ph *persistentHNSW[T]) SearchWithUid(ctx context.Context, c index.CacheType, queryUid uint64,
	maxResults int, filter index.SearchFilter[T]) (nnUids []uint64, err error) {
	var queryVec []T
	err = ph.getVecFromUid(queryUid, c, &queryVec)
	if err != nil {
		if strings.Contains(err.Error(), plError) {
			// No vector. return empty result
			return []uint64{}, nil
		}
		return []uint64{}, err
	}

	if len(queryVec) == 0 {
		// No vector. return empty result
		return []uint64{}, nil
	}

	shouldFilterOutQueryVec := !filter(queryVec, queryVec, queryUid)

	// how normal search works is by cotinuously searching higher layers
	// for the best entry node to the last layer since we already know the
	// best entry node (since it already exists in the lowest level), we
	// can just search the last layer and return the results.
	r, err := ph.searchPersistentLayer(
		c, ph.maxLevels-1, queryUid, queryVec, queryVec,
		shouldFilterOutQueryVec, maxResults, filter)
	for _, n := range r.neighbors {
		nnUids = append(nnUids, n.index)
	}
	return nnUids, err
}

// There will be times when the entry node has been deleted. In that case, we want to make a new node
// the first vector.
func (ph *persistentHNSW[T]) calculateNewEntryVec(
	ctx context.Context,
	c index.CacheType,
	startVec *[]T) (uint64, error) {

	itr, err := c.Find([]byte(ph.pred), func(value []byte) bool {
		index.BytesAsFloatArray(value, startVec, ph.floatBits)
		return len(*startVec) != 0
	})

	if err != nil {
		return 0, errors.Wrapf(err, "HNSW tree has no elements")
	}
	if itr == 0 {
		return itr, errors.New("HNSW tree has no elements")
	}

	return itr, nil
}

func (ph *persistentHNSW[T]) PickStartNode(
	ctx context.Context,
	c index.CacheType,
	startVec *[]T) (uint64, error) {

	data, err := getDataFromKeyWithCacheType(ph.vecEntryKey, 1, c)
	if err != nil {
		if strings.Contains(err.Error(), plError) {
			// The index might be empty
			return ph.calculateNewEntryVec(ctx, c, startVec)
		}
		return 0, err
	}

	entry := BytesToUint64(data.([]byte))
	err = ph.getVecFromUid(entry, c, startVec)
	if err != nil {
		fmt.Println(err)
	}

	if len(*startVec) == 0 {
		return ph.calculateNewEntryVec(ctx, c, startVec)
	}
	return entry, err
}

// SearchWithPath allows persistentHNSW to implement index.OptionalIndexSupport.
// See index.OptionalIndexSupport.SearchWithPath for more info.
func (ph *persistentHNSW[T]) SearchWithPath(
	ctx context.Context,
	c index.CacheType,
	query []T,
	maxResults int,
	filter index.SearchFilter[T]) (r *index.SearchPathResult, err error) {
	start := time.Now().UnixMilli()
	r = index.NewSearchPathResult()

	// 0-profile_vector_entry
	var startVec []T
	entry, err := ph.PickStartNode(ctx, c, &startVec)
	if err != nil {
		return ph.emptyFinalResultWithError(err)
	}

	// Calculates best entry for last level (maxLevels-1) by searching each
	// layer and using new best entry.
	for level := 0; level < ph.maxLevels-1; level++ {
		if isEqual(startVec, query) {
			break
		}
		filterOut := !filter(query, startVec, entry)
		layerResult, err := ph.searchPersistentLayer(
			c, level, entry, startVec, query, filterOut, ph.efSearch, filter)
		if err != nil {
			return ph.emptyFinalResultWithError(err)
		}
		layerResult.updateFinalMetrics(r)
		entry = layerResult.bestNeighbor().index
		layerResult.updateFinalPath(r)
		err = ph.getVecFromUid(entry, c, &startVec)
		if err != nil {
			return ph.emptyFinalResultWithError(err)
		}
	}
	filterOut := !filter(query, startVec, entry)
	layerResult, err := ph.searchPersistentLayer(
		c, ph.maxLevels-1, entry, startVec, query, filterOut, maxResults, filter)
	if err != nil {
		return ph.emptyFinalResultWithError(err)
	}
	layerResult.updateFinalMetrics(r)
	layerResult.updateFinalPath(r)
	layerResult.addFinalNeighbors(r)
	t := time.Now().UnixMilli()
	elapsed := t - start
	r.Metrics[searchTime] = uint64(elapsed)
	return r, nil
}

// InsertToPersistentStorage inserts a node into the hnsw graph and returns the
// traversal path and the edges created
func (ph *persistentHNSW[T]) Insert(ctx context.Context, c index.CacheType,
	inUuid uint64, inVec []T) ([]*index.KeyValue, error) {
	tc, ok := c.(*TxnCache)
	if !ok {
		return []*index.KeyValue{}, nil
	}
	_, edges, err := ph.insertHelper(ctx, tc, inUuid, inVec)
	return edges, err
}

// InsertToPersistentStorage inserts a node into the hnsw graph and returns the
// traversal path and the edges created
func (ph *persistentHNSW[T]) insertHelper(ctx context.Context, tc *TxnCache,
	inUuid uint64, inVec []T) ([]minPersistentHeapElement[T], []*index.KeyValue, error) {

	// return all the new edges created at all HNSW levels
	var startVec []T
	entry, edges, err := ph.createEntryAndStartNodes(ctx, tc, inUuid, &startVec)
	if err != nil || len(edges) > 0 {
		return []minPersistentHeapElement[T]{}, edges, err
	}

	if entry == inUuid {
		// something interesting is you physically cannot add duplicate nodes,
		// it'll just overwrite w the same info
		// only situation where you can add duplicate nodes is if your
		// mutation adds the same node as entry
		return []minPersistentHeapElement[T]{}, []*index.KeyValue{}, nil
	}

	// startVecs: vectors used to calc where to start up until inLevel,
	// nns: nearest neighbors to return,
	// visited: all visited nodes
	// var nns []minPersistentHeapElement[T]
	visited := []minPersistentHeapElement[T]{}
	inLevel := getInsertLayer(ph.maxLevels) // calculate layer to insert node at (randomized every time)
	var layerErr error

	for level := 0; level < inLevel; level++ {
		// perform insertion for layers [level, max_level) only, when level < inLevel just find better start
		err := ph.getVecFromUid(entry, tc, &startVec)
		if err != nil {
			return []minPersistentHeapElement[T]{}, []*index.KeyValue{}, err
		}
		layerResult, err := ph.searchPersistentLayer(tc, level, entry, startVec,
			inVec, false, 1, index.AcceptAll[T])
		if err != nil {
			return []minPersistentHeapElement[T]{}, []*index.KeyValue{}, err
		}
		entry = layerResult.bestNeighbor().index
	}

	emptyEdges := make([][]uint64, ph.maxLevels)
	_, err = ph.addNeighbors(ctx, tc, inUuid, emptyEdges)
	if err != nil {
		return []minPersistentHeapElement[T]{}, []*index.KeyValue{}, err
	}

	var outboundEdgesAllLayers = make([][]uint64, ph.maxLevels)
	var inboundEdgesAllLayersMap = make(map[uint64][][]uint64)
	nnUidArray := []uint64{}
	for level := inLevel; level < ph.maxLevels; level++ {
		err := ph.getVecFromUid(entry, tc, &startVec)
		if err != nil {
			return []minPersistentHeapElement[T]{}, []*index.KeyValue{}, err
		}
		layerResult, err := ph.searchPersistentLayer(tc, level, entry, startVec,
			inVec, false, ph.efConstruction, index.AcceptAll[T])
		if err != nil {
			return []minPersistentHeapElement[T]{}, []*index.KeyValue{}, layerErr
		}

		nns := layerResult.neighbors
		for i := 0; i < len(nns); i++ {
			nnUidArray = append(nnUidArray, nns[i].index)
			inboundEdgesAllLayersMap[nns[i].index] = make([][]uint64, ph.maxLevels)
			inboundEdgesAllLayersMap[nns[i].index][level] =
				append(inboundEdgesAllLayersMap[nns[i].index][level], inUuid)
			// add nn to outboundEdges.
			// These should already be correctly ordered.
			outboundEdgesAllLayers[level] =
				append(outboundEdgesAllLayers[level], nns[i].index)
		}
	}
	edge, err := ph.addNeighbors(ctx, tc, inUuid, outboundEdgesAllLayers)
	for i := 0; i < len(nnUidArray); i++ {
		edge, err := ph.addNeighbors(
			ctx, tc, nnUidArray[i], inboundEdgesAllLayersMap[nnUidArray[i]])
		if err != nil {
			return []minPersistentHeapElement[T]{}, []*index.KeyValue{}, err
		}
		edges = append(edges, edge)
	}
	if err != nil {
		return []minPersistentHeapElement[T]{}, []*index.KeyValue{}, err
	}
	edges = append(edges, edge)

	return visited, edges, nil
}
