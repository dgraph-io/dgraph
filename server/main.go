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
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("rdf")

var postingDir = flag.String("postings", "", "Directory to store posting lists")
var mutationDir = flag.String("mutations", "", "Directory to store mutations")
var rdfData = flag.String("rdfdata", "", "File containing RDF data")

func handleRdfReader(reader io.Reader) (int, error) {
	cnq := make(chan rdf.NQuad, 1000)
	done := make(chan error)
	go rdf.ParseStream(reader, cnq, done)

	count := 0
Loop:
	for {
		select {
		case nq := <-cnq:
			edge, err := nq.ToEdge()
			if err != nil {
				x.Err(glog, err).WithField("nq", nq).Error("While converting to edge")
				return 0, err
			}
			key := posting.Key(edge.Entity, edge.Attribute)
			plist := posting.Get(key)
			plist.AddMutation(edge, posting.Set)
			count += 1
		case err := <-done:
			if err != nil {
				x.Err(glog, err).Error("While reading request")
				return 0, err
			}
			break Loop
		}
	}
	return count, nil
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
	sg, err := gql.Parse(string(q))
	if err != nil {
		x.Err(glog, err).Error("While parsing query")
		x.SetStatus(w, x.E_INVALID_REQUEST, err.Error())
		return
	}
	rch := make(chan error)
	go query.ProcessGraph(sg, rch)
	err = <-rch
	if err != nil {
		x.Err(glog, err).Error("While executing query")
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
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
	logrus.SetLevel(logrus.DebugLevel)

	ps := new(store.Store)
	ps.Init(*postingDir)
	clog := commit.NewLogger(*mutationDir, "dgraph", 50<<20)
	clog.Init()
	defer clog.Close()
	posting.Init(ps, clog)

	if len(*rdfData) > 0 {
		f, err := os.Open(*rdfData)
		if err != nil {
			glog.Fatal(err)
		}
		defer f.Close()

		count, err := handleRdfReader(f)
		if err != nil {
			glog.Fatal(err)
		}
		glog.WithField("count", count).Debug("RDFs parsed")
	}

	http.HandleFunc("/rdf", rdfHandler)
	http.HandleFunc("/query", queryHandler)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		x.Err(glog, err).Fatal("ListenAndServe")
	}
}
