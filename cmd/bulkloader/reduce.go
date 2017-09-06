package main

import (
	"sync/atomic"

	"github.com/dgraph-io/dgraph/protos"
)

func reduce(batch []*protos.FlatPosting, prog *progress) {
	for _ = range batch {
		// TODO: Reduction logic goes here.
		atomic.AddInt64(&prog.reduceEdgeCount, 1)
	}
}
