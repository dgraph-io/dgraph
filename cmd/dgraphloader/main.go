// This script is used to load data into Dgraph from an RDF file by performing
// mutations using the HTTP interface.
//
// You can run the script like
// go build . && ./dgraphloader -r path-to-gzipped-rdf.gz -s path-to-gzipped-schema-rdf.gz
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
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
func processSchemaFile(file string, dgraphClient *client.Dgraph) {
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

	var buf bytes.Buffer
	bufReader := bufio.NewReader(reader)
	var line int
	for {
		err = readLine(bufReader, &buf)
		if err != nil {
			break
		}
		line++
		schemaUpdate, err := schema.Parse(buf.String())
		if err != nil {
			log.Fatalf("Error while parsing schema: %v, on line:%v %v", err, line, buf.String())
		}
		buf.Reset()
		if len(schemaUpdate) == 0 {
			continue
		}
		if err = dgraphClient.AddSchema(*schemaUpdate[0]); err != nil {
			log.Fatal("While adding schema to batch ", err)
		}

	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
}

func Node(val string, c *client.Dgraph) string {
	if uid, err := strconv.ParseUint(val, 0, 64); err == nil {
		return c.NodeUid(uid).String()
	}
	if strings.HasPrefix(val, "_:") {
		n, err := c.NodeBlank(val[2:])
		if err != nil {
			log.Fatal("Error while converting to node: %v", err)
		}
		return n.String()
	}
	n, err := c.NodeXid(val, *storeXid)
	if err != nil {
		log.Fatal("Error while converting to node: %v", err)
	}
	return n.String()
}

// processFile sends mutations for a given gz file.
func processFile(file string, dgraphClient *client.Dgraph) {
	fmt.Printf("\nProcessing %s\n", file)
	f, err := os.Open(file)
	x.Check(err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	x.Check(err)

	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)

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
		err = readLine(bufReader, &buf)
		if err != nil {
			break
		}
		line++
		if line <= checkpoint {
			// No need to parse. We have already sent it to server.
			continue
		}
		batchSize++
		nq, err := rdf.Parse(buf.String())
		if err == rdf.ErrEmpty { // special case: comment/empty line
			batchSize--
			buf.Reset()
			continue
		} else if err != nil {
			log.Fatalf("Error while parsing RDF: %v, on line:%v %v", err, line, buf.String())
		}
		buf.Reset()

		nq.Subject = Node(nq.Subject, dgraphClient)
		if len(nq.ObjectId) > 0 {
			nq.ObjectId = Node(nq.ObjectId, dgraphClient)
		}
		r.Set(client.NewEdge(nq))
		if batchSize == *numRdf {
			if err = dgraphClient.BatchSetWithMark(r, absPath, line); err != nil {
				log.Fatal("While adding mutation to batch: ", err)
			}
			batchSize = 0
			r = new(client.Req)
		}
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
	if batchSize > 0 {
		if err = dgraphClient.BatchSetWithMark(r, absPath, line); err != nil {
			log.Fatal("While adding mutation to batch: ", err)
		}
	}
}

func setupConnection(host string) (*grpc.ClientConn, error) {
	if !*tlsEnabled {
		return grpc.Dial(host, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
			grpc.MaxCallSendMsgSize(x.GrpcMaxSize)), grpc.WithInsecure())
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

	return grpc.Dial(host, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxCallSendMsgSize(x.GrpcMaxSize)), grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}

func main() {
	flag.Parse()
	x.Init()
	runtime.SetBlockProfileRate(*blockRate)
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
	}
	dgraphClient := client.NewDgraphClient(conns, bmOpts, *clientDir)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	dgraphClient.CheckVersion(ctx)

	filesList := strings.Split(*files, ",")
	x.AssertTrue(len(filesList) > 0)
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
		processSchemaFile(*schemaFile, dgraphClient)
	}

	// wait for schema changes to be done before starting mutations
	time.Sleep(1 * time.Second)
	var wg sync.WaitGroup
	x.Check(dgraphClient.NewSyncMarks(filesList))
	for _, file := range filesList {
		file = strings.Trim(file, " \t")
		wg.Add(1)
		go func(file string) {
			defer wg.Done()
			processFile(file, dgraphClient)
		}(file)
	}
	wg.Wait()
	dgraphClient.BatchFlush()

	c := dgraphClient.Counter()
	var rate uint64
	if c.Elapsed.Seconds() < 1 {
		rate = c.Rdfs
	} else {
		rate = c.Rdfs / uint64(c.Elapsed.Seconds())
	}
	// Lets print an empty line, otherwise Number of Mutations overwrites the previous
	// printed line.
	fmt.Printf("%100s\r", "")
	fmt.Printf("Number of mutations run   : %d\n", c.Mutations)
	fmt.Printf("Number of RDFs processed  : %d\n", c.Rdfs)
	fmt.Printf("Time spent                : %v\n", c.Elapsed)

	fmt.Printf("RDFs processed per second : %d\n", rate)

	// Lets call this so that badger is closed properly.
	dgraphClient.Close()
}
