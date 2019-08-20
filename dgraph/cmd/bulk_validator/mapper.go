package bulkvalidator

import (
	"fmt"
	"sync"

	"github.com/dgraph-io/dgraph/chunker"
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
				fmt.Println("Error Found in file ", chunkBuf.filename, ": ", err)
			}

		}
	}()

	go func() {
		for range chunker.NQuads().Ch() {
		}
	}()
}
