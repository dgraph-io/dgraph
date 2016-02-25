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
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"runtime"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("server")

var postingDir = flag.String("postings", "", "Directory to store posting lists")
var xiduidDir = flag.String("xiduid", "", "XID UID posting lists directory")
var mutationDir = flag.String("mutations", "", "Directory to store mutations")
var port = flag.String("port", "8080", "Port to run server on.")
var numcpu = flag.Int("numCpu", runtime.NumCPU(),
	"Number of cores to be used by the process")
var instanceIdx = flag.Uint64("instanceIdx", 0,
	"serves only entities whose Fingerprint % numInstance == instanceIdx.")
var numInstances = flag.Uint64("numInstances", 1,
	"Total number of server instances")

func addCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers",
		"Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token,"+
			"X-Auth-Token, Cache-Control, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Connection", "close")
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		x.SetStatus(w, x.E_INVALID_METHOD, "Invalid method")
		return
	}

	var l query.Latency
	l.Start = time.Now()
	defer r.Body.Close()
	q, err := ioutil.ReadAll(r.Body)
	if err != nil || len(q) == 0 {
		x.Err(glog, err).Error("While reading query")
		x.SetStatus(w, x.E_INVALID_REQUEST, "Invalid request encountered.")
		return
	}

	glog.WithField("q", string(q)).Debug("Query received.")
	gq, _, err := gql.Parse(string(q))
	if err != nil {
		x.Err(glog, err).Error("While parsing query")
		x.SetStatus(w, x.E_INVALID_REQUEST, err.Error())
		return
	}
	sg, err := query.ToSubGraph(gq)
	if err != nil {
		x.Err(glog, err).Error("While conversion to internal format")
		x.SetStatus(w, x.E_INVALID_REQUEST, err.Error())
		return
	}
	l.Parsing = time.Since(l.Start)
	glog.WithField("q", string(q)).Debug("Query parsed.")

	rch := make(chan error)
	go query.ProcessGraph(sg, rch)
	err = <-rch
	if err != nil {
		x.Err(glog, err).Error("While executing query")
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
	l.Processing = time.Since(l.Start) - l.Parsing
	glog.WithField("q", string(q)).Debug("Graph processed.")
	js, err := sg.ToJson(&l)
	if err != nil {
		x.Err(glog, err).Error("While converting to Json.")
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
	glog.WithFields(logrus.Fields{
		"total":   time.Since(l.Start),
		"parsing": l.Parsing,
		"process": l.Processing,
		"json":    l.Json,
	}).Info("Query Latencies")

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, string(js))
}

func main() {
	flag.Parse()
	if !flag.Parsed() {
		glog.Fatal("Unable to parse flags")
	}
	logrus.SetLevel(logrus.InfoLevel)
	numCpus := *numcpu
	prev := runtime.GOMAXPROCS(numCpus)
	glog.WithField("num_cpu", numCpus).
		WithField("prev_maxprocs", prev).
		Info("Set max procs to num cpus")

	ps := new(store.Store)
	ps.Init(*postingDir)
	defer ps.Close()

	clog := commit.NewLogger(*mutationDir, "dgraph", 50<<20)
	clog.SyncEvery = 1
	clog.Init()
	defer clog.Close()

	posting.Init(clog)
	if *instanceIdx == 0 {
		xiduidStore := new(store.Store)
		xiduidStore.Init(*xiduidDir)
		defer xiduidStore.Close()
		worker.Init(ps, xiduidStore) //Only server instance 0 will have xiduidStore
		uid.Init(xiduidStore)
	} else {
		worker.Init(ps, nil)
	}
	worker.Connect()

	http.HandleFunc("/query", queryHandler)
	glog.WithField("port", *port).Info("Listening for requests...")
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		x.Err(glog, err).Fatal("ListenAndServe")
	}
}
