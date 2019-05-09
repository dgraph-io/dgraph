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

package live

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof" // http profiler
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/dgraph-io/badger"
	bopt "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"

	"github.com/dgraph-io/dgraph/chunker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"

	"github.com/spf13/cobra"
)

type options struct {
	dataFiles           string
	dataFormat          string
	schemaFile          string
	alpha               string
	zero                string
	concurrent          int
	batchSize           int
	clientDir           string
	ignoreIndexConflict bool
	authToken           string
	useCompression      bool
	newUids             bool
}

var (
	opt    options
	tlsCfg *tls.Config
	Live   x.SubCommand
)

func init() {
	Live.Cmd = &cobra.Command{
		Use:   "live",
		Short: "Run Dgraph live loader",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Live.Conf).Stop()
			if err := run(); err != nil {
				os.Exit(1)
			}
		},
	}
	Live.EnvPrefix = "DGRAPH_LIVE"

	flag := Live.Cmd.Flags()
	flag.StringP("files", "f", "", "Location of *.rdf(.gz) or *.json(.gz) file(s) to load")
	flag.StringP("schema", "s", "", "Location of schema file")
	flag.String("format", "", "Specify file format (rdf or json) instead of getting it from filename")
	flag.StringP("alpha", "a", "127.0.0.1:9080",
		"Comma-separated list of Dgraph alpha gRPC server addresses")
	flag.StringP("zero", "z", "127.0.0.1:5080", "Dgraph zero gRPC server address")
	flag.IntP("conc", "c", 10,
		"Number of concurrent requests to make to Dgraph")
	flag.IntP("batch", "b", 1000,
		"Number of N-Quads to send as part of a mutation.")
	flag.StringP("xidmap", "x", "", "Directory to store xid to uid mapping")
	flag.BoolP("ignore_index_conflict", "i", true,
		"Ignores conflicts on index keys during transaction")
	flag.StringP("auth_token", "t", "",
		"The auth token passed to the server for Alter operation of the schema file")
	flag.BoolP("use_compression", "C", false,
		"Enable compression on connection to alpha server")
	flag.Bool("new_uids", false,
		"Ignore UIDs in load files and assign new ones.")

	// TLS configuration
	x.RegisterClientTLSFlags(flag)
}

// Reads a single line from a buffered reader. The line is read into the
// passed in buffer to minimize allocations. This is the preferred
// method for loading long lines which could be longer than the buffer
// size of bufio.Scanner.
func readLine(r *bufio.Reader, buf *bytes.Buffer) error {
	isPrefix := true
	var err error
	for isPrefix && err == nil {
		var line []byte
		// The returned line is an pb.buffer in bufio and is only
		// valid until the next call to ReadLine. It needs to be copied
		// over to our own buffer.
		line, isPrefix, err = r.ReadLine()
		if err == nil {
			buf.Write(line)
		}
	}
	return err
}

// processSchemaFile process schema for a given gz file.
func processSchemaFile(ctx context.Context, file string, dgraphClient *dgo.Dgraph) error {
	fmt.Printf("\nProcessing schema file %q\n", file)
	if len(opt.authToken) > 0 {
		md := metadata.New(nil)
		md.Append("auth-token", opt.authToken)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	f, err := os.Open(file)
	x.CheckfNoTrace(err)
	defer f.Close()

	var reader io.Reader
	if strings.HasSuffix(strings.ToLower(file), ".gz") {
		reader, err = gzip.NewReader(f)
		x.Check(err)
	} else {
		reader = f
	}

	b, err := ioutil.ReadAll(reader)
	if err != nil {
		x.Checkf(err, "Error while reading file")
	}

	op := &api.Operation{}
	op.Schema = string(b)
	return dgraphClient.Alter(ctx, op)
}

func (l *loader) uid(val string) string {
	// Attempt to parse as a UID (in the same format that dgraph outputs - a
	// hex number prefixed by "0x"). If parsing succeeds, then this is assumed
	// to be an existing node in the graph. There is limited protection against
	// a user selecting an unassigned UID in this way - it may be assigned
	// later to another node. It is up to the user to avoid this.
	if !opt.newUids {
		if uid, err := strconv.ParseUint(val, 0, 64); err == nil {
			l.alloc.BumpTo(uid)
			return fmt.Sprintf("%#x", uid)
		}
	}

	uid := l.alloc.AssignUid(val)
	return fmt.Sprintf("%#x", uint64(uid))
}

// processFile forwards a file to the RDF or JSON processor as appropriate
func (l *loader) processFile(ctx context.Context, filename string) error {
	fmt.Printf("Processing data file %q\n", filename)

	rd, cleanup := chunker.FileReader(filename)
	defer cleanup()

	loadType := chunker.DataFormat(filename, opt.dataFormat)
	if loadType == chunker.UnknownFormat {
		if isJson, err := chunker.IsJSONData(rd); err == nil {
			if isJson {
				loadType = chunker.JsonFormat
			} else {
				return fmt.Errorf("need --format=rdf or --format=json to load %s", filename)
			}
		}
	}

	return l.processLoadFile(ctx, rd, chunker.NewChunker(loadType))
}

func (l *loader) processLoadFile(ctx context.Context, rd *bufio.Reader, ck chunker.Chunker) error {
	x.CheckfNoTrace(ck.Begin(rd))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		chunkBuf, err := ck.Chunk(rd)
		l.processChunk(chunkBuf, ck)
		if err == io.EOF {
			break
		} else {
			x.Check(err)
		}
	}
	x.CheckfNoTrace(ck.End(rd))

	return nil
}

// processChunk parses the rdf entries from the chunk, and group them into
// batches (each one containing opt.batchSize entries) and sends the batches
// to the loader.reqs channel
func (l *loader) processChunk(chunkBuf *bytes.Buffer, ck chunker.Chunker) {
	if chunkBuf == nil || chunkBuf.Len() == 0 {
		return
	}

	nqs, err := ck.Parse(chunkBuf)
	x.CheckfNoTrace(err)

	batch := make([]*api.NQuad, 0, opt.batchSize)
	for _, nq := range nqs {
		nq.Subject = l.uid(nq.Subject)
		if len(nq.ObjectId) > 0 {
			nq.ObjectId = l.uid(nq.ObjectId)
		}

		batch = append(batch, nq)

		if len(batch) >= opt.batchSize {
			mu := api.Mutation{Set: batch}
			l.reqs <- mu

			// The following would create a new batch slice. We should not use batch =
			// batch[:0], because it would end up modifying the batch array passed
			// to l.reqs above.
			batch = make([]*api.NQuad, 0, opt.batchSize)
		}
	}

	// sends the left over nqs
	if len(batch) > 0 {
		l.reqs <- api.Mutation{Set: batch}
	}
}

func setup(opts batchMutationOptions, dc *dgo.Dgraph) *loader {
	var db *badger.DB
	if len(opt.clientDir) > 0 {
		x.Check(os.MkdirAll(opt.clientDir, 0700))
		o := badger.DefaultOptions
		o.Dir = opt.clientDir
		o.ValueDir = opt.clientDir
		o.TableLoadingMode = bopt.MemoryMap
		o.SyncWrites = false

		var err error
		db, err = badger.Open(o)
		x.Checkf(err, "Error while creating badger KV posting store")
	}

	// compression with zero server actually makes things worse
	connzero, err := x.SetupConnection(opt.zero, tlsCfg, false)
	x.Checkf(err, "Unable to connect to zero, Is it running at %s?", opt.zero)

	alloc := xidmap.New(connzero, db)
	l := &loader{
		opts:     opts,
		dc:       dc,
		start:    time.Now(),
		reqs:     make(chan api.Mutation, opts.Pending*2),
		alloc:    alloc,
		db:       db,
		zeroconn: connzero,
	}

	l.requestsWg.Add(opts.Pending)
	for i := 0; i < opts.Pending; i++ {
		go l.makeRequests()
	}

	rand.Seed(time.Now().Unix())
	return l
}

func run() error {
	x.PrintVersion()
	opt = options{
		dataFiles:           Live.Conf.GetString("files"),
		dataFormat:          Live.Conf.GetString("format"),
		schemaFile:          Live.Conf.GetString("schema"),
		alpha:               Live.Conf.GetString("alpha"),
		zero:                Live.Conf.GetString("zero"),
		concurrent:          Live.Conf.GetInt("conc"),
		batchSize:           Live.Conf.GetInt("batch"),
		clientDir:           Live.Conf.GetString("xidmap"),
		ignoreIndexConflict: Live.Conf.GetBool("ignore_index_conflict"),
		authToken:           Live.Conf.GetString("auth_token"),
		useCompression:      Live.Conf.GetBool("use_compression"),
		newUids:             Live.Conf.GetBool("new_uids"),
	}
	tlsCfg, err := x.LoadClientTLSConfig(Live.Conf)
	if err != nil {
		return err
	}

	go http.ListenAndServe("localhost:6060", nil)
	ctx := context.Background()
	bmOpts := batchMutationOptions{
		Size:          opt.batchSize,
		Pending:       opt.concurrent,
		PrintCounters: true,
		Ctx:           ctx,
		MaxRetries:    math.MaxUint32,
	}

	ds := strings.Split(opt.alpha, ",")
	var clients []api.DgraphClient
	for _, d := range ds {
		conn, err := x.SetupConnection(d, tlsCfg, opt.useCompression)
		x.Checkf(err, "While trying to setup connection to Dgraph alpha %v", ds)
		defer conn.Close()

		dc := api.NewDgraphClient(conn)
		clients = append(clients, dc)
	}
	dgraphClient := dgo.NewDgraphClient(clients...)

	l := setup(bmOpts, dgraphClient)
	defer l.zeroconn.Close()

	if len(opt.schemaFile) > 0 {
		if err := processSchemaFile(ctx, opt.schemaFile, dgraphClient); err != nil {
			if err == context.Canceled {
				fmt.Printf("Interrupted while processing schema file %q\n", opt.schemaFile)
				return nil
			}
			fmt.Printf("Error while processing schema file %q: %s\n", opt.schemaFile, err)
			return err
		}
		fmt.Printf("Processed schema file %q\n\n", opt.schemaFile)
	}

	if opt.dataFiles == "" {
		return errors.New("RDF or JSON file(s) location must be specified")
	}

	filesList := x.FindDataFiles(opt.dataFiles, []string{".rdf", ".rdf.gz", ".json", ".json.gz"})
	totalFiles := len(filesList)
	if totalFiles == 0 {
		return fmt.Errorf("No data files found in %s", opt.dataFiles)
	}
	fmt.Printf("Found %d data file(s) to process\n", totalFiles)

	//	x.Check(dgraphClient.NewSyncMarks(filesList))
	errCh := make(chan error, totalFiles)
	for _, file := range filesList {
		file = strings.Trim(file, " \t")
		go func(file string) {
			errCh <- l.processFile(ctx, file)
		}(file)
	}

	// PrintCounters should be called after schema has been updated.
	if bmOpts.PrintCounters {
		go l.printCounters()
	}

	for i := 0; i < totalFiles; i++ {
		if err := <-errCh; err != nil {
			fmt.Printf("Error while processing data file %q: %s\n", filesList[i], err)
			return err
		}
	}

	close(l.reqs)
	// First we wait for requestsWg, when it is done we know all retry requests have been added
	// to retryRequestsWg. We can't have the same waitgroup as by the time we call Wait, we can't
	// be sure that all retry requests have been added to the waitgroup.
	l.requestsWg.Wait()
	l.retryRequestsWg.Wait()
	c := l.Counter()
	var rate uint64
	if c.Elapsed.Seconds() < 1 {
		rate = c.Nquads
	} else {
		rate = c.Nquads / uint64(c.Elapsed.Seconds())
	}
	// Lets print an empty line, otherwise Interrupted or Number of Mutations overwrites the
	// previous printed line.
	fmt.Printf("%100s\r", "")
	fmt.Printf("Number of TXs run            : %d\n", c.TxnsDone)
	fmt.Printf("Number of N-Quads processed  : %d\n", c.Nquads)
	fmt.Printf("Time spent                   : %v\n", c.Elapsed)
	fmt.Printf("N-Quads processed per second : %d\n", rate)

	if l.db != nil {
		l.alloc.Flush()
		l.db.Close()
	}
	return nil
}
