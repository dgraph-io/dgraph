// CreateFactory creates an instance of the private struct persistentIndexFactory.
// NOTE: if T and floatBits do not match in # of bits, there will be consequences.

package partitioned_hnsw

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"github.com/dgraph-io/dgraph/v25/protos/pb"
	c "github.com/dgraph-io/dgraph/v25/tok/constraints"
	hnsw "github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/tok/kmeans"
	opt "github.com/dgraph-io/dgraph/v25/tok/options"
)

const maxRouteMemoEntries = 32 << 20 // Cap routing memo at ~32M entries (~150MB per 1M vectors)

type partitionedHNSW[T c.Float] struct {
	floatBits int
	pred      string

	clusterMap      map[int]index.VectorIndex[T]
	numClusters     int
	vectorDimension int
	vecCount        int
	numPasses       int
	partition       index.VectorPartitionStrat[T]

	hnswOptions    opt.Options
	partitionStrat string

	caches        []index.CacheType
	buildPass     int
	buildSyncMaps map[int]*sync.Mutex

	routeMemo      *sync.Map    // Memoize uuid -> cluster routing during index passes
	routeMemoCount atomic.Int64 // Track memo size to enforce cap
}

func (ph *partitionedHNSW[T]) applyOptions(o opt.Options) error {
	ph.numClusters, _, _ = opt.GetOpt(o, NumClustersOpt, 1000)
	ph.vectorDimension, _, _ = opt.GetOpt(o, vectorDimension, -1)
	ph.partitionStrat, _, _ = opt.GetOpt(o, PartitionStratOpt, "kmeans")

	if ph.numClusters < 1 {
		return errors.New("numClusters must be at least 1")
	}
	if ph.partitionStrat != "kmeans" {
		return errors.New("partition strategy must be kmeans")
	}

	// numProbes is how many clusters a search visits (IVF nprobe). More
	// probes cost latency and buy recall. Default: a small slice of the
	// clusters, but never fewer than 4 (or all of them if numClusters < 4).
	defaultProbes := max(4, ph.numClusters/25)
	numProbes, _, _ := opt.GetOpt(o, NumProbesOpt, defaultProbes)
	numProbes = max(1, min(numProbes, ph.numClusters))

	ph.partition = kmeans.CreateKMeans(ph.floatBits, ph.pred, ph.numClusters, numProbes,
		hnsw.EuclideanDistanceSq[T])

	ph.buildPass = 0
	ph.numPasses = 10
	ph.hnswOptions = o
	for i := range ph.numClusters {
		factory := hnsw.CreateFactory[T](ph.floatBits)
		vi, err := factory.Create(ph.pred, ph.hnswOptions, ph.floatBits)
		if err != nil {
			return err
		}
		err = hnsw.UpdateIndexSplit(vi, i)
		if err != nil {
			return err
		}
		ph.clusterMap[i] = vi
	}
	return nil
}

func (ph *partitionedHNSW[T]) AddSeedVector(vec []T) {
	ph.partition.AddSeedVector(vec)
}

func (ph *partitionedHNSW[T]) BuildInsert(ctx context.Context, uuid uint64, vec []T) error {
	passIdx := ph.buildPass - ph.partition.NumPasses()
	if passIdx < 0 {
		return ph.partition.AddVector(vec)
	}
	// The build populated the centroids in memory; no cache needed.
	var index int
	var err error

	// During index passes with routing memo, check memo first to avoid redundant routing
	if ph.routeMemo != nil {
		if memoIdx, ok := ph.routeMemo.Load(uuid); ok {
			index = memoIdx.(int)
		} else {
			// Cache miss: compute and store (if within cap)
			index, err = ph.partition.FindIndexForInsert(nil, vec)
			if err != nil {
				return err
			}
			// Only store if we haven't exceeded the cap
			if ph.routeMemoCount.Add(1) <= maxRouteMemoEntries {
				ph.routeMemo.Store(uuid, index)
			} else {
				ph.routeMemoCount.Add(-1) // Undo the increment if we hit the cap
			}
		}
	} else {
		// No memo: compute routing normally
		index, err = ph.partition.FindIndexForInsert(nil, vec)
		if err != nil {
			return err
		}
	}

	if index%ph.numPasses != passIdx {
		return nil
	}
	ph.buildSyncMaps[index].Lock()
	defer ph.buildSyncMaps[index].Unlock()
	_, err = ph.clusterMap[index].Insert(ctx, ph.caches[index], uuid, vec)
	return err
}

func (ph *partitionedHNSW[T]) GetCentroids() [][]T {
	return ph.partition.GetCentroids()
}

func (ph *partitionedHNSW[T]) NumBuildPasses() int {
	return ph.partition.NumPasses()
}

func (ph *partitionedHNSW[T]) SetNumPasses(n int) {
	ph.partition.SetNumPasses(n)
}

func (ph *partitionedHNSW[T]) Dimension() int {
	return ph.vectorDimension
}

// SetDimension records the inferred dimension on the instance only. It
// deliberately does NOT write a vectorDimension option into the schema: the
// alter path persists the schema update after the rebuild, so an appended
// option would leak derived state into schema queries and exports — including
// a nonsensical "-1" when the predicate is empty, and duplicate entries across
// rebuilds (a predicate has exactly one dimension). The dimension is persisted
// separately as internal index metadata (see addDimensionMetaInDB) and
// re-hydrated where a fresh instance needs it.
func (ph *partitionedHNSW[T]) SetDimension(schema *pb.SchemaUpdate, dimension int) {
	ph.vectorDimension = dimension
}

func (ph *partitionedHNSW[T]) NumIndexPasses() int {
	return ph.numPasses
}

func (ph *partitionedHNSW[T]) NumThreads() int {
	return ph.numClusters
}

func (ph *partitionedHNSW[T]) NumSeedVectors() int {
	return ph.partition.NumSeedVectors()
}

func (ph *partitionedHNSW[T]) StartBuild(caches []index.CacheType) {
	ph.caches = caches
	if ph.buildPass < ph.partition.NumPasses() {
		ph.partition.StartBuildPass()
		return
	}

	// Initialize routing memo on entry to the first index pass (when centroids are frozen)
	if ph.buildPass == ph.partition.NumPasses() && ph.partition.NumPasses() > 0 {
		ph.routeMemo = &sync.Map{}
		ph.routeMemoCount.Store(0)
	}

	for i := range ph.clusterMap {
		ph.buildSyncMaps[i] = &sync.Mutex{}
		if i%ph.numPasses != (ph.buildPass - ph.partition.NumPasses()) {
			continue
		}
		ph.clusterMap[i].StartBuild([]index.CacheType{ph.caches[i]})
	}
}

func (ph *partitionedHNSW[T]) EndBuild() []int {
	res := []int{}

	if ph.buildPass >= ph.partition.NumPasses() {
		for i := range ph.clusterMap {
			if i%ph.numPasses != (ph.buildPass - ph.partition.NumPasses()) {
				continue
			}
			ph.clusterMap[i].EndBuild()
			res = append(res, i)
		}
	}

	ph.buildPass += 1

	// Free routing memo after all index passes complete
	if ph.routeMemo != nil && ph.buildPass >= ph.partition.NumPasses()+ph.numPasses {
		ph.routeMemo = nil
		ph.routeMemoCount.Store(0)
	}

	if len(res) > 0 {
		return res
	}

	if ph.buildPass < ph.partition.NumPasses() {
		ph.partition.EndBuildPass()
	}
	return []int{}
}

func (ph *partitionedHNSW[T]) Insert(ctx context.Context, txn index.CacheType, uid uint64, vec []T) ([]*index.KeyValue, error) {
	if ph.vectorDimension <= 0 {
		ph.vectorDimension = len(vec)
	}

	if len(vec) != ph.vectorDimension {
		return nil, fmt.Errorf("cannot insert vector of length %d, vector length should be %d", len(vec), ph.vectorDimension)
	}

	index, err := ph.partition.FindIndexForInsert(txn, vec)
	if err != nil {
		return nil, err
	}
	subIndex, err := ph.subIndex(index)
	if err != nil {
		return nil, err
	}
	return subIndex.Insert(ctx, txn, uid, vec)
}

// subIndex builds a fresh persistentHNSW view over cluster i's keyspace.
// Mutation and query paths intentionally get a new sub-index per call:
// persistentHNSW caches graph edges and dead nodes in per-instance maps
// scoped to one transaction's view, so sharing instances across concurrent
// operations would race and leak state between transactions. The long-lived
// state of a partitioned index is only the kmeans routing centroids.
func (ph *partitionedHNSW[T]) subIndex(i int) (index.VectorIndex[T], error) {
	factory := hnsw.CreateFactory[T](ph.floatBits)
	vi, err := factory.Create(ph.pred, ph.hnswOptions, ph.floatBits)
	if err != nil {
		return nil, err
	}
	if err := hnsw.UpdateIndexSplit(vi, i); err != nil {
		return nil, err
	}
	return vi, nil
}

// isEmptyClusterErr reports whether a per-shard search failed only because
// that cluster has no elements yet. Sparse clusters are normal for a
// partitioned index; they contribute zero results instead of failing the
// whole query.
func isEmptyClusterErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), hnsw.EmptyHNSWTreeError)
}

// searchShards fans a per-cluster search out over the routed shards with a
// bounded worker pool, collecting the union of their results. Shard errors
// abort the query except for empty clusters.
func (ph *partitionedHNSW[T]) searchShards(ctx context.Context, indexes []int,
	search func(subIndex index.VectorIndex[T]) ([]uint64, error)) ([]uint64, error) {

	res := []uint64{}
	mutex := &sync.Mutex{}
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(min(len(indexes), 2*runtime.GOMAXPROCS(0)))
	for _, index := range indexes {
		eg.Go(func() error {
			subIndex, err := ph.subIndex(index)
			if err != nil {
				return err
			}
			ids, err := search(subIndex)
			if err != nil {
				if isEmptyClusterErr(err) {
					return nil
				}
				return err
			}
			mutex.Lock()
			res = append(res, ids...)
			mutex.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return res, nil
}

func (ph *partitionedHNSW[T]) Search(ctx context.Context, txn index.CacheType, query []T, maxResults int, filter index.SearchFilter[T]) ([]uint64, error) {
	indexes, err := ph.partition.FindIndexForSearch(txn, query)
	if err != nil {
		return nil, err
	}
	res, err := ph.searchShards(ctx, indexes, func(subIndex index.VectorIndex[T]) ([]uint64, error) {
		return subIndex.Search(ctx, txn, query, maxResults, filter)
	})
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return res, nil
	}

	return ph.MergeResults(ctx, txn, res, query, maxResults, filter)
}

func (ph *partitionedHNSW[T]) SearchWithPath(ctx context.Context, txn index.CacheType, query []T, maxResults int, filter index.SearchFilter[T]) (*index.SearchPathResult, error) {
	// A path is only meaningful within a single HNSW graph, so search the
	// cluster the query vector belongs to.
	idx, err := ph.partition.FindIndexForInsert(txn, query)
	if err != nil {
		return nil, err
	}
	subIndex, err := ph.subIndex(idx)
	if err != nil {
		return nil, err
	}
	return subIndex.SearchWithPath(ctx, txn, query, maxResults, filter)
}

func (ph *partitionedHNSW[T]) SearchWithUid(ctx context.Context, txn index.CacheType, queryUid uint64, maxResults int, filter index.SearchFilter[T]) ([]uint64, error) {
	queryVec, err := hnsw.GetVectorFromUid[T](ph.pred, queryUid, ph.floatBits, txn)
	if err != nil {
		return []uint64{}, err
	}
	if len(queryVec) == 0 {
		// The query uid has no vector: nothing to search for.
		return []uint64{}, nil
	}

	// Mirror persistentHNSW.SearchWithUid: when the filter rejects the query
	// vector itself, the query uid must not appear in the results, so search
	// one extra candidate and drop it.
	shouldFilterOutQueryUid := !filter(queryVec, queryVec, queryUid)
	searchResults := maxResults
	if shouldFilterOutQueryUid {
		searchResults++
	}

	uids, err := ph.Search(ctx, txn, queryVec, searchResults, filter)
	if err != nil {
		return nil, err
	}
	if !shouldFilterOutQueryUid {
		return uids, nil
	}
	out := make([]uint64, 0, len(uids))
	for _, uid := range uids {
		if uid == queryUid {
			continue
		}
		out = append(out, uid)
	}
	if len(out) > maxResults {
		out = out[:maxResults]
	}
	return out, nil
}

func (ph *partitionedHNSW[T]) MergeResults(ctx context.Context, txn index.CacheType, list []uint64, query []T, maxResults int, filter index.SearchFilter[T]) ([]uint64, error) {
	// MergeResults only reads vectors through the cache; any sub-index view
	// can serve it (the data keys are on the unsplit predicate).
	subIndex, err := ph.subIndex(0)
	if err != nil {
		return nil, err
	}
	return subIndex.MergeResults(ctx, txn, list, query, maxResults, filter)
}

// DeadAttrForVector implements index.VectorDeadListResolver: a deleted
// vector's uid must be recorded in the dead list of the cluster that indexed
// it, or the shard's searches will keep returning it. Routing is
// deterministic for a fixed centroid set, so the delete lands where the
// insert did.
func (ph *partitionedHNSW[T]) DeadAttrForVector(c index.CacheType, vec []T) (string, error) {
	idx, err := ph.partition.FindIndexForInsert(c, vec)
	if err != nil {
		return "", err
	}
	return hnsw.SplitDeadAttr(ph.pred, idx), nil
}

// SearchWithOptions implements index.OptionalSearchOptions by fanning the
// per-call tuning parameters (ef, distance threshold) out to the routed
// shards. Without this, worker/task.go would silently drop the options for
// partitioned indexes.
func (ph *partitionedHNSW[T]) SearchWithOptions(ctx context.Context, txn index.CacheType, query []T,
	maxResults int, opts index.VectorIndexOptions[T]) ([]uint64, error) {

	filter := opts.Filter
	if filter == nil {
		filter = index.AcceptAll[T]
	}

	indexes, err := ph.partition.FindIndexForSearch(txn, query)
	if err != nil {
		return nil, err
	}
	res, err := ph.searchShards(ctx, indexes, func(subIndex index.VectorIndex[T]) ([]uint64, error) {
		if o, ok := subIndex.(index.OptionalSearchOptions[T]); ok {
			return o.SearchWithOptions(ctx, txn, query, maxResults, opts)
		}
		return subIndex.Search(ctx, txn, query, maxResults, filter)
	})
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return res, nil
	}

	return ph.MergeResults(ctx, txn, res, query, maxResults, filter)
}

// SearchWithUidAndOptions implements index.OptionalSearchOptions for the
// similar_to(pred, <uid>) query form.
func (ph *partitionedHNSW[T]) SearchWithUidAndOptions(ctx context.Context, txn index.CacheType,
	queryUid uint64, maxResults int, opts index.VectorIndexOptions[T]) ([]uint64, error) {

	filter := opts.Filter
	if filter == nil {
		filter = index.AcceptAll[T]
	}

	queryVec, err := hnsw.GetVectorFromUid[T](ph.pred, queryUid, ph.floatBits, txn)
	if err != nil {
		return []uint64{}, err
	}
	if len(queryVec) == 0 {
		return []uint64{}, nil
	}

	shouldFilterOutQueryUid := !filter(queryVec, queryVec, queryUid)
	searchResults := maxResults
	if shouldFilterOutQueryUid {
		searchResults++
	}

	uids, err := ph.SearchWithOptions(ctx, txn, queryVec, searchResults, opts)
	if err != nil {
		return nil, err
	}
	if !shouldFilterOutQueryUid {
		return uids, nil
	}
	out := make([]uint64, 0, len(uids))
	for _, uid := range uids {
		if uid == queryUid {
			continue
		}
		out = append(out, uid)
	}
	if len(out) > maxResults {
		out = out[:maxResults]
	}
	return out, nil
}
