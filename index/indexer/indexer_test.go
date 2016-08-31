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
func (d *DummyIndexer) Delete(pred, key, val string) error       { return nil }
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
