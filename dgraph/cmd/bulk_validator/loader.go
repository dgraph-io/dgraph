package bulkvalidator

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/chunker"

	"github.com/dgraph-io/dgraph/x"
)

type options struct {
	DataFiles     string
	SchemaFile    string
	TmpDir        string
	NumGoroutines int
	CleanupTmp    bool
	DataFormat    string
}

type state struct {
	opt           options
	readerChunkCh chan *bytes.Buffer
	foundError    bool
}

type loader struct {
	*state
	mappers []*mapper
}

func newLoader(opt options) *loader {
	st := &state{
		opt:           opt,
		readerChunkCh: make(chan *bytes.Buffer, opt.NumGoroutines),
	}
	ld := &loader{
		state:   st,
		mappers: make([]*mapper, opt.NumGoroutines),
	}

	for i := 0; i < opt.NumGoroutines; i++ {
		ld.mappers[i] = newMapper(st)
	}

	return ld
}

func (ld *loader) mapStage() {
	files := x.FindDataFiles(ld.opt.DataFiles, []string{".rdf", ".rdf.gz", ".json", ".json.gz"})
	if len(files) == 0 {
		fmt.Println("No data files found in %s.", ld.opt.DataFiles)
		os.Exit(1)
	}

	loadType := chunker.DataFormat(files[0], ld.opt.DataFormat)
	if loadType == chunker.UnknownFormat {
		// Dont't try to detect JSON input in bulk loader.
		fmt.Printf("Need --format=rdf or --format=json to load %s", files[0])
		os.Exit(1)
	}

	var mapperWg sync.WaitGroup
	mapperWg.Add(len(ld.mappers))
	for _, m := range ld.mappers {
		go func(m *mapper) {
			m.run(loadType)
			mapperWg.Done()
		}(m)
	}

	thr := y.NewThrottle(ld.opt.NumGoroutines)
	for i, file := range files {
		x.Check(thr.Do())
		fmt.Printf("Processing file (%d out of %d): %s\n", i+1, len(files), file)

		go func(file string) {
			defer thr.Done(nil)

			r, cleanup := chunker.FileReader(file)
			defer cleanup()

			chunker := chunker.NewChunker(loadType, 1000)
			x.Check(chunker.Begin(r))
			for {
				chunkBuf, err := chunker.Chunk(r)
				if chunkBuf != nil && chunkBuf.Len() > 0 {
					ld.readerChunkCh <- chunkBuf
				}
				if err == io.EOF {
					break
				} else if err != nil {
					x.Check(err)
				}
			}
			x.Check(chunker.End(r))
		}(file)
	}
	x.Check(thr.Finish())

	close(ld.readerChunkCh)
	mapperWg.Wait()

	if !ld.foundError {
		fmt.Println("No Errors found. All inputs files are valid.")
	}

	for i := range ld.mappers {
		ld.mappers[i] = nil
	}
}
