// Package dummy contains a dummy Indexer for testing purposes.
package memtable

import (
	"fmt"
	"sort"

	"github.com/dgraph-io/dgraph/index/indexer"
	"github.com/dgraph-io/dgraph/x"
)

// Indexer implements indexer.Indexer. It does not talk to disk at all.
type Indexer struct {
	m map[string]predIndex
}

type predIndex map[string]uidSet
type uidSet map[string]struct{}

type indexJob struct {
	del            bool
	pred, key, val string
}

type batch struct {
	job []*indexJob
}

func init() {
	x.AddInit(func() {
		// This registration will also ensure at compile-time that our Indexer
		// satisfies the interface indexer.Indexer.
		indexer.Register("memtable", New)
	})
}

func New() indexer.Indexer {
	return &Indexer{make(map[string]predIndex)}
}

func (x *Indexer) NewBatch() (indexer.Batch, error) {
	return &batch{}, nil
}

func (x *Indexer) Open(dir string) error   { return nil }
func (x *Indexer) Close() error            { return nil }
func (x *Indexer) Create(dir string) error { return nil }

func (x *Indexer) Insert(pred, key, val string) error {
	pi := x.m[pred]
	if pi == nil {
		pi = make(map[string]uidSet)
		x.m[pred] = pi
	}
	us := pi[val]
	if us == nil {
		us = make(map[string]struct{})
		pi[val] = us
	}
	us[key] = struct{}{}
	fmt.Println(pred)
	fmt.Println(val)
	fmt.Println(us)
	return nil
}

func (x *Indexer) Delete(pred, key, val string) error {
	pi := x.m[pred]
	if pi == nil {
		// No predicate.
		return nil
	}
	us := pi[val]
	if us == nil {
		// Value is not present.
		return nil
	}
	delete(us, key)
	return nil
}

func (x *Indexer) Query(pred, val string) ([]string, error) {
	fmt.Println("~~Query")
	fmt.Println(x.m)
	pi := x.m[pred]
	if pi == nil {
		return nil, nil
	}
	us := pi[val]
	if us == nil {
		return nil, nil
	}
	// Return "us"'s keys sorted.
	keys := make([]string, 0, len(us))
	for k, _ := range us {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func (b *batch) Insert(pred, key, val string) error {
	b.job = append(b.job, &indexJob{
		pred: pred,
		key:  key,
		val:  val,
	})
	return nil
}

func (b *batch) Delete(pred, key, val string) error {
	b.job = append(b.job, &indexJob{
		del:  true,
		pred: pred,
		key:  key,
		val:  val,
	})
	return nil
}
func (x *Indexer) Batch(b indexer.Batch) error {
	bb := b.(*batch)
	for _, job := range bb.job {
		if job.del {
			x.Delete(job.pred, job.key, job.val)
		} else {
			x.Insert(job.pred, job.key, job.val)
		}
	}
	return nil
}
