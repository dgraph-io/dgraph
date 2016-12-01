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
	concurrent = flag.Int("c", 500, "Number of concurrent requests to make to Dgraph")
	numRdf     = flag.Int("m", 100, "Number of RDF N-Quads to send as part of a mutation.")
)

func body(rdf string) string {
	return fmt.Sprintf("mutation { set { %s } }", rdf)
}

type response struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var hc http.Client
var r response

func makeRequest(mutation chan string, c *uint64, wg *sync.WaitGroup) {
	var counter uint64
	for m := range mutation {
		counter = atomic.AddUint64(c, 1)
		if counter%100 == 0 {
			fmt.Printf("Request: %v\n", counter)
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
	fmt.Printf("Processing %s\n", file)
	f, err := os.Open(file)
	x.Check(err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	x.Check(err)

	hc = http.Client{Timeout: time.Minute}
	mutation := make(chan string, 3*(*concurrent))

	var count uint64
	var wg sync.WaitGroup
	for i := 0; i < *concurrent; i++ {
		wg.Add(1)
		go makeRequest(mutation, &count, &wg)
	}

	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	num := 0
	var rdfCount uint64
	for {
		err = readLine(bufReader, &buf)
		if err != nil {
			break
		}
		buf.WriteRune('\n')

		if num >= *numRdf {
			mutation <- buf.String()
			buf.Reset()
			num = 0
		}
		rdfCount++
		num++
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
	if buf.Len() > 0 {
		mutation <- buf.String()
	}
	close(mutation)

	wg.Wait()
	fmt.Println("Number of RDF's parsed: ", rdfCount)
	fmt.Println("Number of mutations run: ", count)
}

func main() {
	x.Init()

	filesList := strings.Split(*files, ",")
	x.AssertTrue(len(filesList) > 0)
	for _, file := range filesList {
		processFile(file)
	}
}
