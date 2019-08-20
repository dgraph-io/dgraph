package bulkvalidator

import (
	"fmt"
	"sync"

	"github.com/dgraph-io/dgraph/chunker"

	"github.com/dgraph-io/dgraph/protos/pb"
)

type mapper struct {
	*state
	mePool *sync.Pool
}

func newMapper(st *state) *mapper {
	return &mapper{
		mePool: &sync.Pool{
			New: func() interface{} {
				return &pb.MapEntry{}
			},
		},
		state: st,
	}
}

func (m *mapper) run(inputFormat chunker.InputFormat) {
	chunker := chunker.NewChunker(inputFormat, 1000)

	go func() {
		for chunkBuf := range m.readerChunkCh {
			if err := chunker.Parse(chunkBuf); err != nil {
				m.foundError = true
				fmt.Println(err.Error())
			}
		}

	}()
}
