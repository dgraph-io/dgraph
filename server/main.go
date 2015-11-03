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

	"github.com/Sirupsen/logrus"
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

func rdfHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		x.SetStatus(w, x.E_INVALID_METHOD, "Invalid method")
		return
	}

	defer r.Body.Close()
	cnq := make(chan rdf.NQuad, 1000)
	done := make(chan error)
	go rdf.ParseStream(r.Body, cnq, done)

	count := 0
Loop:
	for {
		select {
		case nq := <-cnq:
			edge, err := nq.ToEdge()
			if err != nil {
				x.Err(glog, err).WithField("nq", nq).Error("While converting to edge")
				x.SetStatus(w, x.E_ERROR, err.Error())
				return
			}
			key := posting.Key(edge.Entity, edge.Attribute)
			plist := posting.Get(key)
			plist.AddMutation(edge, posting.Set)
			count += 1
		case err := <-done:
			if err != nil {
				x.Err(glog, err).Error("While reading request")
				x.SetStatus(w, x.E_ERROR, err.Error())
				return
			}
			break Loop
		}
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
	if err != nil {
		x.Err(glog, err).Error("While reading query")
		x.SetStatus(w, x.E_INVALID_REQUEST, err.Error())
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
	x.SetStatus(w, x.E_OK, "Successfully ran the query")
}

func main() {
	flag.Parse()
	if !flag.Parsed() {
		glog.Fatal("Unable to parse flags")
	}
	logrus.SetLevel(logrus.DebugLevel)

	ps := new(store.Store)
	ps.Init(*postingDir)
	ms := new(store.Store)
	ms.Init(*mutationDir)
	posting.Init(ps, ms)

	http.HandleFunc("/rdf", rdfHandler)
	http.HandleFunc("/query", queryHandler)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		x.Err(glog, err).Fatal("ListenAndServe")
	}
}
