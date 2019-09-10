package validator

import (
	"sync"
	"sync/atomic"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/golang/glog"
)

type mapper struct {
	*state
}

func newMapper(st *state) *mapper {
	return &mapper{
		state: st,
	}
}

func (m *mapper) run(inputFormat chunker.InputFormat, wg *sync.WaitGroup) {
	chunker := chunker.NewChunker(inputFormat, 10000)

	go func() {
		defer wg.Done()

		for chunkBuf := range m.readerChunkCh {
			if err := chunker.Parse(chunkBuf.chunk); err != nil {
				atomic.CompareAndSwapUint32(&m.foundError, 0, 1)
				glog.Errorf("Error Found in file %s: %s\n", chunkBuf.filename, err)
			}

		}
	}()

	go func() {
		for range chunker.NQuads().Ch() {
		}
	}()
}
