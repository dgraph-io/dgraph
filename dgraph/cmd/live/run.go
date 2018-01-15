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

package live

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/badger"
	bopt "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/dgraph/xidmap"
	"github.com/spf13/cobra"
)

type options struct {
	files               string
	schemaFile          string
	dgraph              string
	zero                string
	concurrent          int
	numRdf              int
	clientDir           string
	ignoreIndexConflict bool
}

var opt options
var tlsConf x.TLSHelperConfig

var Live x.SubCommand

func init() {
	Live.Cmd = &cobra.Command{
		Use:   "live",
		Short: "Run Dgraph live loader",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Live.Conf).Stop()
			run()
		},
	}
	Live.EnvPrefix = "DGRAPH_LIVE"

	flag := Live.Cmd.Flags()
	flag.StringP("rdfs", "r", "", "Location of rdf files to load")
	flag.StringP("schema", "s", "", "Location of schema file")
	flag.StringP("dgraph", "d", "127.0.0.1:9080", "Dgraph gRPC server address")
	flag.StringP("zero", "z", "127.0.0.1:7080", "Dgraphzero gRPC server address")
	flag.IntP("conc", "c", 1,
		"Number of concurrent requests to make to Dgraph")
	flag.IntP("batch", "b", 10000,
		"Number of RDF N-Quads to send as part of a mutation.")
	flag.StringP("xidmap", "x", "x", "Directory to store xid to uid mapping")
	flag.BoolP("ignore_index_conflict", "i", true,
		"Ignores conflicts on index keys during transaction")

	// TLS configuration
	x.RegisterTLSFlags(flag)
	flag.Bool("tls_insecure", false, "Skip certificate validation (insecure)")
	flag.String("tls_ca_certs", "", "CA Certs file path.")
	flag.String("tls_server_name", "", "Server name.")
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
		// The returned line is an intern.buffer in bufio and is only
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
func processSchemaFile(ctx context.Context, file string, dgraphClient *client.Dgraph) error {
	fmt.Printf("\nProcessing %s\n", file)
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

func fileReader(file string) (io.Reader, *os.File) {
	f, err := os.Open(file)
	x.Check(err)

	var r io.Reader
	if filepath.Ext(file) == ".gz" {
		r, err = gzip.NewReader(f)
		x.Check(err)
	} else {
		r = bufio.NewReader(f)
	}
	return r, f
}

// processFile sends mutations for a given gz file.
func (l *loader) processFile(ctx context.Context, file string) error {
	fmt.Printf("\nProcessing %s\n", file)
	gr, f := fileReader(file)
	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	defer f.Close()

	var line uint64
	mu := api.Mutation{}
	var batchSize int
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
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
		batchSize++
		buf.Reset()

		nq.Subject = l.uid(nq.Subject)
		if len(nq.ObjectId) > 0 {
			nq.ObjectId = l.uid(nq.ObjectId)
		}
		mu.Set = append(mu.Set, &nq)

		if batchSize >= opt.numRdf {
			l.reqs <- mu
			atomic.AddUint64(&l.rdfs, uint64(batchSize))
			batchSize = 0
			mu = api.Mutation{}
		}
	}
	if batchSize > 0 {
		l.reqs <- mu
		atomic.AddUint64(&l.rdfs, uint64(batchSize))
		mu = api.Mutation{}
	}
	return nil
}

func setupConnection(host string, insecure bool) (*grpc.ClientConn, error) {
	if insecure {
		return grpc.Dial(host,
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
				grpc.MaxCallSendMsgSize(x.GrpcMaxSize)),
			grpc.WithInsecure(),
			grpc.WithBlock(),
			grpc.WithTimeout(10*time.Second))
	}

	tlsConf.ConfigType = x.TLSClientConfig
	tlsConf.CertRequired = false
	tlsCfg, _, err := x.GenerateTLSConfig(tlsConf)
	if err != nil {
		return nil, err
	}

	return grpc.Dial(host,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
			grpc.MaxCallSendMsgSize(x.GrpcMaxSize)),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second))
}

func fileList(files string) []string {
	if len(files) == 0 {
		return []string{}
	}
	return strings.Split(files, ",")
}

func setup(opts batchMutationOptions, dc *client.Dgraph) *loader {
	x.Check(os.MkdirAll(opt.clientDir, 0700))
	o := badger.DefaultOptions
	o.SyncWrites = true // So that checkpoints are persisted immediately.
	o.TableLoadingMode = bopt.MemoryMap
	o.Dir = opt.clientDir
	o.ValueDir = opt.clientDir

	kv, err := badger.Open(o)
	x.Checkf(err, "Error while creating badger KV posting store")

	alloc := xidmap.New(
		kv,
		xidmap.NewZeroPool(
			func(addr string) (*grpc.ClientConn, error) {
				return setupConnection(addr, true)
			},
			opt.zero,
		),
		xidmap.Options{
			NumShards: 100,
			LRUSize:   1e5,
		},
	)

	l := &loader{
		opts:  opts,
		dc:    dc,
		start: time.Now(),
		reqs:  make(chan api.Mutation, opts.Pending*2),
		alloc: alloc,
		kv:    kv,
	}

	l.wg.Add(opts.Pending)
	for i := 0; i < opts.Pending; i++ {
		go l.makeRequests()
	}

	rand.Seed(time.Now().Unix())
	if opts.PrintCounters {
		go l.printCounters()
	}
	return l
}

func run() {
	opt = options{
		files:               Live.Conf.GetString("rdfs"),
		schemaFile:          Live.Conf.GetString("schema"),
		dgraph:              Live.Conf.GetString("dgraph"),
		zero:                Live.Conf.GetString("zero"),
		concurrent:          Live.Conf.GetInt("conc"),
		numRdf:              Live.Conf.GetInt("batch"),
		clientDir:           Live.Conf.GetString("xidmap"),
		ignoreIndexConflict: Live.Conf.GetBool("ignore_index_conflict"),
	}
	x.LoadTLSConfig(&tlsConf, Live.Conf)
	tlsConf.Insecure = Live.Conf.GetBool("tls_insecure")
	tlsConf.RootCACerts = Live.Conf.GetString("tls_ca_certs")
	tlsConf.ServerName = Live.Conf.GetString("tls_server_name")

	go http.ListenAndServe("localhost:6060", nil)
	ctx := context.Background()
	bmOpts := batchMutationOptions{
		Size:          opt.numRdf,
		Pending:       opt.concurrent,
		PrintCounters: true,
		Ctx:           ctx,
		MaxRetries:    math.MaxUint32,
	}

	ds := strings.Split(opt.dgraph, ",")
	var clients []api.DgraphClient
	for _, d := range ds {
		conn, err := setupConnection(d, !tlsConf.CertRequired)
		x.Checkf(err, "While trying to setup connection to Dgraph server.")
		defer conn.Close()

		dc := api.NewDgraphClient(conn)
		clients = append(clients, dc)
	}
	dgraphClient := client.NewDgraphClient(clients...)
	l := setup(bmOpts, dgraphClient)
	defer l.kv.Close()
	defer l.alloc.EvictAll()

	if len(opt.schemaFile) > 0 {
		if err := processSchemaFile(ctx, opt.schemaFile, dgraphClient); err != nil {
			if err == context.Canceled {
				log.Println("Interrupted while processing schema file")
			} else {
				log.Println(err)
			}
			return
		}
		x.Printf("Processed schema file")
	}

	filesList := fileList(opt.files)
	totalFiles := len(filesList)
	if totalFiles == 0 {
		os.Exit(0)
	}

	//	x.Check(dgraphClient.NewSyncMarks(filesList))
	errCh := make(chan error, totalFiles)
	for _, file := range filesList {
		file = strings.Trim(file, " \t")
		go func(file string) {
			errCh <- l.processFile(ctx, file)
		}(file)
	}

	for i := 0; i < totalFiles; i++ {
		if err := <-errCh; err != nil {
			log.Fatal("While processing file ", err)
		}
	}

	close(l.reqs)
	l.wg.Wait()
	c := l.Counter()
	var rate uint64
	if c.Elapsed.Seconds() < 1 {
		rate = c.Rdfs
	} else {
		rate = c.Rdfs / uint64(c.Elapsed.Seconds())
	}
	// Lets print an empty line, otherwise Interrupted or Number of Mutations overwrites the
	// previous printed line.
	fmt.Printf("%100s\r", "")

	fmt.Printf("Number of mutations run   : %d\n", c.TxnsDone)
	fmt.Printf("Number of RDFs processed  : %d\n", c.Rdfs)
	fmt.Printf("Time spent                : %v\n", c.Elapsed)

	fmt.Printf("RDFs processed per second : %d\n", rate)
}
