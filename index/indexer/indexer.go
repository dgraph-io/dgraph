// Package indexer is an interface to indexing solutions such as Bleve.
package indexer

import (
	"sync"

	"github.com/dgraph-io/dgraph/x"
)

var (
	registry     map[string]func() Indexer
	registryLock sync.Mutex
)

// Register adds an Indexer constructor to registry. It is recommended that
// you register
func Register(name string, f func() Indexer) {
	registryLock.Lock()
	defer registryLock.Unlock()
	if registry == nil {
		registry = make(map[string]func() Indexer)
	}
	x.Assertf(registry[name] == nil, "Indexer already defined %s", name)
	registry[name] = f
}

func New(name string) Indexer {
	x.Assertf(registry[name] != nil, "Unknown indexer %s", name)
	return registry[name]()
}

// Batch allows the batching of updates to Indexer.
type Batch interface {
	Insert(pred, key, val string) error
	Delete(pred, key, val string) error
}

// Indexer adds (key, val) to index. Given val, it returns matching keys.
// We also assume that it has support for different predicates.
type Indexer interface {
	Open(dir string) error
	Close() error
	Create(dir string) error

	// Updates.
	Insert(pred, key, val string) error
	Delete(pred, key, val string) error
	NewBatch() (Batch, error)
	Batch(b Batch) error

	// Query. Returns matching keys. The keys should be sorted.
	Query(pred, val string) ([]string, error)
}
