/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package indexer

import (
	"sync"

	"github.com/dgraph-io/dgraph/x"
)

var (
	registry     map[string]func() Indexer
	registryLock sync.Mutex
)

// Register adds an Indexer constructor to registry. If you need to call this
// function, we recommend you do it in your package init.
func Register(name string, f func() Indexer) {
	registryLock.Lock()
	defer registryLock.Unlock()
	if registry == nil {
		registry = make(map[string]func() Indexer)
	}
	x.Assertf(registry[name] == nil, "Indexer already defined %s", name)
	registry[name] = f
}

// New returns a new Indexer object given the string name of the Indexer.
func New(name string) Indexer {
	x.Assertf(registry[name] != nil, "Unknown indexer %s", name)
	return registry[name]()
}

// Batch allows the batching of updates to Indexer.
type Batch interface {
	Size() int
	Reset()
	Insert(pred, key, val string) error
	Remove(pred, key string) error
}

// Indexer adds (key, val) to index. Given val, it returns matching keys.
// We also assume that it has support for different predicates.
// We expect Indexer to be a pointer to some struct.
type Indexer interface {
	Open(dir string) error
	Close() error
	Create(dir string) error

	NewBatch() (Batch, error)
	Apply(b Batch) error

	// Query. Returns matching keys. The keys should be sorted.
	Query(pred, val string) ([]string, error)
}
