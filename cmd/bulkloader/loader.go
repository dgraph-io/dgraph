package main

import (
	"bufio"
	"compress/gzip"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

type options struct {
	rdfFile    string
	schemaFile string
	badgerDir  string
	tmpDir     string
	workers    int
}

type loader struct {
	opt        options
	prog       *progress
	rdfCh      chan string
	mappers    []*mapper
	postingsCh chan *protos.FlatPosting
}

func newLoader(opt options) *loader {
	schemaBuf, err := ioutil.ReadFile(opt.schemaFile)
	x.Checkf(err, "Could not load schema.")
	initialSchema, err := schema.Parse(string(schemaBuf))
	x.Checkf(err, "Could not parse schema.")

	a := &loader{
		opt:        opt,
		prog:       newProgress(),
		rdfCh:      make(chan string, 1<<10),
		mappers:    make([]*mapper, opt.workers),
		postingsCh: make(chan *protos.FlatPosting, 1<<10),
	}
	x.Check(err)

	um := newUIDMap()
	ss := newSchemaStore(initialSchema)

	for i := 0; i < opt.workers; i++ {
		a.mappers[i] = &mapper{a.rdfCh, um, ss, a.prog, a.postingsCh}
	}
	return a
}

func (a *loader) run() {

	go a.prog.report()

	var postingWriterWg sync.WaitGroup
	postingWriterWg.Add(1)
	tmpPostingsDir, err := ioutil.TempDir(a.opt.tmpDir, "bulkloader_tmp_posting_")
	x.Check(err)
	defer func() { x.Check(os.RemoveAll(tmpPostingsDir)) }()
	go func() {
		writePostings(tmpPostingsDir, a.postingsCh, a.prog)
		postingWriterWg.Done()
	}()

	f, err := os.Open(a.opt.rdfFile)
	x.Checkf(err, "Could not read RDF file.")
	defer f.Close()

	var mapperWg sync.WaitGroup
	mapperWg.Add(len(a.mappers))
	for _, m := range a.mappers {
		m := m
		go func() {
			m.run()
			mapperWg.Done()
		}()
	}

	var sc *bufio.Scanner
	if !strings.HasSuffix(a.opt.rdfFile, ".gz") {
		sc = bufio.NewScanner(f)
	} else {
		gzr, err := gzip.NewReader(f)
		x.Checkf(err, "Could not create gzip reader for RDF file.")
		sc = bufio.NewScanner(gzr)
	}

	for i := 0; sc.Scan(); i++ {
		a.rdfCh <- sc.Text()
	}
	x.Check(sc.Err())

	close(a.rdfCh)
	mapperWg.Wait()
	close(a.postingsCh)
	postingWriterWg.Wait()
	a.prog.endSummary()
}
