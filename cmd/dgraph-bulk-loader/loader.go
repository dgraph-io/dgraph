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
	"sync/atomic"

	"github.com/dgraph-io/badger"
	bo "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"
)

type options struct {
	RDFDir        string
	SchemaFile    string
	DgraphsDir    string
	LeaseFile     string
	TmpDir        string
	NumGoroutines int
	MapBufSize    int64
	ExpandEdges   bool
	BlockRate     int
	SkipMapPhase  bool
	CleanupTmp    bool
	NumShufflers  int
	Version       bool
	StoreXids     bool

	MapShards    int
	ReduceShards int

	shardOutputDirs []string
}

type state struct {
	opt        options
	prog       *progress
	um         *xidmap.XidMap
	up         *uidProvider
	ss         *schemaStore
	sm         *shardMap
	rdfChunkCh chan *bytes.Buffer
	mapFileId  uint32 // Used atomically to name the output files of the mappers.
	kvs        []*badger.DB
}

type loader struct {
	*state
	mappers []*mapper
	xidKV   *badger.DB
}

func newLoader(opt options) *loader {
	st := &state{
		opt:  opt,
		prog: newProgress(),
		ss:   newSchemaStore(readSchema(opt.SchemaFile), opt),
		sm:   newShardMap(opt.MapShards),

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

func readSchema(filename string) []*protos.SchemaUpdate {
	f, err := os.Open(filename)
	x.Check(err)
	defer f.Close()
	var r io.Reader = f
	if filepath.Ext(filename) == ".gz" {
		r, err = gzip.NewReader(f)
		x.Check(err)
	}

	buf, err := ioutil.ReadAll(r)
	x.Check(err)

	initialSchema, err := schema.Parse(string(buf))
	x.Check(err)
	return initialSchema
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

type uidProvider uint64

func (p *uidProvider) ReserveUidRange() (start, end uint64, err error) {
	const uidChunk = 1e5
	end = atomic.AddUint64((*uint64)(p), uidChunk)
	start = end - uidChunk + 1
	return
}

func (ld *loader) mapStage() {
	ld.prog.setPhase(mapPhase)

	xidDir := filepath.Join(ld.opt.TmpDir, "xids")
	x.Check(os.Mkdir(xidDir, 0755))
	opt := badger.DefaultOptions
	opt.SyncWrites = false
	opt.TableLoadingMode = bo.MemoryMap
	opt.Dir = xidDir
	opt.ValueDir = xidDir
	var err error
	ld.xidKV, err = badger.Open(opt)
	x.Check(err)
	ld.up = new(uidProvider)
	ld.um = xidmap.New(ld.xidKV, ld.up, xidmap.Options{
		NumShards: 1 << 10,
		LRUSize:   1 << 19,
	})

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

	thr := x.NewThrottle(ld.opt.NumGoroutines)
	for _, r := range readers {
		thr.Start()
		go func(r *bufio.Reader) {
			defer thr.Done()
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
		}(r)
	}
	thr.Wait()

	close(ld.rdfChunkCh)
	mapperWg.Wait()

	// Allow memory to GC before the reduce phase.
	for i := range ld.mappers {
		ld.mappers[i] = nil
	}
	ld.writeLease()
	x.Check(ld.xidKV.Close())
	ld.um = nil
	runtime.GC()
}

func (ld *loader) writeLease() {
	// Obtain a fresh uid range - since uids are allocated in increasing order,
	// the start of the new range can be used as the lease.
	lease, _, err := ld.up.ReserveUidRange()
	// Cannot give an error because uid ranges are produced by incrementing an integer.
	x.AssertTrue(err == nil)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d\n", lease)
	x.Check(ioutil.WriteFile(ld.opt.LeaseFile, buf.Bytes(), 0644))
}

type shuffleOutput struct {
	kv         *badger.DB
	mapEntries []*protos.MapEntry
}

func (ld *loader) reduceStage() {
	ld.prog.setPhase(reducePhase)

	shuffleOutputCh := make(chan shuffleOutput, 100)
	go func() {
		shuf := shuffler{state: ld.state, output: shuffleOutputCh}
		shuf.run()
	}()

	redu := reducer{
		state:     ld.state,
		input:     shuffleOutputCh,
		writesThr: x.NewThrottle(100),
	}
	redu.run()
}

func (ld *loader) writeSchema() {
	for _, kv := range ld.kvs {
		ld.ss.write(kv)
	}
}

func (ld *loader) cleanup() {
	for _, kv := range ld.kvs {
		x.Check(kv.Close())
	}
	ld.prog.endSummary()
}
