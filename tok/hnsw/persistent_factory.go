/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package hnsw

import (
	"fmt"
	"sync"

	c "github.com/hypermodeinc/dgraph/v25/tok/constraints"
	"github.com/hypermodeinc/dgraph/v25/tok/index"
	opt "github.com/hypermodeinc/dgraph/v25/tok/options"
	"github.com/pkg/errors"
)

const (
	ExponentOpt       string = "exponent"
	MaxLevelsOpt      string = "maxLevels"
	EfConstructionOpt string = "efConstruction"
	EfSearchOpt       string = "efSearch"
	MetricOpt         string = "metric"
	Hnsw              string = "hnsw"
)

// persistentIndexFactory is an in memory implementation of the IndexFactory interface.
// indexMap is an in memory map that corresponds an index name with a corresponding VectorIndex.
// In persistentIndexFactory, the VectorIndex will be of type HNSW.
type persistentIndexFactory[T c.Float] struct {
	// TODO: Can we kill the map?? This should disappear once server
	//       restarts, correct? So at the very least, this is not dependable.
	indexMap  map[string]index.VectorIndex[T]
	floatBits int
	mu        sync.RWMutex
}

// CreateFactory creates an instance of the private struct persistentIndexFactory.
// NOTE: if T and floatBits do not match in # of bits, there will be consequences.
func CreateFactory[T c.Float](floatBits int) index.IndexFactory[T] {
	f := &persistentIndexFactory[T]{
		indexMap:  map[string]index.VectorIndex[T]{},
		floatBits: floatBits,
	}
	return f
}

// Implements NamedFactory interface for use as a plugin.
func (hf *persistentIndexFactory[T]) Name() string { return Hnsw }

func (hf *persistentIndexFactory[T]) GetOptions(o opt.Options) string {
	return GetPersistantOptions[T](o)
}

func (hf *persistentIndexFactory[T]) isNameAvailableWithLock(name string) bool {
	_, nameUsed := hf.indexMap[name]
	return !nameUsed
}

// hf.AllowedOptions() allows persistentIndexFactory to implement the
// IndexFactory interface (see vector-indexer/index/index.go for details).
// We define here options for exponent, maxLevels, efSearch, efConstruction,
// and metric.
func (hf *persistentIndexFactory[T]) AllowedOptions() opt.AllowedOptions {
	retVal := opt.NewAllowedOptions()
	retVal.AddIntOption(ExponentOpt).
		AddIntOption(MaxLevelsOpt).
		AddIntOption(EfConstructionOpt).
		AddIntOption(EfSearchOpt)
	getSimFunc := func(optValue string) (any, error) {
		if optValue != Euclidean && optValue != Cosine && optValue != DotProd {
			return nil, errors.New(fmt.Sprintf("Can't create a vector index for %s", optValue))
		}
		return GetSimType[T](optValue, hf.floatBits), nil
	}

	retVal.AddCustomOption(MetricOpt, getSimFunc)
	return retVal
}

// Create is an implementation of the IndexFactory interface function, invoked by an HNSWIndexFactory
// instance. It takes in a string name and a VectorSource implementation, and returns a VectorIndex and error
// flag. It creates an HNSW instance using the index name and populates other parts of the HNSW struct such as
// multFactor, maxLevels, efConstruction, maxNeighbors, and efSearch using struct parameters.
// It then populates the HNSW graphs using the InsertChunk function until there are no more items to populate.
// Finally, the function adds the name and hnsw object to the in memory map and returns the object.
func (hf *persistentIndexFactory[T]) Create(
	name string,
	o opt.Options,
	floatBits int,
	split int) (index.VectorIndex[T], error) {
	hf.mu.Lock()
	defer hf.mu.Unlock()
	return hf.createWithLock(name, o, floatBits, split)
}

func (hf *persistentIndexFactory[T]) createWithLock(
	name string,
	o opt.Options,
	floatBits int,
	split int) (index.VectorIndex[T], error) {
	if !hf.isNameAvailableWithLock(fmt.Sprintf("%s-%d", name, split)) {
		err := errors.New("index with name " + name + " already exists")
		return nil, err
	}
	retVal := &persistentHNSW[T]{
		pred:         name,
		vecEntryKey:  ConcatStrings(name, VecEntry, fmt.Sprintf("_%d", split)),
		vecKey:       ConcatStrings(name, VecKeyword, fmt.Sprintf("_%d", split)),
		vecDead:      ConcatStrings(name, VecDead, fmt.Sprintf("_%d", split)),
		floatBits:    floatBits,
		nodeAllEdges: map[uint64][][]uint64{},
	}
	err := retVal.applyOptions(o)
	if err != nil {
		return nil, err
	}
	hf.indexMap[name] = retVal
	return retVal, nil
}

// Find is an implementation of the IndexFactory interface function, invoked by an persistentIndexFactory
// instance. It returns the VectorIndex corresponding with a string name using the in memory map.
func (hf *persistentIndexFactory[T]) Find(name string) (index.VectorIndex[T], error) {
	hf.mu.RLock()
	defer hf.mu.RUnlock()
	return hf.findWithLock(name)
}

func (hf *persistentIndexFactory[T]) findWithLock(name string) (index.VectorIndex[T], error) {
	vecInd := hf.indexMap[name]
	return vecInd, nil
}

// Remove is an implementation of the IndexFactory interface function, invoked by an persistentIndexFactory
// instance. It removes the VectorIndex corresponding with a string name using the in memory map.
func (hf *persistentIndexFactory[T]) Remove(name string) error {
	hf.mu.Lock()
	defer hf.mu.Unlock()
	return hf.removeWithLock(name)
}

func (hf *persistentIndexFactory[T]) removeWithLock(name string) error {
	delete(hf.indexMap, name)
	return nil
}

// CreateOrReplace is an implementation of the IndexFactory interface funciton,
// invoked by an persistentIndexFactory. It checks if a VectorIndex
// correpsonding with name exists. If it does, it removes it, and replaces it
// via the Create function using the passed VectorSource. If the VectorIndex
// does not exist, it creates that VectorIndex corresponding with the name using
// the VectorSource.
func (hf *persistentIndexFactory[T]) CreateOrReplace(
	name string,
	o opt.Options,
	floatBits int,
	split int) (index.VectorIndex[T], error) {
	hf.mu.Lock()
	defer hf.mu.Unlock()
	vi, err := hf.findWithLock(name)
	if err != nil {
		return nil, err
	}
	if vi != nil {
		err = hf.removeWithLock(name)
		if err != nil {
			return nil, err
		}
	}
	return hf.createWithLock(name, o, floatBits, split)
}
