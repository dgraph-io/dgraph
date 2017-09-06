package main

import (
	"bufio"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

type options struct {
	rdfFile       string
	schemaFile    string
	badgerDir     string
	tmpDir        string
	numGoroutines int
}

type state struct {
	opt        options
	prog       *progress
	um         *uidMap
	ss         *schemaStore
	rdfCh      chan string
	postingsCh chan *protos.FlatPosting
}

type loader struct {
	*state
	mappers []*mapper
}

func newLoader(opt options) *loader {
	schemaBuf, err := ioutil.ReadFile(opt.schemaFile)
	x.Checkf(err, "Could not load schema.")
	initialSchema, err := schema.Parse(string(schemaBuf))
	x.Checkf(err, "Could not parse schema.")

	st := &state{
		opt:        opt,
		prog:       newProgress(),
		um:         newUIDMap(),
		ss:         newSchemaStore(initialSchema),
		rdfCh:      make(chan string, 1000),
		postingsCh: make(chan *protos.FlatPosting, 1000),
	}
	ld := &loader{
		state:   st,
		mappers: make([]*mapper, opt.numGoroutines),
	}
	for i := 0; i < opt.numGoroutines; i++ {
		ld.mappers[i] = &mapper{state: st}
	}
	return ld
}

func (ld *loader) run() {
	go ld.prog.report()

	var postingWriterWg sync.WaitGroup
	postingWriterWg.Add(1)

	tmpPostingsDir, err := ioutil.TempDir(ld.opt.tmpDir, "bulkloader_tmp_posting_")
	x.Check(err)
	defer func() { x.Check(os.RemoveAll(tmpPostingsDir)) }()

	var numFlatFiles int
	go func() {
		numFlatFiles = writePostings(tmpPostingsDir, ld.postingsCh, ld.prog)
		writePostings(tmpPostingsDir, ld.postingsCh, ld.prog)
		postingWriterWg.Done()
	}()

	f, err := os.Open(ld.opt.rdfFile)
	x.Checkf(err, "Could not read RDF file.")
	defer f.Close()

	var mapperWg sync.WaitGroup
	mapperWg.Add(len(ld.mappers))
	for _, m := range ld.mappers {
		go func(m *mapper) {
			m.run()
			mapperWg.Done()
		}(m)
	}

	var sc *bufio.Scanner
	if !strings.HasSuffix(ld.opt.rdfFile, ".gz") {
		sc = bufio.NewScanner(f)
	} else {
		gzr, err := gzip.NewReader(f)
		x.Checkf(err, "Could not create gzip reader for RDF file.")
		sc = bufio.NewScanner(gzr)
	}

	for i := 0; sc.Scan(); i++ {
		ld.rdfCh <- sc.Text()
	}
	x.Check(sc.Err())

	close(ld.rdfCh)
	mapperWg.Wait()
	close(ld.postingsCh)
	postingWriterWg.Wait()

	flatPostingChs := make([]chan *protos.FlatPosting, numFlatFiles)
	for i := 0; i < numFlatFiles; i++ {
		flatPostingChs[i] = make(chan *protos.FlatPosting, 1<<10)
		filename := filepath.Join(tmpPostingsDir, fmt.Sprintf("%06d.bin", i))
		go readFlatFile(filename, flatPostingChs[i])
	}
	shuffleFlatFiles(tmpPostingsDir, flatPostingChs, ld.prog)

	ld.prog.endSummary()

	fa, err := os.OpenFile("/ssd/debug.txt", os.O_CREATE|os.O_RDWR, 0644)
	x.Check(err)
	defer func() { x.Check(fa.Close()) }()
	catFlatFile(filepath.Join(tmpPostingsDir, "merged_000000.bin"), fa)
	catFlatFile(filepath.Join(tmpPostingsDir, "merged_000001.bin"), fa)
	catFlatFile(filepath.Join(tmpPostingsDir, "merged_000002.bin"), fa)
	catFlatFile(filepath.Join(tmpPostingsDir, "merged_000004.bin"), fa)
}

func catFlatFile(filename string, w io.Writer) {
	x.Check2(w.Write([]byte(filename)))
	x.Check2(w.Write([]byte{'\n'}))
	ch := make(chan *protos.FlatPosting)
	go func() {
		readFlatFile(filename, ch)
	}()
	for p := range ch {
		x.Check2(w.Write([]byte(hex.Dump(p.Key))))
		x.Check2(w.Write([]byte{'\n'}))
	}
}
