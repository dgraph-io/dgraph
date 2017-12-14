/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package bulk

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
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/protos/intern"
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
	SkipMapPhase  bool
	CleanupTmp    bool
	NumShufflers  int
	Version       bool
	StoreXids     bool
	ZeroAddr      string
	HttpAddr      string

	MapShards    int
	ReduceShards int

	shardOutputDirs []string
}

type state struct {
	opt        options
	prog       *progress
	xids       *xidmap.XidMap
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
	zeros   *xidmap.ZeroPool
}

func newLoader(opt options) *loader {
	st := &state{
		opt:    opt,
		prog:   newProgress(),
		shards: newShardMap(opt.MapShards),
		// Lots of gz readers, so not much channel buffer needed.
		rdfChunkCh: make(chan *bytes.Buffer, opt.NumGoroutines),
	}
	st.schema = newSchemaStore(readSchema(opt.SchemaFile), opt, st)
	ld := &loader{
		state:   st,
		mappers: make([]*mapper, opt.NumGoroutines),
		zeros: xidmap.NewZeroPool(func(addr string) (*grpc.ClientConn, error) {
			return grpc.Dial(addr, grpc.WithInsecure())
		}, opt.ZeroAddr),
	}
	ld.state.writeTs = ld.getWriteTimestamp()
	for i := 0; i < opt.NumGoroutines; i++ {
		ld.mappers[i] = newMapper(st)
	}
	go ld.prog.report()
	return ld
}

func (ld *loader) getWriteTimestamp() uint64 {
	for {
		client, err := ld.zeros.Leader()
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			ts, err := client.Timestamps(ctx, &intern.Num{Val: 1})
			cancel()
			if err == nil {
				return ts.GetStartId()
			}
		}
		x.Printf("error communicating with dgraph zero, retrying: %v", err)
		time.Sleep(time.Second)
	}
}

func readSchema(filename string) []*intern.SchemaUpdate {
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
	uids *api.AssignedIds
	err  error
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
	ld.xids = xidmap.New(ld.xidDB, ld.zeros, xidmap.Options{
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
	mapEntries []*intern.MapEntry
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
