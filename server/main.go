/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bufio"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

var glog = x.Log("rdf")

var postingDir = flag.String("postings", "", "Directory to store posting lists")
var mutationDir = flag.String("mutations", "", "Directory to store mutations")
var rdfGzips = flag.String("rdfgzips", "",
	"Comma separated gzip files containing RDF data")
var mod = flag.Uint64("mod", 1, "Only pick entities, where uid % mod == 0.")
var port = flag.String("port", "8080", "Port to run server on.")
var numgo = flag.Int("numgo", 4,
	"Number of goroutines to use for reading file.")

type counters struct {
	read      uint64
	parsed    uint64
	processed uint64
	ignored   uint64
}

func printCounters(ticker *time.Ticker, c *counters) {
	for _ = range ticker.C {
		glog.WithFields(logrus.Fields{
			"read":      atomic.LoadUint64(&c.read),
			"parsed":    atomic.LoadUint64(&c.parsed),
			"processed": atomic.LoadUint64(&c.processed),
			"ignored":   atomic.LoadUint64(&c.ignored),
		}).Info("Counters")
	}
}

func readLines(r io.Reader, input chan string, c *counters) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		input <- scanner.Text()
		atomic.AddUint64(&c.read, 1)
	}
	if err := scanner.Err(); err != nil {
		glog.WithError(err).Fatal("While reading file.")
	}
	close(input)
}

func parseStream(input chan string, cnq chan rdf.NQuad,
	done chan error, c *counters) {

	for line := range input {
		line = strings.Trim(line, " \t")
		if len(line) == 0 {
			glog.Info("Empty line.")
			continue
		}

		glog.Debugf("Got line: %q", line)
		nq, err := rdf.Parse(line)
		if err != nil {
			x.Err(glog, err).Errorf("While parsing: %q", line)
			done <- err
			return
		}
		cnq <- nq
		atomic.AddUint64(&c.parsed, 1)
	}
	done <- nil
}

func handleNQuads(cnq chan rdf.NQuad, ctr *counters, wg *sync.WaitGroup) {
	for nq := range cnq {
		if farm.Fingerprint64([]byte(nq.Subject))%*mod != 0 {
			// Ignore due to mod sampling.
			atomic.AddUint64(&ctr.ignored, 1)
			continue
		}

		edge, err := nq.ToEdge()
		if err != nil {
			x.Err(glog, err).WithField("nq", nq).Error("While converting to edge")
			return
		}

		key := posting.Key(edge.Entity, edge.Attribute)
		plist := posting.Get(key)
		plist.AddMutation(edge, posting.Set)
		atomic.AddUint64(&ctr.processed, 1)
	}
	wg.Done()
}

// Blocking function.
func handleRdfReader(reader io.Reader) (uint64, error) {
	ctr := new(counters)
	ticker := time.NewTicker(time.Second)
	go printCounters(ticker, ctr)

	// Producer: Start buffering input to channel.
	input := make(chan string, 10000)
	go readLines(reader, input, ctr)

	cnq := make(chan rdf.NQuad, 10000)
	done := make(chan error, *numgo)
	wg := new(sync.WaitGroup)
	for i := 0; i < *numgo; i++ {
		wg.Add(1)
		go parseStream(input, cnq, done, ctr) // Input --> NQuads
		go handleNQuads(cnq, ctr, wg)         // NQuads --> Posting list
	}

	// The following will block until all ParseStream goroutines finish.
	for i := 0; i < *numgo; i++ {
		if err := <-done; err != nil {
			glog.WithError(err).Fatal("While reading input.")
		}
	}
	close(cnq)
	// Okay, we've stopped input to cnq, and closed it.
	// Now wait for handleNQuads to finish.
	wg.Wait()

	// We're doing things in this complicated way, because generating
	// string -> NQuad is the slower task here.
	ticker.Stop()
	return atomic.LoadUint64(&ctr.processed), nil
}

func rdfHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		x.SetStatus(w, x.E_INVALID_METHOD, "Invalid method")
		return
	}

	defer r.Body.Close()
	count, err := handleRdfReader(r.Body)
	if err != nil {
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
	glog.WithField("count", count).Debug("RDFs parsed")
	x.SetStatus(w, x.E_OK, fmt.Sprintf("%d RDFs parsed", count))
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		x.SetStatus(w, x.E_INVALID_METHOD, "Invalid method")
		return
	}

	defer r.Body.Close()
	q, err := ioutil.ReadAll(r.Body)
	if err != nil || len(q) == 0 {
		x.Err(glog, err).Error("While reading query")
		x.SetStatus(w, x.E_INVALID_REQUEST, "Invalid request encountered.")
		return
	}
	glog.WithField("q", string(q)).Info("Query received.")
	sg, err := gql.Parse(string(q))
	if err != nil {
		x.Err(glog, err).Error("While parsing query")
		x.SetStatus(w, x.E_INVALID_REQUEST, err.Error())
		return
	}
	glog.WithField("q", string(q)).Info("Query parsed.")
	rch := make(chan error)
	go query.ProcessGraph(sg, rch)
	err = <-rch
	if err != nil {
		x.Err(glog, err).Error("While executing query")
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
	glog.WithField("q", string(q)).Info("Graph processed.")
	js, err := sg.ToJson()
	if err != nil {
		x.Err(glog, err).Error("While converting to Json.")
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(js))
}

func main() {
	flag.Parse()
	if !flag.Parsed() {
		glog.Fatal("Unable to parse flags")
	}
	logrus.SetLevel(logrus.InfoLevel)
	glog.WithField("gomaxprocs", runtime.GOMAXPROCS(-1)).Info("Number of CPUs")

	ps := new(store.Store)
	ps.Init(*postingDir)
	clog := commit.NewLogger(*mutationDir, "dgraph", 50<<20)
	clog.SkipWrite = true // Don't write to commit logs.
	clog.Init()
	defer clog.Close()
	posting.Init(ps, clog)

	if len(*rdfGzips) > 0 {
		files := strings.Split(*rdfGzips, ",")
		for _, path := range files {
			if len(path) == 0 {
				continue
			}
			glog.WithField("path", path).Info("Handling...")
			f, err := os.Open(path)
			if err != nil {
				glog.WithError(err).Fatal("Unable to open rdf file.")
			}

			r, err := gzip.NewReader(f)
			if err != nil {
				glog.WithError(err).Fatal("Unable to create gzip reader.")
			}

			count, err := handleRdfReader(r)
			if err != nil {
				glog.Fatal(err)
			}
			glog.WithField("count", count).Info("RDFs parsed")
			r.Close()
			f.Close()
		}
	}
	// Okay, now start accumulating mutations.
	clog.SkipWrite = false
	clog.SyncEvery = 1

	http.HandleFunc("/rdf", rdfHandler)
	http.HandleFunc("/query", queryHandler)
	glog.WithField("port", *port).Info("Listening for requests...")
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		x.Err(glog, err).Fatal("ListenAndServe")
	}
}
