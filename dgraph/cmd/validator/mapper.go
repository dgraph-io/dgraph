package validator

import (
	"sync"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/prometheus/common/log"
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
				m.foundError = true
				log.Errorf("Error Found in file %s: %s\n", chunkBuf.filename, err)
			}

		}
	}()

	go func() {
		for range chunker.NQuads().Ch() {
		}
	}()
}
