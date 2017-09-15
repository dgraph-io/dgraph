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
	bo "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

type options struct {
	RDFDir          string
	SchemaFile      string
	BadgerDir       string
	LeaseFile       string
	TmpDir          string
	NumGoroutines   int
	MapBufSize      int64
	SkipExpandEdges bool
	NumShards       int
	BlockRate       int
	SkipMapPhase    bool
	CleanupTmp      bool
}

type state struct {
	opt        options
	prog       *progress
	um         *uidMap
	ss         *schemaStore
	rdfChunkCh chan *bytes.Buffer
	mapFileId  uint32 // Used atomically to name the output files of the mappers.
	kv         *badger.KV
}

type loader struct {
	*state
	mappers []*mapper
}

func newLoader(opt options) *loader {
	schemaBuf, err := ioutil.ReadFile(opt.SchemaFile)
	x.Checkf(err, "Could not load schema.")
	initialSchema, err := schema.Parse(string(schemaBuf))
	x.Checkf(err, "Could not parse schema.")

	st := &state{
		opt:  opt,
		prog: newProgress(),
		um:   newUIDMap(),
		ss:   newSchemaStore(initialSchema),

		// Lots of gz readers, so not much channel buffer needed.
		rdfChunkCh: make(chan *bytes.Buffer, opt.NumGoroutines),
	}
	ld := &loader{
		state:   st,
		mappers: make([]*mapper, opt.NumGoroutines),
	}
	for i := 0; i < opt.NumGoroutines; i++ {
		ld.mappers[i] = newMapper(st)
	}
	go ld.prog.report()
	return ld
}

func readChunk(r *bufio.Reader) (*bytes.Buffer, error) {
	batch := new(bytes.Buffer)
	batch.Grow(10 << 20)
	for lineCount := 0; lineCount < 1e5; lineCount++ {
		slc, err := r.ReadSlice('\n')
		if err == io.EOF {
			batch.Write(slc)
			return batch, err
		}
		if err == bufio.ErrBufferFull {
			// This should only happen infrequently.
			batch.Write(slc)
			var str string
			str, err = r.ReadString('\n')
			if err == io.EOF {
				batch.WriteString(str)
				return batch, err
			}
			if err != nil {
				return nil, err
			}
			batch.WriteString(str)
			continue
		}
		if err != nil {
			return nil, err
		}
		batch.Write(slc)
	}
	return batch, nil
}

func findRDFFiles(dir string) []string {
	var files []string
	x.Check(filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".rdf") || strings.HasSuffix(path, ".rdf.gz") {
			files = append(files, path)
		}
		return nil
	}))
	return files
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

	var readers []*bufio.Reader
	for _, rdfFile := range findRDFFiles(ld.opt.RDFDir) {
		f, err := os.Open(rdfFile)
		x.Check(err)
		defer f.Close()
		if !strings.HasSuffix(rdfFile, ".gz") {
			readers = append(readers, bufio.NewReaderSize(f, 1<<20))
		} else {
			gzr, err := gzip.NewReader(f)
			x.Checkf(err, "Could not create gzip reader for RDF file %q.", rdfFile)
			readers = append(readers, bufio.NewReader(gzr))
		}
	}

	pending := make(chan struct{}, ld.opt.NumGoroutines)
	for _, r := range readers {
		pending <- struct{}{}
		go func(r *bufio.Reader) {
			for {
				chunkBuf, err := readChunk(r)
				if err == io.EOF {
					if chunkBuf.Len() != 0 {
						ld.rdfChunkCh <- chunkBuf
					}
					break
				}
				x.Check(err)
				ld.rdfChunkCh <- chunkBuf
			}
			<-pending
		}(r)
	}
	for i := 0; i < ld.opt.NumGoroutines; i++ {
		pending <- struct{}{}
	}

	close(ld.rdfChunkCh)
	mapperWg.Wait()

	// Allow memory to GC before the reduce phase.
	for i := range ld.mappers {
		ld.mappers[i] = nil
	}
	ld.writeLease()
	ld.um = nil
	runtime.GC()
}

func (ld *loader) writeLease() {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d\n", ld.um.lease)
	x.Check(ioutil.WriteFile(ld.opt.LeaseFile, buf.Bytes(), 0644))
}

func (ld *loader) reduceStage() {
	ld.prog.setPhase(reducePhase)

	opt := badger.DefaultOptions
	opt.Dir = ld.opt.BadgerDir
	opt.ValueDir = opt.Dir
	opt.ValueGCRunInterval = time.Hour * 100
	opt.SyncWrites = false
	opt.TableLoadingMode = bo.MemoryMap
	var err error
	ld.kv, err = badger.NewKV(&opt)
	x.Check(err)

	// Orchestrate finalisation of shufflers.
	var shuffleWg sync.WaitGroup
	shuffleWg.Add(ld.opt.NumShards)
	shuffleOutputCh := make(chan []*protos.MapEntry, 100)
	go func() {
		shuffleWg.Wait()
		close(shuffleOutputCh)
	}()

	// Start shufflers.
	mapOutputShards := ld.findMapOutputFiles()
	for _, mapOutput := range mapOutputShards {
		shuffleInputChs := make([]chan *protos.MapEntry, len(mapOutput))
		for i, mappedFile := range mapOutput {
			shuffleInputChs[i] = make(chan *protos.MapEntry, 1000)
			go readMapOutput(mappedFile, shuffleInputChs[i])
		}
		go func() {
			shufflePostings(shuffleOutputCh, shuffleInputChs, ld.prog)
			shuffleWg.Done()
		}()
	}

	// Run reducers.
	var badgerWg sync.WaitGroup
	pending := make(chan struct{}, ld.opt.NumGoroutines)
	ld.prog.debugMu.Lock()
	ld.prog.debug = func() string {
		return fmt.Sprintf("Reducers:%d ShuffleOutputLen:%d", len(pending), len(shuffleOutputCh))
	}
	ld.prog.debugMu.Unlock()
	for batch := range shuffleOutputCh {
		pending <- struct{}{}
		badgerWg.Add(1)
		go func(batch []*protos.MapEntry) {
			reduce(batch, ld.kv, ld.prog, badgerWg.Done)
			<-pending
		}(batch)
	}
	badgerWg.Wait()
}

func (ld *loader) findMapOutputFiles() [][]string {
	shards := make([][]string, ld.opt.NumShards)
	x.Checkf(filepath.Walk(ld.opt.TmpDir, func(path string, _ os.FileInfo, err error) error {
		if !strings.HasSuffix(path, ".map") {
			return nil
		}
		var shard, fileNum int
		base := filepath.Base(path)
		x.Check2(fmt.Sscanf(base, "%d_%d.map", &shard, &fileNum))
		x.AssertTruef(shard < len(shards), "map filename doesn't match number of shards: %q", base)
		shards[shard] = append(shards[shard], path)
		return nil
	}), "while looking for map output files")
	return shards
}

func (ld *loader) writeSchema() {
	ld.ss.write(ld.kv)
}

func (ld *loader) cleanup() {
	ld.prog.endSummary()
	x.Check(ld.kv.Close())
}
