package query

import (
	"log"

	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/gogo/protobuf/proto"
)

type Codec struct{}

func (c *Codec) Marshal(v interface{}) ([]byte, error) {
	r, ok := v.(*graph.Response)
	if !ok {
		log.Fatalf("Invalid type of value: %+v", v)
	}

	b, err := proto.Marshal(r)
	if err != nil {
		return []byte{}, err
	}

	select {
	// Passing onto to channel which would put it into the sync pool.
	case nodeCh <- r.N:
	default:
	}

	return b, nil
}

func (c *Codec) Unmarshal(data []byte, v interface{}) error {
	n, ok := v.(*graph.Request)
	if !ok {
		log.Fatalf("Invalid type of value: %+v", v)
	}

	return proto.Unmarshal(data, n)
}

func (c *Codec) String() string {
	return "query.Codec"
}
