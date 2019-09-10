package validator

import (
	"bytes"
	"io"
	"sync"

	"github.com/dgraph-io/badger/y"
	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type options struct {
	DataFiles     string
	TmpDir        string
	NumGoroutines int
	CleanupTmp    bool
	DataFormat    string
}

type readerChunk struct {
	chunk    *bytes.Buffer
	filename string
}

type state struct {
	opt           options
	readerChunkCh chan readerChunk
	foundError    uint32
}

type loader struct {
	*state
	mappers []*mapper
}

func newLoader(opt options) *loader {
	st := &state{
		opt:           opt,
		readerChunkCh: make(chan readerChunk, opt.NumGoroutines),
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
		x.Fatalf("No data files found in %s\n", ld.opt.DataFiles)
	}

	loadType := chunker.DataFormat(files[0], ld.opt.DataFormat)
	if loadType == chunker.UnknownFormat {
		// Dont't try to detect JSON input in bulk loader.
		x.Fatalf("Need --format=rdf or --format=json to load %s\n", files[0])
	}

	var mapperWg sync.WaitGroup
	mapperWg.Add(len(ld.mappers))
	for _, m := range ld.mappers {
		go m.run(loadType, &mapperWg)
	}

	thr := y.NewThrottle(ld.opt.NumGoroutines)
	for _, file := range files {
		x.Check(thr.Do())

		go func(file string) {
			defer thr.Done(nil)
			glog.Infof("Processing file %s\n", file)

			r, cleanup := chunker.FileReader(file)
			defer cleanup()

			chunker := chunker.NewChunker(loadType, 1000)
			x.Check(chunker.Begin(r))
			for {
				chunkBuf, err := chunker.Chunk(r)
				if chunkBuf != nil && chunkBuf.Len() > 0 {
					ld.readerChunkCh <- readerChunk{
						chunk:    chunkBuf,
						filename: file,
					}
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

	if ld.foundError == 0 {
		glog.Infof("No errors found. All inputs files are valid.\n")
	}

	for i := range ld.mappers {
		ld.mappers[i] = nil
	}
}
