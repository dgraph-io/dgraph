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

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"

	"github.com/spf13/cobra"
)

type options struct {
	dataFiles           string
	schemaFile          string
	dgraph              string
	zero                string
	concurrent          int
	batchSize           int
	clientDir           string
	ignoreIndexConflict bool
	authToken           string
	useCompression      bool
	keyFields           string
}

var opt options
var tlsConf x.TLSHelperConfig

var Live x.SubCommand

var keyFields []string

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
	flag.StringP("rdfs", "r", "", "Location of RDF or JSON files to load")
	flag.StringP("schema", "s", "", "Location of schema file")
	flag.StringP("dgraph", "d", "127.0.0.1:9080", "Dgraph alpha gRPC server address")
	flag.StringP("zero", "z", "127.0.0.1:5080", "Dgraph zero gRPC server address")
	flag.IntP("conc", "c", 10,
		"Number of concurrent requests to make to Dgraph")
	flag.IntP("batch", "b", 1000,
		"Number of N-Quads to send as part of a mutation.")
	flag.StringP("xidmap", "x", "", "Directory to store xid to uid mapping")
	flag.BoolP("ignore_index_conflict", "i", true,
		"Ignores conflicts on index keys during transaction")
	flag.StringP("auth_token", "a", "",
		"The auth token passed to the server for Alter operation of the schema file")
	flag.BoolP("use_compression", "C", false,
		"Enable compression on connection to alpha server")
	flag.StringP("key", "k", "", "Comma-separated list of JSON fields to identify a uid")

	// TLS configuration
	x.RegisterTLSFlags(flag)
	flag.String("tls_server_name", "", "Used to verify the server hostname.")
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
	x.Check(err)
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
	if strings.HasPrefix(val, "0x") {
		if _, err := strconv.ParseUint(val[2:], 16, 64); err == nil {
			return val
		}
	}

	uid, _ := l.alloc.AssignUid(val)
	return fmt.Sprintf("%#x", uint64(uid))
}

// forward file to the RDF or JSON processor as appropriate
func (l *loader) processFile(ctx context.Context, file string) error {
	fmt.Printf("Processing data file %q\n", file)

	rd, cleanup_fn := x.FileReader(file)
	defer cleanup_fn()

	var err error
	var isJson bool
	if strings.HasSuffix(file, ".rdf") || strings.HasSuffix(file, ".rdf.gz") {
		err = l.processRdfFile(ctx, rd)
	} else if strings.HasSuffix(file, ".json") || strings.HasSuffix(file, ".json.gz") {
		err = l.processJsonFile(ctx, rd)
	} else {
		isJson, err = x.IsJSONData(rd)
		if isJson {
			err = l.processJsonFile(ctx, rd)
		} else if err == nil {
			err = fmt.Errorf("Unable to determine file content format: %s", file)
		}
	}

	return err
}

func parseJson(chunkBuf *bytes.Buffer) ([]*api.NQuad, error) {
	if chunkBuf.Len() == 0 {
		return nil, io.EOF
	}

	nqs, err := edgraph.JsonToNquads(chunkBuf.Bytes(), &keyFields)
	if err != nil && err != io.EOF {
		x.Check(err)
	}
	chunkBuf.Reset()

	return nqs, err
}

func (l *loader) nextNquads(mu *api.Mutation, nqs []*api.NQuad) *api.Mutation {
	for _, nq := range nqs {
		nq.Subject = l.uid(nq.Subject)
		if len(nq.ObjectId) > 0 {
			nq.ObjectId = l.uid(nq.ObjectId)
		}
	}

	mu.Set = append(mu.Set, nqs...)
	if len(mu.Set) >= opt.batchSize {
		l.reqs <- *mu
		mu = &api.Mutation{}
	}

	return mu
}

func (l *loader) finalNquads(mu *api.Mutation) {
	if len(mu.Set) > 0 {
		l.reqs <- *mu
	}
}

func (l *loader) processJsonFile(ctx context.Context, rd *bufio.Reader) error {
	chunker := x.NewChunker(x.JsonInput)
	x.CheckfNoTrace(chunker.Begin(rd))

	mu := &api.Mutation{}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var nqs []*api.NQuad
		chunkBuf, err := chunker.Chunk(rd)
		if chunkBuf != nil && chunkBuf.Len() > 0 {
			nqs, err = parseJson(chunkBuf)
			x.CheckfNoTrace(err)
			mu = l.nextNquads(mu, nqs)
		}
		if err == io.EOF {
			l.finalNquads(mu)
			break
		} else {
			x.Check(err)
		}
	}
	x.CheckfNoTrace(chunker.End(rd))

	return nil
}

func (l *loader) processRdfFile(ctx context.Context, bufReader *bufio.Reader) error {
	var buf bytes.Buffer
	var line uint64

	mu := &api.Mutation{}
	nqs := make([]*api.NQuad, 1)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		buf.Reset()
		err := readLine(bufReader, &buf)
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		line++

		nq, err := rdf.Parse(buf.String())
		if err == rdf.ErrEmpty { // special case: comment/empty line
			buf.Reset()
			continue
		} else if err != nil {
			return fmt.Errorf("Error while parsing RDF: %v, on line:%v %v", err, line, buf.String())
		}
		nqs[0] = &nq
		mu = l.nextNquads(mu, nqs)
	}
	l.finalNquads(mu)

	return nil
}

func fileList(files string) []string {
	return x.FindDataFiles(files, []string{".rdf", ".rdf.gz", ".json", ".json.gz"})
}

func setup(opts batchMutationOptions, dc *dgo.Dgraph) *loader {
	x.Check(os.MkdirAll(opt.clientDir, 0700))
	o := badger.DefaultOptions
	o.SyncWrites = true // So that checkpoints are persisted immediately.
	o.TableLoadingMode = bopt.MemoryMap
	o.Dir = opt.clientDir
	o.ValueDir = opt.clientDir

	kv, err := badger.Open(o)
	x.Checkf(err, "Error while creating badger KV posting store")

	// compression with zero server actually makes things worse
	connzero, err := x.SetupConnection(opt.zero, &tlsConf, false)
	x.Checkf(err, "Unable to connect to zero, Is it running at %s?", opt.zero)

	alloc := xidmap.New(
		kv,
		connzero,
		xidmap.Options{
			NumShards: 100,
			LRUSize:   1e5,
		},
	)

	l := &loader{
		opts:     opts,
		dc:       dc,
		start:    time.Now(),
		reqs:     make(chan api.Mutation, opts.Pending*2),
		alloc:    alloc,
		kv:       kv,
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
		dataFiles:           Live.Conf.GetString("rdfs"),
		schemaFile:          Live.Conf.GetString("schema"),
		dgraph:              Live.Conf.GetString("dgraph"),
		zero:                Live.Conf.GetString("zero"),
		concurrent:          Live.Conf.GetInt("conc"),
		batchSize:           Live.Conf.GetInt("batch"),
		clientDir:           Live.Conf.GetString("xidmap"),
		ignoreIndexConflict: Live.Conf.GetBool("ignore_index_conflict"),
		authToken:           Live.Conf.GetString("auth_token"),
		useCompression:      Live.Conf.GetBool("use_compression"),
		keyFields:           Live.Conf.GetString("key"),
	}
	x.LoadTLSConfig(&tlsConf, Live.Conf, x.TlsClientCert, x.TlsClientKey)
	tlsConf.ServerName = Live.Conf.GetString("tls_server_name")

	for _, f := range strings.Split(opt.keyFields, ",") {
		keyFields = append(keyFields, strings.TrimSpace(f))
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

	ds := strings.Split(opt.dgraph, ",")
	var clients []api.DgraphClient
	for _, d := range ds {
		conn, err := x.SetupConnection(d, &tlsConf, opt.useCompression)
		x.Checkf(err, "While trying to setup connection to Dgraph alpha.")
		defer conn.Close()

		dc := api.NewDgraphClient(conn)
		clients = append(clients, dc)
	}
	dgraphClient := dgo.NewDgraphClient(clients...)

	if len(opt.clientDir) == 0 {
		var err error
		opt.clientDir, err = ioutil.TempDir("", "x")
		x.Checkf(err, "Error while trying to create temporary client directory.")
		fmt.Printf("Creating temp client directory at %s\n", opt.clientDir)
		defer os.RemoveAll(opt.clientDir)
	}
	l := setup(bmOpts, dgraphClient)
	defer l.zeroconn.Close()
	defer l.kv.Close()
	defer l.alloc.EvictAll()

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

	filesList := fileList(opt.dataFiles)
	totalFiles := len(filesList)
	if totalFiles == 0 {
		fmt.Printf("No data files to process\n")
		return nil
	} else {
		fmt.Printf("Found %d data file(s) to process\n", totalFiles)
	}

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

	return nil
}
