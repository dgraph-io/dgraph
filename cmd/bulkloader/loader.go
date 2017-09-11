package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/table"
	"github.com/dgraph-io/dgraph/bp128"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

type options struct {
	rdfFiles      string
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
	kv         *badger.KV
}

type loader struct {
	*state
	mappers   []*mapper
	mapOutput []string
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

func (ld *loader) mapStage() {
	go ld.prog.report()

	var postingWriterWg sync.WaitGroup
	postingWriterWg.Add(1)

	tmpPostingsDir, err := ioutil.TempDir(ld.opt.tmpDir, "bulkloader_tmp_posting_")
	x.Check(err)

	go func() {
		ld.mapOutput = writeMapOutput(tmpPostingsDir, ld.postingsCh, ld.prog)
		postingWriterWg.Done()
	}()

	var mapperWg sync.WaitGroup
	mapperWg.Add(len(ld.mappers))
	for _, m := range ld.mappers {
		go func(m *mapper) {
			m.run()
			mapperWg.Done()
		}(m)
	}

	var scanners []*bufio.Scanner
	for _, rdfFile := range strings.Split(ld.opt.rdfFiles, ",") {
		f, err := os.Open(rdfFile)
		x.Check(err)
		defer f.Close()
		var sc *bufio.Scanner
		if !strings.HasSuffix(rdfFile, ".gz") {
			sc = bufio.NewScanner(f)
		} else {
			gzr, err := gzip.NewReader(f)
			x.Checkf(err, "Could not create gzip reader for RDF file.")
			sc = bufio.NewScanner(gzr)
		}
		scanners = append(scanners, sc)
	}
	for _, sc := range scanners {
		for i := 0; sc.Scan(); i++ {
			ld.rdfCh <- sc.Text()
		}
		x.Check(sc.Err())
	}

	close(ld.rdfCh)
	mapperWg.Wait()
	close(ld.postingsCh)
	postingWriterWg.Wait()
}

func (ld *loader) reduceStage() {
	// Read from map stage.
	shuffleInputChs := make([]chan *protos.FlatPosting, len(ld.mapOutput))
	for i, mappedFile := range ld.mapOutput {
		shuffleInputChs[i] = make(chan *protos.FlatPosting, 1000)
		go readMapOutput(mappedFile, shuffleInputChs[i])
	}

	opt := badger.DefaultOptions
	opt.Dir = ld.opt.badgerDir
	opt.ValueDir = opt.Dir
	opt.ValueGCRunInterval = time.Hour * 100
	opt.SyncWrites = false
	opt.MapTablesTo = table.MemoryMap
	var err error
	ld.kv, err = badger.NewKV(&opt)
	x.Check(err)

	// Shuffle concurrently with reduce.
	ci := &countIndexer{state: ld.state}
	// Small buffer size since each element has a lot of data.
	reduceCh := make(chan []*protos.FlatPosting, 3)
	go shufflePostings(reduceCh, shuffleInputChs, ld.prog, ci)

	// Reduce stage.
	pending := make(chan struct{}, ld.opt.numGoroutines)
	var reduceWg sync.WaitGroup
	for batch := range reduceCh {
		pending <- struct{}{}
		reduceWg.Add(1)
		go func() {
			reduce(batch, ld.kv, ld.prog)
			<-pending
			reduceWg.Done()
		}()
	}
	reduceWg.Wait()
	ci.wait()
}

func (ld *loader) writeSchema() {
	ld.ss.write(ld.kv)
}

func (ld *loader) writeLease() {
	// TODO: Come back to this after dgraphzero. The approach will change.
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], ld.um.lease())
	p := &protos.Posting{
		Uid:         math.MaxUint64,
		Value:       buf[:],
		ValType:     protos.Posting_INT,
		PostingType: protos.Posting_VALUE,
	}
	pl := &protos.PostingList{
		Postings: []*protos.Posting{p},
		Uids:     bp128.DeltaPack([]uint64{math.MaxUint64}),
	}
	plBuf, err := pl.Marshal()
	x.Check(err)
	x.Check(ld.kv.Set(x.DataKey("_lease_", 1), plBuf, 0x00))
}

func (ld *loader) cleanup() {
	ld.prog.endSummary()
	x.Check(ld.kv.Close())
	if len(ld.mapOutput) > 0 {
		dir := filepath.Dir(ld.mapOutput[0])
		x.Check(os.RemoveAll(dir))
	}
}
