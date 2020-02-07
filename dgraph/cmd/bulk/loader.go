/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package bulk

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"hash/adler32"
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
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"
	"google.golang.org/grpc"
)

type options struct {
	RDFDir           string
	JSONDir          string
	SchemaFile       string
	DgraphsDir       string
	TmpDir           string
	NumGoroutines    int
	MapBufSize       int64
	ExpandEdges      bool
	SkipMapPhase     bool
	CleanupTmp       bool
	NumShufflers     int
	Version          bool
	StoreXids        bool
	ZeroAddr         string
	HttpAddr         string
	IgnoreErrors     bool
	CustomTokenizers string

	MapShards    int
	ReduceShards int

	shardOutputDirs []string
}

type state struct {
	opt           options
	prog          *progress
	xids          *xidmap.XidMap
	schema        *schemaStore
	shards        *shardMap
	readerChunkCh chan *bytes.Buffer
	mapFileId     uint32 // Used atomically to name the output files of the mappers.
	dbs           []*badger.DB
	writeTs       uint64 // All badger writes use this timestamp
}

type loader struct {
	*state
	mappers []*mapper
	xidDB   *badger.DB
	zero    *grpc.ClientConn
}

func newLoader(opt options) *loader {
	fmt.Printf("Connecting to zero at %s\n", opt.ZeroAddr)
	zero, err := grpc.Dial(opt.ZeroAddr,
		grpc.WithBlock(),
		grpc.WithInsecure(),
		grpc.WithTimeout(time.Minute))
	x.Checkf(err, "Unable to connect to zero, Is it running at %s?", opt.ZeroAddr)
	st := &state{
		opt:    opt,
		prog:   newProgress(),
		shards: newShardMap(opt.MapShards),
		// Lots of gz readers, so not much channel buffer needed.
		readerChunkCh: make(chan *bytes.Buffer, opt.NumGoroutines),
		writeTs:       getWriteTimestamp(zero),
	}
	st.schema = newSchemaStore(readSchema(opt.SchemaFile), opt, st)
	ld := &loader{
		state:   st,
		mappers: make([]*mapper, opt.NumGoroutines),
		zero:    zero,
	}
	for i := 0; i < opt.NumGoroutines; i++ {
		ld.mappers[i] = newMapper(st)
	}
	go ld.prog.report()
	return ld
}

func getWriteTimestamp(zero *grpc.ClientConn) uint64 {
	client := pb.NewZeroClient(zero)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		ts, err := client.Timestamps(ctx, &pb.Num{Val: 1})
		cancel()
		if err == nil {
			return ts.GetStartId()
		}
		fmt.Printf("Error communicating with dgraph zero, retrying: %v", err)
		time.Sleep(time.Second)
	}
}

func readSchema(filename string) []*pb.SchemaUpdate {
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

func findDataFiles(dir string, ext string) []string {
	var files []string
	x.Check(filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ext) || strings.HasSuffix(path, ext+".gz") {
			files = append(files, path)
		}
		return nil
	}))
	return files
}

type uidRangeResponse struct {
	uids *pb.AssignedIds
	err  error
}

func (ld *loader) mapStage() {
	ld.prog.setPhase(mapPhase)

	xidDir := filepath.Join(ld.opt.TmpDir, "xids")
	x.Check(os.Mkdir(xidDir, 0755))
	opt := badger.DefaultOptions("")
	opt.SyncWrites = false
	opt.TableLoadingMode = bo.MemoryMap
	opt.Dir = xidDir
	opt.ValueDir = xidDir
	var err error
	ld.xidDB, err = badger.Open(opt)
	x.Check(err)
	ld.xids = xidmap.New(ld.xidDB, ld.zero, xidmap.Options{
		NumShards: 1 << 10,
		LRUSize:   1 << 19,
	})

	var files []string
	var ext string
	var loaderType int
	if ld.opt.RDFDir != "" {
		loaderType = rdfInput
		ext = ".rdf"
		files = findDataFiles(ld.opt.RDFDir, ext)
	} else {
		loaderType = jsonInput
		ext = ".json"
		files = findDataFiles(ld.opt.JSONDir, ext)
	}

	if len(files) == 0 {
		fmt.Printf("No *%s files found.\n", ext)
		os.Exit(1)
	}

	var mapperWg sync.WaitGroup
	mapperWg.Add(len(ld.mappers))
	for _, m := range ld.mappers {
		go func(m *mapper) {
			m.run(loaderType)
			mapperWg.Done()
		}(m)
	}

	// This is the main map loop.
	thr := x.NewThrottle(ld.opt.NumGoroutines)
	for i, file := range files {
		thr.Start()
		fmt.Printf("Processing file (%d out of %d): %s\n", i+1, len(files), file)
		chunker := newChunker(loaderType)
		go func(file string) {
			defer thr.Done()

			f, err := os.Open(file)
			x.Check(err)
			defer f.Close()

			var r *bufio.Reader
			if !strings.HasSuffix(file, ".gz") {
				r = bufio.NewReaderSize(f, 1<<20)
			} else {
				gzr, err := gzip.NewReader(f)
				x.Checkf(err, "Could not create gzip reader for file %q.", file)
				r = bufio.NewReaderSize(gzr, 1<<20)
			}
			x.Check(chunker.begin(r))
			for {
				chunkBuf, err := chunker.chunk(r)
				if chunkBuf != nil && chunkBuf.Len() > 0 {
					ld.readerChunkCh <- chunkBuf
				}
				if err == io.EOF {
					break
				} else if err != nil {
					x.Check(err)
				}
			}
			x.Check(chunker.end(r))
		}(file)
	}
	thr.Wait()

	close(ld.readerChunkCh)
	mapperWg.Wait()

	// Allow memory to GC before the reduce phase.
	for i := range ld.mappers {
		ld.mappers[i] = nil
	}
	ld.xids.EvictAll()
	x.Check(ld.xidDB.Close())
	ld.xids = nil
	runtime.GC()
}

type shuffleOutput struct {
	db         *badger.DB
	mapEntries []*pb.MapEntry
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
	numDBs := uint32(len(ld.dbs))
	preds := make([][]string, numDBs)

	// Get all predicates that have data in some DB.
	m := make(map[string]struct{})
	for i, db := range ld.dbs {
		preds[i] = ld.schema.getPredicates(db)
		for _, p := range preds[i] {
			m[p] = struct{}{}
		}
	}

	// Find any predicates that don't have data in any DB
	// and distribute them among all the DBs.
	for p := range ld.schema.m {
		if _, ok := m[p]; !ok {
			i := adler32.Checksum([]byte(p)) % numDBs
			preds[i] = append(preds[i], p)
		}
	}

	// Write out each DB's final predicate list.
	for i, db := range ld.dbs {
		ld.schema.write(db, preds[i])
	}
}

func (ld *loader) cleanup() {
	for _, db := range ld.dbs {
		x.Check(db.Close())
	}
	ld.prog.endSummary()
}
