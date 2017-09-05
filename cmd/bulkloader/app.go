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

type app struct {
	opt        options
	prog       *progress
	rdfCh      chan string
	workers    []*worker
	postingsCh chan *protos.DenormalisedPosting
}

func newApp(opt options) *app {

	schemaBuf, err := ioutil.ReadFile(opt.schemaFile)
	x.Checkf(err, "Could not load schema.")
	initialSchema, err := schema.Parse(string(schemaBuf))
	x.Checkf(err, "Could not parse schema.")

	a := &app{
		opt:        opt,
		prog:       newProgress(),
		rdfCh:      make(chan string, 1<<10),
		workers:    make([]*worker, opt.workers),
		postingsCh: make(chan *protos.DenormalisedPosting, 1<<10),
	}
	x.Check(err)

	um := newUIDMap()
	ss := newSchemaStore(initialSchema)

	for i := 0; i < opt.workers; i++ {
		a.workers[i] = newWorker(a.rdfCh, um, ss, a.prog, a.postingsCh)
	}
	return a
}

func (a *app) run() {

	go a.prog.reportProgress()

	var postingWriterWG sync.WaitGroup
	postingWriterWG.Add(1)
	tmpPostingsDir, err := ioutil.TempDir(a.opt.tmpDir, "bulkloader_tmp_posting_")
	x.Check(err)
	defer func() { x.Check(os.RemoveAll(tmpPostingsDir)) }()
	go func() {
		writeDenormalisedPostings(tmpPostingsDir, a.postingsCh, a.prog)
		postingWriterWG.Done()
	}()

	f, err := os.Open(a.opt.rdfFile)
	x.Checkf(err, "Could not read RDF file.")
	defer f.Close()

	var workerWG sync.WaitGroup
	workerWG.Add(len(a.workers))
	for _, w := range a.workers {
		w := w
		go func() {
			w.run()
			workerWG.Done()
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
	workerWG.Wait()
	close(a.postingsCh)
	postingWriterWG.Wait()
	a.prog.endSummary()
}
