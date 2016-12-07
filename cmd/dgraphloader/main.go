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
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/dgraph/x"
)

var (
	files      = flag.String("r", "", "Location of rdf files to load")
	dgraph     = flag.String("d", "http://127.0.0.1:8080/query", "Dgraph server address")
	concurrent = flag.Int("c", 100, "Number of concurrent requests to make to Dgraph")
	numRdf     = flag.Int("m", 1000, "Number of RDF N-Quads to send as part of a mutation.")
)

func body(rdf string) string {
	return fmt.Sprintf("mutation { set { %s } }", rdf)
}

type response struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type status struct {
	rdfs      uint64
	mutations uint64
	start     time.Time
}

var hc http.Client
var r response
var s *status

func makeRequests(mutation chan string, wg *sync.WaitGroup) {
	for m := range mutation {
		counter := atomic.AddUint64(&s.mutations, 1)
		if counter%100 == 0 {
			num := atomic.LoadUint64(&s.rdfs)
			dur := time.Since(s.start)
			rate := float64(num) / dur.Seconds()
			fmt.Printf("[Request: %6d] Total RDFs done: %8d RDFs per second: %7.0f\r", counter, num, rate)
		}
	RETRY:
		req, err := http.NewRequest("POST", *dgraph, strings.NewReader(body(m)))
		x.Check(err)
		res, err := hc.Do(req)
		if err != nil {
			fmt.Printf("Retrying req: %d. Error: %v\n", counter, err)
			time.Sleep(5 * time.Millisecond)
			goto RETRY
		}

		body, err := ioutil.ReadAll(res.Body)
		x.Check(err)
		if err = json.Unmarshal(body, &r); err != nil {
			// Not doing x.Checkf(json.Unmarshal..., "Response..", string(body))
			// to ensure that we don't try to convert body from []byte to string
			// when there's no errors.
			x.Checkf(err, "HTTP Status: %s Response body: %s.",
				http.StatusText(res.StatusCode), string(body))
		}
		if r.Code != "ErrorOk" {
			log.Fatalf("Error while performing mutation: %v, err: %v", m, r.Message)
		}
	}
	wg.Done()
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
func processFile(file string) {
	fmt.Printf("Processing %s\n\n", file)
	f, err := os.Open(file)
	x.Check(err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	x.Check(err)

	hc = http.Client{Timeout: time.Minute}
	mutation := make(chan string, 3*(*concurrent))

	var wg sync.WaitGroup
	for i := 0; i < *concurrent; i++ {
		wg.Add(1)
		go makeRequests(mutation, &wg)
	}

	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	num := 0
	for {
		err = readLine(bufReader, &buf)
		if err != nil {
			break
		}
		buf.WriteRune('\n')
		atomic.AddUint64(&s.rdfs, 1)
		num++

		if num >= *numRdf {
			mutation <- buf.String()
			buf.Reset()
			num = 0
		}
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
	if buf.Len() > 0 {
		mutation <- buf.String()
	}
	close(mutation)

	wg.Wait()
}

func main() {
	x.Init()

	s = &status{
		start: time.Now(),
	}
	filesList := strings.Split(*files, ",")
	x.AssertTrue(len(filesList) > 0)
	for _, file := range filesList {
		processFile(file)
	}
	fmt.Printf("Number of mutations run   : %d\n", s.mutations)
	fmt.Printf("Number of RDFs processed  : %d\n", s.rdfs)
	fmt.Printf("Time spent                : %v\n", time.Since(s.start))
	fmt.Printf("RDFs processed per second : %d\n", s.rdfs/uint64(time.Since(s.start).Seconds()))
}
