package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
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
	"github.com/dgraph-io/dgraph/xidmap"
	"google.golang.org/grpc"
)

type options struct {
	RDFDir        string
	SchemaFile    string
	DgraphsDir    string
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
	ZeroAddr      string

	MapShards    int
	ReduceShards int

	shardOutputDirs []string
}

type state struct {
	opt        options
	prog       *progress
	xids       *xidmap.XidMap
	uidFetcher *zeroUidFetcher
	schema     *schemaStore
	shards     *shardMap
	rdfChunkCh chan *bytes.Buffer
	mapFileId  uint32 // Used atomically to name the output files of the mappers.
	dbs        []*badger.ManagedDB
	writeTs    uint64 // All badger writes use this timestamp
}

type loader struct {
	*state
	mappers []*mapper
	xidDB   *badger.DB
}

func newLoader(opt options) *loader {
	zeroConn, err := grpc.Dial(opt.ZeroAddr, grpc.WithInsecure())
	x.Check(err)
	zero := protos.NewZeroClient(zeroConn)
	st := &state{
		opt:        opt,
		prog:       newProgress(),
		uidFetcher: &zeroUidFetcher{client: zero, ch: make(chan uidRangeResponse)},
		shards:     newShardMap(opt.MapShards),
		// Lots of gz readers, so not much channel buffer needed.
		rdfChunkCh: make(chan *bytes.Buffer, opt.NumGoroutines),
		writeTs:    getWriteTimestamp(zero),
	}
	go st.uidFetcher.fetchUids()
	st.schema = newSchemaStore(readSchema(opt.SchemaFile), opt, st)
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

func getWriteTimestamp(zero protos.ZeroClient) uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ts, err := zero.Timestamps(ctx, &protos.Num{Val: 1})
	x.Check(x.Wrapf(err, "error communicating with dgraphzero, is it running?"))
	return ts.GetStartId()
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

type uidRangeResponse struct {
	uids *protos.AssignedIds
	err  error
}

type zeroUidFetcher struct {
	client protos.ZeroClient
	ch     chan uidRangeResponse
}

func (z *zeroUidFetcher) fetchUids() {
	for {
		// Use a very long timeout since a failed request is fatal.
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		// Assuming a loading rate of 1M RDFs/sec, an upper bound on the number
		// of UIDs we need to assign is 1M/sec. 100k uids per request results
		// in an average 10 requests/sec to dgraphzero.
		const uidChunk = 1e5
		uids, err := z.client.AssignUids(ctx, &protos.Num{Val: uidChunk})
		cancel()
		z.ch <- uidRangeResponse{
			uids: uids,
			err:  x.Wrapf(err, "error communicating with dgraphzero, is it running?"),
		}
	}
}

func (z *zeroUidFetcher) ReserveUidRange() (start, end uint64, err error) {
	response := <-z.ch
	return response.uids.GetStartId(), response.uids.GetEndId(), response.err
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
	ld.xidDB, err = badger.Open(opt)
	x.Check(err)
	ld.xids = xidmap.New(ld.xidDB, ld.uidFetcher, xidmap.Options{
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
	x.Check(ld.xidDB.Close())
	ld.xids = nil
	runtime.GC()
}

type shuffleOutput struct {
	db         *badger.ManagedDB
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
	for _, db := range ld.dbs {
		ld.schema.write(db)
	}
}

func (ld *loader) cleanup() {
	for _, db := range ld.dbs {
		x.Check(db.Close())
	}
	ld.prog.endSummary()
}
