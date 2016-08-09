/*
 * Copyright 2015 DGraph Labs, Inc.
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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"google.golang.org/grpc"

	"os"

	"github.com/dgraph-io/dgraph/commit"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var postingDir = flag.String("postings", "", "Directory to store posting lists")
var uidDir = flag.String("uids", "", "XID UID posting lists directory")
var mutationDir = flag.String("mutations", "", "Directory to store mutations")
var port = flag.Int("port", 8080, "Port to run server on.")
var numcpu = flag.Int("numCpu", runtime.NumCPU(),
	"Number of cores to be used by the process")
var instanceIdx = flag.Uint64("instanceIdx", 0,
	"serves only entities whose Fingerprint % numInstance == instanceIdx.")
var workers = flag.String("workers", "",
	"Comma separated list of IP addresses of workers")
var nomutations = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
var tracing = flag.Float64("trace", 0.5, "The ratio of queries to trace.")

func addCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers",
		"Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token,"+
			"X-Auth-Token, Cache-Control, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Connection", "close")
}

func convertToEdges(ctx context.Context, mutation string) ([]x.DirectedEdge, error) {
	var edges []x.DirectedEdge

	r := strings.NewReader(mutation)
	scanner := bufio.NewScanner(r)
	var nquads []rdf.NQuad
	for scanner.Scan() {
		ln := strings.Trim(scanner.Text(), " \t")
		if len(ln) == 0 {
			continue
		}
		nq, err := rdf.Parse(ln)
		if err != nil {
			x.Trace(ctx, "Error while parsing RDF: %v", err)
			return edges, err
		}
		nquads = append(nquads, nq)
	}

	xidToUid := make(map[string]uint64)
	for _, nq := range nquads {
		if !strings.HasPrefix(nq.Subject, "_uid_:") {
			xidToUid[nq.Subject] = 0
		}
		if len(nq.ObjectId) > 0 && !strings.HasPrefix(nq.ObjectId, "_uid_:") {
			xidToUid[nq.ObjectId] = 0
		}
	}
	if len(xidToUid) > 0 {
		if err := worker.GetOrAssignUidsOverNetwork(ctx, &xidToUid); err != nil {
			x.Trace(ctx, "Error while GetOrAssignUidsOverNetwork: %v", err)
			return edges, err
		}
	}

	for _, nq := range nquads {
		edge, err := nq.ToEdgeUsing(xidToUid)
		if err != nil {
			x.Trace(ctx, "Error while converting to edge: %v %v", nq, err)
			return edges, err
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

func mutationHandler(ctx context.Context, mu *gql.Mutation) error {
	if *nomutations {
		return fmt.Errorf("Mutations are forbidden on this server.")
	}

	var m worker.Mutations
	var err error
	if m.Set, err = convertToEdges(ctx, mu.Set); err != nil {
		return err
	}
	if m.Del, err = convertToEdges(ctx, mu.Del); err != nil {
		return err
	}

	left, err := worker.MutateOverNetwork(ctx, m)
	if err != nil {
		x.Trace(ctx, "Error while MutateOverNetwork: %v", err)
		return err
	}
	if len(left.Set) > 0 || len(left.Del) > 0 {
		x.Trace(ctx, "%d edges couldn't be applied", len(left.Del)+len(left.Set))
		for _, e := range left.Set {
			x.Trace(ctx, "Unable to apply set mutation for edge: %v", e)
		}
		for _, e := range left.Del {
			x.Trace(ctx, "Unable to apply delete mutation for edge: %v", e)
		}
		return fmt.Errorf("Unapplied mutations")
	}
	return nil
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if rand.Float64() < *tracing {
		tr := trace.New("Dgraph", "Query")
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	var l query.Latency
	l.Start = time.Now()
	defer r.Body.Close()
	q, err := ioutil.ReadAll(r.Body)
	if err != nil || len(q) == 0 {
		x.Trace(ctx, "Error while reading query: %v", err)
		x.SetStatus(w, x.E_INVALID_REQUEST, "Invalid request encountered.")
		return
	}

	x.Trace(ctx, "Query received: %v", string(q))
	gq, mu, err := gql.Parse(string(q))
	if err != nil {
		x.Trace(ctx, "Error while parsing query: %v", err)
		x.SetStatus(w, x.E_INVALID_REQUEST, err.Error())
		return
	}

	// If we have mutations, run them first.
	if mu != nil && (len(mu.Set) > 0 || len(mu.Del) > 0) {
		if err = mutationHandler(ctx, mu); err != nil {
			x.Trace(ctx, "Error while handling mutations: %v", err)
			x.SetStatus(w, x.E_ERROR, err.Error())
			return
		}
	}

	if gq == nil || (gq.UID == 0 && len(gq.XID) == 0) {
		x.SetStatus(w, x.E_OK, "Done")
		return
	}

	sg, err := query.ToSubGraph(ctx, gq)
	if err != nil {
		x.Trace(ctx, "Error while conversion to internal format: %v", err)
		x.SetStatus(w, x.E_INVALID_REQUEST, err.Error())
		return
	}
	l.Parsing = time.Since(l.Start)
	x.Trace(ctx, "Query parsed")

	rch := make(chan error)
	go query.ProcessGraph(ctx, sg, rch)
	err = <-rch
	if err != nil {
		x.Trace(ctx, "Error while executing query: %v", err)
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
	l.Processing = time.Since(l.Start) - l.Parsing
	x.Trace(ctx, "Graph processed")
	js, err := sg.ToJson(&l)
	if err != nil {
		x.Trace(ctx, "Error while converting to Json: %v", err)
		x.SetStatus(w, x.E_ERROR, err.Error())
		return
	}
	x.Trace(ctx, "Latencies: Total: %v Parsing: %v Process: %v Json: %v",
		time.Since(l.Start), l.Parsing, l.Processing, l.Json)

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// server is used to implement graph.DgraphServer
type server struct{}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *server) Query(ctx context.Context,
	req *graph.Request) (*graph.Response, error) {

	if rand.Float64() < *tracing {
		tr := trace.New("Dgraph", "GrpcQuery")
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	resp := new(graph.Response)
	if len(req.Query) == 0 {
		x.Trace(ctx, "Empty query")
		return resp, fmt.Errorf("Empty query")
	}

	var l query.Latency
	l.Start = time.Now()
	// TODO(pawan): Refactor query parsing and graph processing code to a common
	// function used by Query and queryHandler
	x.Trace(ctx, "Query received: %v", req.Query)
	gq, mu, err := gql.Parse(req.Query)
	if err != nil {
		x.Trace(ctx, "Error while parsing query: %v", err)
		return resp, err
	}

	// If we have mutations, run them first.
	if mu != nil && (len(mu.Set) > 0 || len(mu.Del) > 0) {
		if err = mutationHandler(ctx, mu); err != nil {
			x.Trace(ctx, "Error while handling mutations: %v", err)
			return resp, err
		}
	}

	if gq == nil || (gq.UID == 0 && len(gq.XID) == 0) {
		return resp, err
	}

	sg, err := query.ToSubGraph(ctx, gq)
	if err != nil {
		x.Trace(ctx, "Error while conversion to internal format: %v", err)
		return resp, err
	}
	l.Parsing = time.Since(l.Start)
	x.Trace(ctx, "Query parsed")

	rch := make(chan error)
	go query.ProcessGraph(ctx, sg, rch)
	err = <-rch
	if err != nil {
		x.Trace(ctx, "Error while executing query: %v", err)
		return resp, err
	}
	l.Processing = time.Since(l.Start) - l.Parsing
	x.Trace(ctx, "Graph processed")

	node, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		x.Trace(ctx, "Error while converting to ProtocolBuffer: %v", err)
		return resp, err
	}
	resp.N = node

	gl := new(graph.Latency)
	gl.Parsing, gl.Processing, gl.Pb = l.Parsing.String(), l.Processing.String(),
		l.ProtocolBuffer.String()
	resp.L = gl

	return resp, err
}

// This function register a Dgraph grpc server on the address, which is used
// exchanging protocol buffer messages.
func runGrpcServer(address string) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("While running server for client: %v", err)
		return
	}
	log.Printf("Client worker listening: %v", ln.Addr())

	s := grpc.NewServer(grpc.CustomCodec(&query.Codec{}))
	graph.RegisterDgraphServer(s, &server{})
	if err = s.Serve(ln); err != nil {
		log.Fatalf("While serving gRpc requests: %v", err)
	}
	return
}

func main() {
	flag.Parse()
	if !flag.Parsed() {
		log.Fatal("Unable to parse flags")
	}
	numCpus := *numcpu
	prev := runtime.GOMAXPROCS(numCpus)
	log.Printf("num_cpu: %v. prev_maxprocs: %v. Set max procs to num cpus", numCpus, prev)
	if *port%2 != 0 {
		log.Fatalf("Port should be an even number: %v", *port)
	}
	// Create parent directories for postings, uids and mutations
	var err error
	err = os.MkdirAll(*postingDir, 0700)
	if err != nil {
		log.Fatalf("Error while creating the filepath for postings: %v", err)
	}
	err = os.MkdirAll(*mutationDir, 0700)
	if err != nil {
		log.Fatalf("Error while creating the filepath for mutations: %v", err)
	}
	err = os.MkdirAll(*uidDir, 0700)
	if err != nil {
		log.Fatalf("Error while creating the filepath for uids: %v", err)
	}

	ps := new(store.Store)
	ps.Init(*postingDir)
	defer ps.Close()

	clog := commit.NewLogger(*mutationDir, "dgraph", 50<<20)
	clog.SyncEvery = 1
	clog.Init()
	defer clog.Close()

	addrs := strings.Split(*workers, ",")
	lenAddr := uint64(len(addrs))
	if lenAddr == 0 {
		// If no worker is specified, then we're it.
		lenAddr = 1
	}

	posting.Init(clog)
	if *instanceIdx != 0 {
		worker.Init(ps, nil, *instanceIdx, lenAddr)
		uid.Init(nil)
		go posting.CheckMemoryUsage(ps, nil)
	} else {
		uidStore := new(store.Store)
		uidStore.Init(*uidDir)
		defer uidStore.Close()
		// Only server instance 0 will have uidStore
		worker.Init(ps, uidStore, *instanceIdx, lenAddr)
		uid.Init(uidStore)
		go posting.CheckMemoryUsage(ps, uidStore)
	}

	worker.Connect(addrs)
	// Grpc server runs on (port + 1)
	go runGrpcServer(fmt.Sprintf(":%d", *port+1))

	http.HandleFunc("/query", queryHandler)
	log.Printf("Listening for requests at port: %v", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
