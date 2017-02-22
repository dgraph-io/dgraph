// This script is used to load data into Dgraph from an RDF file by performing
// mutations using the HTTP interface.
//
// You can run the script like
// go build . && ./dgraphloader -r path-to-gzipped-rdf.gz
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
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
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
)

var (
	files      = flag.String("r", "", "Location of rdf files to load")
	dgraph     = flag.String("d", "127.0.0.1:8080", "Dgraph server address")
	tlsEnabled = flag.Bool("tls", true, "Enable TLS communication")
	insecure   = flag.Bool("insecure", false, "Skip insecure cert validation")
	concurrent = flag.Int("c", 100, "Number of concurrent requests to make to Dgraph")
	numRdf     = flag.Int("m", 1000, "Number of RDF N-Quads to send as part of a mutation.")
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
	for {
		err = readLine(bufReader, &buf)
		if err != nil {
			break
		}
		nq, err := rdf.Parse(buf.String())
		if err == rdf.ErrEmpty { // special case: comment/empty line
			buf.Reset()
			continue
		} else if err != nil {
			log.Fatal("While parsing RDF: ", err)
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
	for range ticker.C {
		c := batch.Counter()
		rate := float64(c.Rdfs) / c.Elapsed.Seconds()
		fmt.Printf("[Request: %6d] Total RDFs done: %8d RDFs per second: %7.0f \r", c.Mutations, c.Rdfs, rate)
	}
}

func setupConnection() (*grpc.ClientConn, error) {
	if !*tlsEnabled {
		return grpc.Dial(*dgraph, grpc.WithInsecure())
	}

	tlsCfg := &tls.Config{
		InsecureSkipVerify: *insecure,
		MinVersion:         tls.VersionTLS11, //min secure version
	}

	return grpc.Dial(*dgraph, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}

func main() {
	x.Init()

	conn, err := setupConnection()
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	batch := client.NewBatchMutation(context.Background(), conn, *numRdf, *concurrent)

	ticker := time.NewTicker(2 * time.Second)
	go printCounters(batch, ticker)
	filesList := strings.Split(*files, ",")
	x.AssertTrue(len(filesList) > 0)
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
