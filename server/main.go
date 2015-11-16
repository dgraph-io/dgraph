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
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
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

type counters struct {
	read      uint64
	processed uint64
	ignored   uint64
}

func printCounters(ticker *time.Ticker, c *counters) {
	for _ = range ticker.C {
		glog.WithFields(logrus.Fields{
			"read":      atomic.LoadUint64(&c.read),
			"processed": atomic.LoadUint64(&c.processed),
			"ignored":   atomic.LoadUint64(&c.ignored),
		}).Info("Counters")
	}
}

// Blocking function.
func handleRdfReader(reader io.Reader) (uint64, error) {
	cnq := make(chan rdf.NQuad, 1000)
	done := make(chan error)
	ctr := new(counters)
	ticker := time.NewTicker(time.Second)
	go rdf.ParseStream(reader, cnq, done)
	go printCounters(ticker, ctr)
Loop:
	for {
		select {
		case nq := <-cnq:
			atomic.AddUint64(&ctr.read, 1)

			if farm.Fingerprint64([]byte(nq.Subject))%*mod != 0 {
				// Ignore due to mod sampling.
				atomic.AddUint64(&ctr.ignored, 1)
				break
			}

			edge, err := nq.ToEdge()
			if err != nil {
				x.Err(glog, err).WithField("nq", nq).Error("While converting to edge")
				return 0, err
			}
			key := posting.Key(edge.Entity, edge.Attribute)
			plist := posting.Get(key)
			plist.AddMutation(edge, posting.Set)
			atomic.AddUint64(&ctr.processed, 1)

		case err := <-done:
			if err != nil {
				x.Err(glog, err).Error("While reading request")
				return 0, err
			}
			break Loop
		}
	}
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

	ps := new(store.Store)
	ps.Init(*postingDir)
	clog := commit.NewLogger(*mutationDir, "dgraph", 50<<20)
	clog.SyncEvery = 1000
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

	http.HandleFunc("/rdf", rdfHandler)
	http.HandleFunc("/query", queryHandler)
	glog.WithField("port", *port).Info("Listening for requests...")
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		x.Err(glog, err).Fatal("ListenAndServe")
	}
}
