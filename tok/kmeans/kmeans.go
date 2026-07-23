package kmeans

import (
	"encoding/json"
	"math"
	"sync"

	"github.com/golang/glog"

	c "github.com/dgraph-io/dgraph/v25/tok/constraints"
	"github.com/dgraph-io/dgraph/v25/tok/hnsw"
	"github.com/dgraph-io/dgraph/v25/tok/index"
	"github.com/dgraph-io/dgraph/v25/x"
)

const (
	CentroidPrefix = "__centroid_"
)

type Kmeans[T c.Float] struct {
	floatBits   int
	numPasses   int
	numClusters int
	numProbes   int
	centroids   *vectorCentroids[T]
}

func CreateKMeans[T c.Float](floatBits int, pred string, numClusters, numProbes int,
	distFunc func(a, b []T, floatBits int) (T, error)) index.VectorPartitionStrat[T] {
	return &Kmeans[T]{
		floatBits:   floatBits,
		numPasses:   5,
		numClusters: numClusters,
		numProbes:   numProbes,
		centroids: &vectorCentroids[T]{
			distFunc:  distFunc,
			floatBits: floatBits,
			pred:      pred,
		},
	}
}

func (km *Kmeans[T]) AddSeedVector(vec []T) {
	km.centroids.addSeedCentroid(vec)
}

func (km *Kmeans[T]) AddVector(vec []T) error {
	return km.centroids.addVector(vec)
}

func (km *Kmeans[T]) GetCentroids() [][]T {
	return km.centroids.centroids
}

func (km *Kmeans[T]) FindIndexForSearch(c index.CacheType, vec []T) ([]int, error) {
	if km.NumPasses() == 0 {
		return []int{0}, nil
	}
	if err := km.centroids.maybeHydrate(c); err != nil {
		return nil, err
	}
	res, err := km.centroids.findNClosestCentroids(vec, km.numProbes)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		// No centroids (the index was never built): everything lives in
		// cluster 0, so that is the only shard worth probing.
		return []int{0}, nil
	}
	return res, nil
}

func (km *Kmeans[T]) FindIndexForInsert(c index.CacheType, vec []T) (int, error) {
	if km.NumPasses() == 0 {
		return 0, nil
	}
	if err := km.centroids.maybeHydrate(c); err != nil {
		return 0, err
	}
	return km.centroids.findCentroid(vec)
}

func (km *Kmeans[T]) NumPasses() int {
	return km.numPasses
}

func (km *Kmeans[T]) SetNumPasses(n int) {
	km.numPasses = n
	if n == 0 {
		// Zero passes means k-means will never run (fewer vectors than
		// seeds). Clear the seed centroids so the degenerate index persists
		// an empty centroid set and routing consistently uses cluster 0.
		km.centroids.clear()
	}
}

func (km *Kmeans[T]) NumSeedVectors() int {
	return km.numClusters
}

func (km *Kmeans[T]) StartBuildPass() {
	if km.centroids.weights == nil {
		km.centroids.randomInit()
	}
}

func (km *Kmeans[T]) EndBuildPass() {
	km.centroids.updateCentroids()
}

type vectorCentroids[T c.Float] struct {
	// mu guards centroids and the derived fields below. Build passes take the
	// write lock; routing (findCentroid, findNClosestCentroids) takes the
	// read lock, so a long-lived index can serve concurrent lookups.
	mu sync.RWMutex

	dimension  int
	numCenters int
	pred       string

	distFunc func(a, b []T, floatBits int) (T, error)

	centroids [][]T
	counts    []int64
	weights   [][]T
	mutexs    []*sync.Mutex
	floatBits int

	// hydrated records that a disk load was attempted, so a predicate whose
	// index was never built doesn't re-read badger on every routing call.
	hydrated bool
}

func (vc *vectorCentroids[T]) clear() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.centroids = nil
	vc.counts = nil
	vc.weights = nil
	vc.mutexs = nil
	vc.numCenters = 0
	vc.dimension = 0
}

func (vc *vectorCentroids[T]) findCentroid(input []T) (int, error) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.findCentroidWithLockHeld(input)
}

func (vc *vectorCentroids[T]) findCentroidWithLockHeld(input []T) (int, error) {
	minIdx := 0
	minDist := math.MaxFloat32
	for i, centroid := range vc.centroids {
		dist, err := vc.distFunc(centroid, input, vc.floatBits)
		if err != nil {
			return 0, err
		}
		if float64(dist) < minDist {
			minDist = float64(dist)
			minIdx = i
		}
	}
	return minIdx, nil
}

// maybeHydrate loads the persisted centroid set from disk into memory on
// first use. A freshly created index (mutation/query path, or any instance
// after an alpha restart) has no centroids in memory even though a build may
// have persisted them; without this load, every insert would route to
// cluster 0 and every search would probe the wrong shards. A nil CacheType
// (build path) skips hydration — the build populates centroids itself.
func (vc *vectorCentroids[T]) maybeHydrate(c index.CacheType) error {
	if c == nil {
		return nil
	}
	vc.mu.RLock()
	done := vc.hydrated || len(vc.centroids) > 0
	vc.mu.RUnlock()
	if done {
		return nil
	}

	vc.mu.Lock()
	defer vc.mu.Unlock()
	if vc.hydrated || len(vc.centroids) > 0 {
		return nil
	}
	vc.hydrated = true

	indexCountAttr := hnsw.ConcatStrings(vc.pred, CentroidPrefix)
	key := x.DataKey(indexCountAttr, 1)
	centroidsMarshalled, err := c.Get(key)
	if err != nil || len(centroidsMarshalled) == 0 {
		// No persisted centroids (the index was never built): stay in
		// cluster-0 mode until the next rebuild replaces this instance.
		glog.V(1).Infof("vector index %s: no persisted centroids (err: %v), "+
			"routing everything to cluster 0", vc.pred, err)
		return nil
	}
	glog.V(1).Infof("vector index %s: hydrated %d bytes of centroids", vc.pred, len(centroidsMarshalled))

	centroids := [][]T{}
	if err := json.Unmarshal(centroidsMarshalled, &centroids); err != nil {
		return err
	}
	vc.centroids = centroids
	return nil
}

func (vc *vectorCentroids[T]) findNClosestCentroids(input []T, n int) ([]int, error) {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	if n <= 0 || len(vc.centroids) == 0 {
		return []int{}, nil
	}
	if n >= len(vc.centroids) {
		res := make([]int, len(vc.centroids))
		for i := range res {
			res[i] = i
		}
		return res, nil
	}
	res := []int{}
	resDist := []float64{}

	for i, centroid := range vc.centroids {
		dist, err := vc.distFunc(centroid, input, vc.floatBits)
		if err != nil {
			return nil, err
		}
		if len(res) < n {
			res = append(res, i)
			resDist = append(resDist, float64(dist))
		} else {
			// Find the farthest in current top-n
			maxIdx, maxDist := 0, resDist[0]
			for j, d := range resDist {
				if d > maxDist {
					maxIdx, maxDist = j, d
				}
			}
			if float64(dist) < maxDist {
				res[maxIdx] = i
				resDist[maxIdx] = float64(dist)
			}
		}
	}
	return res, nil
}

func (vc *vectorCentroids[T]) addVector(vec []T) error {
	idx, err := vc.findCentroid(vec)
	if err != nil {
		return err
	}
	vc.mutexs[idx].Lock()
	defer vc.mutexs[idx].Unlock()
	for i := 0; i < vc.dimension; i++ {
		vc.weights[idx][i] += vec[i]
	}
	vc.counts[idx]++
	return nil
}

func (vc *vectorCentroids[T]) updateCentroids() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	x.AssertTrue(len(vc.centroids) == vc.numCenters)
	x.AssertTrue(len(vc.counts) == vc.numCenters)
	x.AssertTrue(len(vc.weights) == vc.numCenters)
	for i := 0; i < vc.numCenters; i++ {
		if vc.counts[i] == 0 {
			// No vectors were assigned to this cluster this pass. Keep the
			// previous centroid instead of dividing by zero into NaNs.
			for j := 0; j < vc.dimension; j++ {
				vc.weights[i][j] = 0
			}
			continue
		}
		for j := 0; j < vc.dimension; j++ {
			x.AssertTrue(len(vc.centroids[i]) == vc.dimension)
			x.AssertTrue(len(vc.weights[i]) == vc.dimension)
			vc.centroids[i][j] = vc.weights[i][j] / T(vc.counts[i])
			vc.weights[i][j] = 0
		}
		vc.counts[i] = 0
	}
}

func (vc *vectorCentroids[T]) randomInit() {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.dimension = len(vc.centroids[0])
	for i := range vc.centroids {
		x.AssertTrue(len(vc.centroids[i]) == vc.dimension)
	}
	vc.numCenters = len(vc.centroids)
	vc.counts = make([]int64, vc.numCenters)
	vc.weights = make([][]T, vc.numCenters)
	vc.mutexs = make([]*sync.Mutex, vc.numCenters)
	for i := 0; i < vc.numCenters; i++ {
		vc.weights[i] = make([]T, vc.dimension)
		vc.counts[i] = 0
		vc.mutexs[i] = &sync.Mutex{}
	}
}

func (vc *vectorCentroids[T]) addSeedCentroid(vec []T) {
	vc.mu.Lock()
	defer vc.mu.Unlock()
	vc.centroids = append(vc.centroids, vec)
}
