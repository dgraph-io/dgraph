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

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"

	"github.com/pkg/profile"
)

var (
	files      = flag.String("r", "", "Location of rdf files to load")
	schemaFile = flag.String("s", "", "Location of schema file")
	dgraph     = flag.String("d", "127.0.0.1:9080", "Dgraph gRPC server address")
	concurrent = flag.Int("c", 100, "Number of concurrent requests to make to Dgraph")
	numRdf     = flag.Int("m", 1000, "Number of RDF N-Quads to send as part of a mutation.")
	storeXid   = flag.Bool("x", false, "Store xids by adding corresponding xid edges")
	mode       = flag.String("profile.mode", "", "enable profiling mode, one of [cpu, mem, mutex, block]")
	clientDir  = flag.String("cd", "c", "Directory to store xid to uid mapping")
	blockRate  = flag.Int("block", 0, "Block profiling rate")
	// TLS configuration
	tlsEnabled       = flag.Bool("tls.on", false, "Use TLS connections.")
	tlsInsecure      = flag.Bool("tls.insecure", false, "Skip certificate validation (insecure)")
	tlsServerName    = flag.String("tls.server_name", "", "Server name.")
	tlsCert          = flag.String("tls.cert", "", "Certificate file path.")
	tlsKey           = flag.String("tls.cert_key", "", "Certificate key file path.")
	tlsKeyPass       = flag.String("tls.cert_key_passphrase", "", "Certificate key passphrase.")
	tlsRootCACerts   = flag.String("tls.ca_certs", "", "CA Certs file path.")
	tlsSystemCACerts = flag.Bool("tls.use_system_ca", false, "Include System CA into CA Certs.")
	tlsMinVersion    = flag.String("tls.min_version", "TLS11", "TLS min version.")
	tlsMaxVersion    = flag.String("tls.max_version", "TLS12", "TLS max version.")
	version          = flag.Bool("version", false, "Prints the version of Dgraphloader")
)

// Reads a single line from a buffered reader. The line is read into the
// passed in buffer to minimize allocations. This is the preferred
// method for loading long lines which could be longer than the buffer
// size of bufio.Scanner.
func readLine(r *bufio.Reader, buf *bytes.Buffer) error {
	isPrefix := true
	var err error
	for isPrefix && err == nil {
		var line []byte
		// The returned line is an internal buffer in bufio and is only
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
	return dgraphClient.SetSchemaBlocking(ctx, string(b))
}

func Node(val string, c *client.Dgraph) (string, error) {
	if uid, err := strconv.ParseUint(val, 0, 64); err == nil {
		return c.NodeUid(uid).String(), nil
	}
	if strings.HasPrefix(val, "_:") {
		n, err := c.NodeBlank(val[2:])
		if err != nil {
			return "", err
		}
		return n.String(), nil
	}
	n, err := c.NodeXid(val, *storeXid)
	if err != nil {
		return "", err
	}
	return n.String(), nil
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
func processFile(ctx context.Context, file string, dgraphClient *client.Dgraph) error {
	fmt.Printf("\nProcessing %s\n", file)
	gr, f := fileReader(file)
	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	defer f.Close()

	absPath, err := filepath.Abs(file)
	x.Check(err)
	checkpoint, err := dgraphClient.Checkpoint(absPath)
	x.Check(err)
	if checkpoint != 0 {
		fmt.Printf("\nFound checkpoint for: %s. Skipping: %v lines.\n", file, checkpoint)
	}

	var line uint64
	r := new(client.Req)
	var batchSize int
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		err = readLine(bufReader, &buf)
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		line++
		if line <= checkpoint {
			buf.Reset()
			// No need to parse. We have already sent it to server.
			continue
		}
		nq, err := rdf.Parse(buf.String())
		if err == rdf.ErrEmpty { // special case: comment/empty line
			buf.Reset()
			continue
		} else if err != nil {
			return fmt.Errorf("Error while parsing RDF: %v, on line:%v %v", err, line, buf.String())
		}
		batchSize++
		buf.Reset()

		if nq.Subject, err = Node(nq.Subject, dgraphClient); err != nil {
			return err
		}
		if len(nq.ObjectId) > 0 {
			if nq.ObjectId, err = Node(nq.ObjectId, dgraphClient); err != nil {
				return err
			}
		}
		r.Set(client.NewEdge(nq))
		if batchSize >= *numRdf {
			if err = dgraphClient.BatchSetWithMark(r, absPath, line); err != nil {
				return err
			}
			batchSize = 0
			r = new(client.Req)
		}
	}
	if batchSize > 0 {
		if err = dgraphClient.BatchSetWithMark(r, absPath, line); err != nil {
			return err
		}
	}
	return nil
}

func setupConnection(host string) (*grpc.ClientConn, error) {
	if !*tlsEnabled {
		return grpc.Dial(host,
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
				grpc.MaxCallSendMsgSize(x.GrpcMaxSize)),
			grpc.WithInsecure())
	}

	tlsCfg, _, err := x.GenerateTLSConfig(x.TLSHelperConfig{
		ConfigType:           x.TLSClientConfig,
		Insecure:             *tlsInsecure,
		ServerName:           *tlsServerName,
		Cert:                 *tlsCert,
		Key:                  *tlsKey,
		KeyPassphrase:        *tlsKeyPass,
		RootCACerts:          *tlsRootCACerts,
		UseSystemRootCACerts: *tlsSystemCACerts,
		MinVersion:           *tlsMinVersion,
		MaxVersion:           *tlsMaxVersion,
	})
	if err != nil {
		return nil, err
	}

	return grpc.Dial(host,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
			grpc.MaxCallSendMsgSize(x.GrpcMaxSize)),
		grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}

func fileList(files string) []string {
	if len(files) == 0 {
		return []string{}
	}
	return strings.Split(files, ",")
}

func main() {
	flag.Parse()
	if *version {
		x.PrintVersionOnly()
	}
	runtime.SetBlockProfileRate(*blockRate)

	interruptChan := make(chan os.Signal)
	signal.Notify(interruptChan, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-interruptChan
		cancel()
	}()

	go http.ListenAndServe("localhost:6060", nil)
	switch *mode {
	case "cpu":
		defer profile.Start(profile.CPUProfile).Stop()
	case "mem":
		defer profile.Start(profile.MemProfile).Stop()
	case "mutex":
		defer profile.Start(profile.MutexProfile).Stop()
	case "block":
		defer profile.Start(profile.BlockProfile).Stop()
	default:
		// do nothing
	}

	var conns []*grpc.ClientConn
	hostList := strings.Split(*dgraph, ",")
	x.AssertTrue(len(hostList) > 0)
	for _, host := range hostList {
		host = strings.Trim(host, " \t")
		conn, err := setupConnection(host)
		x.Checkf(err, "While trying to dial gRPC")
		conns = append(conns, conn)
		defer conn.Close()
	}

	bmOpts := client.BatchMutationOptions{
		Size:          *numRdf,
		Pending:       *concurrent,
		PrintCounters: true,
		Ctx:           ctx,
	}
	dgraphClient := client.NewDgraphClient(conns, bmOpts, *clientDir)
	defer dgraphClient.Close()

	{
		ctxTimeout, cancelTimeout := context.WithTimeout(ctx, 1*time.Minute)
		dgraphClient.CheckVersion(ctxTimeout)
		cancelTimeout()
	}

	if *storeXid {
		if err := dgraphClient.AddSchema(protos.SchemaUpdate{
			Predicate: "xid",
			ValueType: uint32(types.StringID),
			Tokenizer: []string{"hash"},
			Directive: protos.SchemaUpdate_INDEX,
		}); err != nil {
			log.Fatal("While adding schema to batch ", err)
		}
	}
	if len(*schemaFile) > 0 {
		if err := processSchemaFile(ctx, *schemaFile, dgraphClient); err != nil {
			if err == context.Canceled {
				log.Println("Interrupted while processing schema file")
			} else {
				log.Println(err)
			}
			return
		}
	}

	filesList := fileList(*files)
	totalFiles := len(filesList)
	if totalFiles == 0 {
		os.Exit(0)
	}

	x.Check(dgraphClient.NewSyncMarks(filesList))
	errCh := make(chan error, totalFiles)
	for _, file := range filesList {
		file = strings.Trim(file, " \t")
		go func(file string) {
			errCh <- processFile(ctx, file, dgraphClient)
		}(file)
	}

	interrupted := false
	for i := 0; i < totalFiles; i++ {
		if err := <-errCh; err != nil {
			if err == context.Canceled {
				interrupted = true
			} else {
				log.Fatal("While processing file ", err)
			}
		}
	}

	{
		if err := dgraphClient.BatchFlush(); err != nil {
			if err == context.Canceled {
				interrupted = true
			} else {
				log.Fatalf("While doing BatchFlush: %+v\n", err)
			}
		}
	}

	c := dgraphClient.Counter()
	var rate uint64
	if c.Elapsed.Seconds() < 1 {
		rate = c.Rdfs
	} else {
		rate = c.Rdfs / uint64(c.Elapsed.Seconds())
	}
	// Lets print an empty line, otherwise Interrupted or Number of Mutations overwrites the
	// previous printed line.
	fmt.Printf("%100s\r", "")

	if interrupted {
		fmt.Println("Interrupted.")
	}
	fmt.Printf("Number of mutations run   : %d\n", c.Mutations)
	fmt.Printf("Number of RDFs processed  : %d\n", c.Rdfs)
	fmt.Printf("Time spent                : %v\n", c.Elapsed)

	fmt.Printf("RDFs processed per second : %d\n", rate)
}
