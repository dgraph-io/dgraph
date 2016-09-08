package x

import (
	"github.com/google/flatbuffers/go"

	"github.com/dgraph-io/dgraph/posting/types"
	"github.com/dgraph-io/dgraph/task"
)

// NewTaskQuery parses bytes into new task.Query. This is very fast.
func NewTaskQuery(b []byte) *task.Query {
	q := new(task.Query)
	ParseTaskQuery(q, b)
	return q
}

// ParseTaskQuery parses bytes into task.Query. This is very fast.
func ParseTaskQuery(q *task.Query, b []byte) {
	q.Init(b, flatbuffers.GetUOffsetT(b))
}

// NewTaskResult parses bytes into new task.Result. This is very fast.
func NewTaskResult(b []byte) *task.Result {
	r := new(task.Result)
	ParseTaskResult(r, b)
	return r
}

// ParseTaskResult parses bytes into task.Result. This is very fast.
func ParseTaskResult(r *task.Result, b []byte) {
	r.Init(b, flatbuffers.GetUOffsetT(b))
}

// NewPosting parses bytes into new types.Posting. This is very fast.
func NewPosting(b []byte) *types.Posting {
	p := new(types.Posting)
	ParsePosting(p, b)
	return p
}

// ParsePosting parses bytes into types.Posting. This is very fast.
func ParsePosting(p *types.Posting, b []byte) {
	p.Init(b, flatbuffers.GetUOffsetT(b))
}
