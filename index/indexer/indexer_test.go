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
	"strconv"
	"sync"
	"testing"
)

type DummyIndexer struct{}

func NewDummyIndexer() Indexer {
	return &DummyIndexer{}
}

func (d *DummyIndexer) Open(dir string) error                    { return nil }
func (d *DummyIndexer) Close() error                             { return nil }
func (d *DummyIndexer) Create(dir string) error                  { return nil }
func (d *DummyIndexer) Insert(pred, key, val string) error       { return nil }
func (d *DummyIndexer) Remove(pred, key string) error            { return nil }
func (d *DummyIndexer) NewBatch() (Batch, error)                 { return nil, nil }
func (d *DummyIndexer) Batch(b Batch) error                      { return nil }
func (d *DummyIndexer) Query(pred, val string) ([]string, error) { return nil, nil }

func TestRegistry(t *testing.T) {
	Register("dummy", NewDummyIndexer)
	Register("dummy2", NewDummyIndexer)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			Register(strconv.Itoa(i), NewDummyIndexer)
			wg.Done()
		}(i)
	}
	wg.Wait()
	if len(registry) != 102 {
		t.Fatalf("Expected %d registrations got %d", 102, len(registry))
	}
}
