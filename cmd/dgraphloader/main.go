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
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
)

var (
	files      = flag.String("r", "", "Location of rdf files to load")
	schemaFile = flag.String("s", "", "Location of schema file")
	dgraph     = flag.String("d", "127.0.0.1:8080", "Dgraph server address")
	concurrent = flag.Int("c", 100, "Number of concurrent requests to make to Dgraph")
	numRdf     = flag.Int("m", 1000, "Number of RDF N-Quads to send as part of a mutation.")
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
func processSchemaFile(file string, batch *client.BatchMutation) {
	fmt.Printf("\nProcessing %s\n", file)
	f, err := os.Open(file)
	x.Check(err)
	defer f.Close()

	var reader io.Reader
	reader, err = gzip.NewReader(f)
	if err != nil {
		if err == gzip.ErrHeader {
			log.Println("Schema file is not a valid gzip file, reading as plain text instead.")
			if _, err = f.Seek(0, 0); err != nil {
				log.Fatal(err)
			}
			reader = f
		} else {
			log.Fatal(err)
		}
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
		if err = batch.AddSchema(*schemaUpdate[0]); err != nil {
			log.Fatal("While adding schema to batch ", err)
		}

	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
}

// processFile sends mutations for a given gz file.
func processFile(file string, batch *client.BatchMutation) {
	fmt.Printf("\nProcessing %s\n", file)
	f, err := os.Open(file)
	x.Check(err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	x.Check(err)

	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	var line int
	for {
		err = readLine(bufReader, &buf)
		if err != nil {
			break
		}
		line++
		nq, err := rdf.Parse(buf.String())
		if err == rdf.ErrEmpty { // special case: comment/empty line
			buf.Reset()
			continue
		} else if err != nil {
			log.Fatalf("Error while parsing RDF: %v, on line:%v %v", err, line, buf.String())
		}
		buf.Reset()
		if err = batch.AddMutation(nq, client.SET); err != nil {
			log.Fatal("While adding mutation to batch: ", err)
		}
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
}

func printCounters(batch *client.BatchMutation, ticker *time.Ticker) {
	start := time.Now()

	for range ticker.C {
		c := batch.Counter()
		rate := float64(c.Rdfs) / c.Elapsed.Seconds()
		elapsed := ((time.Since(start) / time.Second) * time.Second).String()
		fmt.Printf("[Request: %6d] Total RDFs done: %8d RDFs per second: %7.0f Time Elapsed: %v \r",
			c.Mutations, c.Rdfs, rate, elapsed)

	}
}

func setupConnection() (*grpc.ClientConn, error) {
	if !*tlsEnabled {
		return grpc.Dial(*dgraph, grpc.WithInsecure())
	}

	tlsCfg, err := x.GenerateTLSConfig(x.TLSHelperConfig{
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

	return grpc.Dial(*dgraph, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}

func main() {
	x.Init()

	conn, err := setupConnection()
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	dgraphClient := graphp.NewDgraphClient(conn)
	v, err := dgraphClient.CheckVersion(ctx, &graphp.Check{})
	if err != nil {
		// Error could be unknown service graphp.Dgraph (because we updated proto
		// package) if Dgraph version is < 0.7.4 and loader version is >= 0.7.4.
		// It could be Unknown method CheckVersion if Dgraph version < 0.7.5 and
		// Dgraphloader version is >= 0.7.5 because this method was added in v0.7.5.
		fmt.Printf(`
Could not fetch version information from Dgraph. Got err: %v.
Looks like you are using an older version of Dgraph. Get the latest version from https://docs.dgraph.io
`, err)
	} else {
		version := x.Version()
		if version != "" && v.Tag != "" && version != v.Tag {
			fmt.Printf(`
Dgraph server: %v, loader: %v dont match.
You can get the latest version from https://docs.dgraph.io
`, v.Tag, version)
		}
	}

	batch := client.NewBatchMutation(context.Background(), dgraphClient, *numRdf, *concurrent)

	ticker := time.NewTicker(2 * time.Second)
	go printCounters(batch, ticker)
	filesList := strings.Split(*files, ",")
	x.AssertTrue(len(filesList) > 0)
	if len(*schemaFile) > 0 {
		processSchemaFile(*schemaFile, batch)
	}
	// wait for schema changes to be done before starting mutations
	time.Sleep(1 * time.Second)
	for _, file := range filesList {
		processFile(file, batch)
	}
	batch.Flush()
	ticker.Stop()

	c := batch.Counter()
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
}
