package main

import (
	"fmt"

	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
	"github.com/google/flatbuffers/go"
)

func main() {
	fmt.Println("Hello World")

	b := flatbuffers.NewBuilder(0)

	intersect := []uint64{1, 2, 3}
	intersectOffset := x.UidlistOffset(b, intersect)

	// Finish building UidList before starting to build Query.
	task.QueryStart(b)
	task.QueryAddIntersect(b, intersectOffset)
	b.Finish(task.QueryEnd(b))

	out := b.FinishedBytes()
	var q task.Query
	x.ParseTaskQuery(&q, out)
	uidList := q.Intersect(nil)
	fmt.Println(uidList.UidsLength())
}
