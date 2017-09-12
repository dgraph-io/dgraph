package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/table"
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
	mapBufSize    int64
}

type state struct {
	opt        options
	prog       *progress
	um         *uidMap
	ss         *schemaStore
	rdfCh      chan string
	mapEntryCh chan *protos.MapEntry
	mapFileId  uint32 // Used atomically to name the output files of the mappers.
	kv         *badger.KV
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
		mapEntryCh: make(chan *protos.MapEntry, 1000),
	}
	ld := &loader{
		state:   st,
		mappers: make([]*mapper, opt.numGoroutines),
	}
	for i := 0; i < opt.numGoroutines; i++ {
		ld.mappers[i] = &mapper{state: st}
	}
	go ld.prog.report()
	return ld
}

func readLine(r *bufio.Reader, buf *bytes.Buffer) error {
	isPrefix := true
	var err error
	for isPrefix && err == nil {
		var line []byte
		line, isPrefix, err = r.ReadLine()
		if err == nil {
			buf.Write(line)
		}
	}
	return err
}

func (ld *loader) mapStage() {
	ld.prog.setPhase(mapPhase)

	var mapperWg sync.WaitGroup
	mapperWg.Add(len(ld.mappers))
	for _, m := range ld.mappers {
		go func(m *mapper) {
			m.run()
			mapperWg.Done()
		}(m)
	}

	var readers []io.Reader
	for _, rdfFile := range strings.Split(ld.opt.rdfFiles, ",") {
		f, err := os.Open(rdfFile)
		x.Check(err)
		defer f.Close()
		if !strings.HasSuffix(rdfFile, ".gz") {
			readers = append(readers, f)
		} else {
			gzr, err := gzip.NewReader(f)
			x.Checkf(err, "Could not create gzip reader for RDF file %q.", rdfFile)
			readers = append(readers, gzr)
		}
	}
	var lineBuf bytes.Buffer
	for _, r := range readers {
		bufReader := bufio.NewReader(r)
		for {
			lineBuf.Reset()
			err := readLine(bufReader, &lineBuf)
			if err == io.EOF {
				break
			} else {
				x.Check(err)
			}
			ld.rdfCh <- lineBuf.String()
		}
	}

	close(ld.rdfCh)
	mapperWg.Wait()
	close(ld.mapEntryCh)

	// Allow memory to GC before the reduce phase.
	for i := range ld.mappers {
		ld.mappers[i] = nil
	}
	// TODO: Put lease in file.
	fmt.Println("LEASE:", ld.um.lease())
	ld.um = nil
	runtime.GC()
}

func (ld *loader) reduceStage() {
	ld.prog.setPhase(reducePhase)

	// Read output from map stage.
	var mapOutput []string
	err := filepath.Walk(ld.opt.tmpDir, func(path string, _ os.FileInfo, err error) error {
		if !strings.HasSuffix(path, ".map") {
			return nil
		}
		mapOutput = append(mapOutput, path)
		return nil
	})
	x.Checkf(err, "While walking the map output.")

	shuffleInputChs := make([]chan *protos.MapEntry, len(mapOutput))
	for i, mappedFile := range mapOutput {
		shuffleInputChs[i] = make(chan *protos.MapEntry, 1000)
		go readMapOutput(mappedFile, shuffleInputChs[i])
	}

	opt := badger.DefaultOptions
	opt.Dir = ld.opt.badgerDir
	opt.ValueDir = opt.Dir
	opt.ValueGCRunInterval = time.Hour * 100
	opt.SyncWrites = false
	opt.MapTablesTo = table.MemoryMap
	ld.kv, err = badger.NewKV(&opt)
	x.Check(err)

	// Shuffle concurrently with reduce.
	ci := &countIndexer{state: ld.state}
	// Small buffer size since each element has a lot of data.
	reduceCh := make(chan []*protos.MapEntry, 3)
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

func (ld *loader) cleanup() {
	ld.prog.endSummary()
	x.Check(ld.kv.Close())
}
