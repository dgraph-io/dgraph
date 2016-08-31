// Package indexer is an interface to various indexing solutions, e.g., Bleve.
package indexer

import (
	"github.com/dgraph-io/dgraph/x"
)

var (
	registry = make(map[string]func() Indexer)
)

func Register(name string, f func() Indexer) {
	x.Assertf(registry[name] != nil, "Unknown indexer %s", name)
	registry[name] = f
}

func New(name string) Indexer {
	x.Assertf(registry[name] != nil, "Unknown indexer %s", name)
	return registry[name]()
}

type Batch interface {
	Insert(pred, key, val string)
	Delete(pred, key string)
}

type Indexer interface {
	Open(dir string)   // Open index at given directory.
	Close()            // Close Indexer.
	Create(dir string) // Create empty index at given directory.

	// Update operations.
	Insert(pred, key, val string) error
	Delete(pred, key string) error
	Batch(b Batch) error
}
