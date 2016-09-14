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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/soheilhy/cmux"
)

var (
	postingDir  = flag.String("postings", "p", "Directory to store posting lists")
	uidDir      = flag.String("uids", "u", "XID UID posting lists directory")
	mutationDir = flag.String("mutations", "m", "Directory to store mutations")
	port        = flag.Int("port", 8080, "Port to run server on.")
	numcpu      = flag.Int("numCpu", runtime.NumCPU(),
		"Number of cores to be used by the process")
	instanceIdx = flag.Uint64("instanceIdx", 0,
		"serves only entities whose Fingerprint % numInstance == instanceIdx.")
	workers = flag.String("workers", "",
		"Comma separated list of IP addresses of workers")
	workerPort = flag.String("workerport", ":12345",
		"Port used by worker for internal communication.")
	nomutations = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
	shutdown    = flag.Bool("shutdown", false, "Allow client to send shutdown signal.")
	tracing     = flag.Float64("trace", 0.5, "The ratio of queries to trace.")
	schemaFile  = flag.String("schema", "", "Path to the file that specifies schema in json format")
	cpuprofile  = flag.String("cpuprof", "", "write cpu profile to file")
	memprofile  = flag.String("memprof", "", "write memory profile to file")
)

type mutationResult struct {
	edges   []x.DirectedEdge
	newUids map[string]uint64
}

func exitWithProfiles() {
	log.Println("Got clean exit request")

	// Stop the CPU profiling that was initiated.
	if len(*cpuprofile) > 0 {
		pprof.StopCPUProfile()
	}

	// Write memory profile before exit.
	if len(*memprofile) > 0 {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Println(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}

	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
}

func addCorsHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers",
		"Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token,"+
			"X-Auth-Token, Cache-Control, X-Requested-With")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Connection", "close")
}

func convertToNQuad(ctx context.Context, mutation string) ([]rdf.NQuad, error) {
	var nquads []rdf.NQuad
	r := strings.NewReader(mutation)
	scanner := bufio.NewScanner(r)

	// Scanning the mutation string, one line at a time.
	for scanner.Scan() {
		ln := strings.Trim(scanner.Text(), " \t")
		if len(ln) == 0 {
			continue
		}
		nq, err := rdf.Parse(ln)
		if err != nil {
			x.Trace(ctx, "Error while parsing RDF: %v", err)
			return nquads, err
		}
		nquads = append(nquads, nq)
	}
	return nquads, nil
}

func convertToEdges(ctx context.Context, nquads []rdf.NQuad) (mutationResult, error) {
	var edges []x.DirectedEdge
	var mr mutationResult
	allocatedIds := make(map[string]uint64)

	// xidToUid is used to store ids which are not uids. It is sent to the instance
	// which has the xid <-> uid mapping to get uids.
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
		if err := worker.GetOrAssignUidsOverNetwork(ctx, xidToUid); err != nil {
			x.Trace(ctx, "Error while GetOrAssignUidsOverNetwork: %v", err)
			return mr, err
		}
	}

	for _, nq := range nquads {
		// Get edges from nquad using xidToUid.
		edge, err := nq.ToEdgeUsing(xidToUid)
		if err != nil {
			x.Trace(ctx, "Error while converting to edge: %v %v", nq, err)
			return mr, err
		}
		edges = append(edges, edge)
	}

	for k, v := range xidToUid {
		if strings.HasPrefix(k, "_new_:") {
			allocatedIds[k[6:]] = v
		}
	}

	mr = mutationResult{
		edges:   edges,
		newUids: allocatedIds,
	}
	return mr, nil
}

func applyMutations(ctx context.Context, m worker.Mutations) error {
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

func mutationToNQuad(nq []*graph.NQuad) []rdf.NQuad {
	resp := make([]rdf.NQuad, 0, len(nq))

	for _, n := range nq {
		resp = append(resp, rdf.NQuad{
			Subject:     n.Sub,
			Predicate:   n.Pred,
			ObjectId:    n.ObjId,
			ObjectValue: n.ObjVal,
			Label:       n.Label,
		})
	}
	return resp
}

func convertAndApply(ctx context.Context, set []rdf.NQuad, del []rdf.NQuad) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var m worker.Mutations
	var err error
	var mr mutationResult

	if *nomutations {
		return nil, fmt.Errorf("Mutations are forbidden on this server.")
	}

	if mr, err = convertToEdges(ctx, set); err != nil {
		return nil, err
	}
	m.Set, allocIds = mr.edges, mr.newUids
	if mr, err = convertToEdges(ctx, del); err != nil {
		return nil, err
	}
	m.Del = mr.edges

	if err := applyMutations(ctx, m); err != nil {
		return nil, x.Wrap(err)
	}
	return allocIds, nil
}

// This function is used to run mutations for the requests from different
// language clients.
func runMutations(ctx context.Context, mu *graph.Mutation) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var err error

	set := mutationToNQuad(mu.Set)
	del := mutationToNQuad(mu.Del)
	if allocIds, err = convertAndApply(ctx, set, del); err != nil {
		return nil, err
	}
	return allocIds, nil
}

// This function is used to run mutations for the requests received from the
// http client.
func mutationHandler(ctx context.Context, mu *gql.Mutation) (map[string]uint64, error) {
	var set []rdf.NQuad
	var del []rdf.NQuad
	var allocIds map[string]uint64
	var err error

	if set, err = convertToNQuad(ctx, mu.Set); err != nil {
		return nil, x.Wrap(err)
	}
	if del, err = convertToNQuad(ctx, mu.Del); err != nil {
		return nil, x.Wrap(err)
	}
	if allocIds, err = convertAndApply(ctx, set, del); err != nil {
		return nil, err
	}
	return allocIds, nil
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == "OPTIONS" {
		return
	}
	if r.Method != "POST" {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
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
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
		return
	}

	if *shutdown && string(q) == "SHUTDOWN" {
		exitWithProfiles()
		x.SetStatus(w, x.ErrorOk, "Server has been shutdown")
		return
	}

	x.Trace(ctx, "Query received: %v", string(q))
	gq, mu, err := gql.Parse(string(q))
	if err != nil {
		x.Trace(ctx, "Error while parsing query: %v", err)
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	var allocIds map[string]uint64
	// If we have mutations, run them first.
	if mu != nil && (len(mu.Set) > 0 || len(mu.Del) > 0) {
		if allocIds, err = mutationHandler(ctx, mu); err != nil {
			x.Trace(ctx, "Error while handling mutations: %v", err)
			x.SetStatus(w, x.Error, err.Error())
			return
		}
	}

	if gq == nil || (gq.UID == 0 && len(gq.XID) == 0) {
		mp := map[string]interface{}{
			"code":    x.ErrorOk,
			"message": "Done",
			"uids":    allocIds,
		}
		if js, err := json.Marshal(mp); err == nil {
			w.Write(js)
		} else {
			x.SetStatus(w, "Error", "Unable to marshal map")
		}
		return
	}

	sg, err := query.ToSubGraph(ctx, gq)
	if err != nil {
		x.Trace(ctx, "Error while conversion o internal format: %v", err)
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	l.Parsing = time.Since(l.Start)
	x.Trace(ctx, "Query parsed")

	rch := make(chan error)
	go query.ProcessGraph(ctx, sg, rch)
	err = <-rch
	if err != nil {
		x.Trace(ctx, "Error while executing query: %v", err)
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	l.Processing = time.Since(l.Start) - l.Parsing
	x.Trace(ctx, "Graph processed")
	js, err := sg.ToJSON(&l)
	if err != nil {
		x.Trace(ctx, "Error while converting to Json: %v", err)
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	x.Trace(ctx, "Latencies: Total: %v Parsing: %v Process: %v Json: %v",
		time.Since(l.Start), l.Parsing, l.Processing, l.Json)

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// server is used to implement graph.DgraphServer
type grpcServer struct{}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *grpcServer) Query(ctx context.Context,
	req *graph.Request) (*graph.Response, error) {

	var allocIds map[string]uint64
	if rand.Float64() < *tracing {
		tr := trace.New("Dgraph", "GrpcQuery")
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	resp := new(graph.Response)
	if len(req.Query) == 0 && req.Mutation == nil {
		x.Trace(ctx, "Empty query and mutation.")
		return resp, fmt.Errorf("Empty query and mutation.")
	}

	if *shutdown && req.Query == "SHUTDOWN" {
		exitWithProfiles()
		return nil, nil
	}

	var l query.Latency
	l.Start = time.Now()
	x.Trace(ctx, "Query received: %v", req.Query)
	gq, mu, err := gql.Parse(req.Query)
	if err != nil {
		x.Trace(ctx, "Error while parsing query: %v", err)
		return resp, err
	}

	// If mutations are part of the query, we run them through the mutation handler
	// same as the http client.
	if mu != nil && (len(mu.Set) > 0 || len(mu.Del) > 0) {
		if allocIds, err = mutationHandler(ctx, mu); err != nil {
			x.Trace(ctx, "Error while handling mutations: %v", err)
			return resp, err
		}
	}

	// If mutations are sent as part of the mutation object in the request we run
	// them here.
	if req.Mutation != nil && (len(req.Mutation.Set) > 0 || len(req.Mutation.Del) > 0) {
		if allocIds, err = runMutations(ctx, req.Mutation); err != nil {
			x.Trace(ctx, "Error while handling mutations: %v", err)
			return resp, err
		}
	}
	resp.AssignedUids = allocIds

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

func checkFlagsAndInitDirs() {
	numCpus := *numcpu
	if len(*cpuprofile) > 0 {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	}

	prev := runtime.GOMAXPROCS(numCpus)
	log.Printf("num_cpu: %v. prev_maxprocs: %v. Set max procs to num cpus",
		numCpus, prev)
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
}

func serveGRPC(l net.Listener) {
	s := grpc.NewServer(grpc.CustomCodec(&query.Codec{}))
	graph.RegisterDgraphServer(s, &grpcServer{})
	if err := s.Serve(l); err != nil {
		log.Fatalf("While serving gRpc request: %v", err)
	}
}

func serveHTTP(l net.Listener) {
	if err := http.Serve(l, nil); err != nil {
		log.Fatalf("While serving http request: %v", err)
	}
}

func setupServer() {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
	}

	tcpm := cmux.New(l)
	httpl := tcpm.Match(cmux.HTTP1Fast())
	grpcl := tcpm.MatchWithWriters(
		cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
	http2 := tcpm.Match(cmux.HTTP2())

	http.HandleFunc("/query", queryHandler)
	// Initilize the servers.
	go serveGRPC(grpcl)
	go serveHTTP(httpl)
	go serveHTTP(http2)

	log.Println("grpc server started.")
	log.Println("http server started.")
	log.Println("Server listening on port", *port)

	// Start cmux serving.
	if err := tcpm.Serve(); !strings.Contains(err.Error(),
		"use of closed network connection") {
		log.Fatal(err)
	}
}

func main() {
	x.Init()
	checkFlagsAndInitDirs()

	ps, err := store.NewStore(*postingDir)
	x.Checkf(err, "Error initializing postings store")
	defer ps.Close()

	posting.InitIndex(ps)

	addrs := strings.Split(*workers, ",")
	lenAddr := uint64(len(addrs))
	if lenAddr == 0 {
		// If no worker is specified, then we're it.
		lenAddr = 1
	}

	posting.Init()
	var ws *worker.State
	if *instanceIdx != 0 {
		ws = worker.NewState(ps, nil, *instanceIdx, lenAddr)
		worker.SetWorkerState(ws)
		uid.Init(nil)
	} else {
		uidStore, err := store.NewStore(*uidDir)
		if err != nil {
			log.Fatalf("error initializing uid store: %s", err)
		}
		defer uidStore.Close()
		// Only server instance 0 will have uidStore
		ws = worker.NewState(ps, uidStore, *instanceIdx, lenAddr)
		worker.SetWorkerState(ws)
		uid.Init(uidStore)
	}

	if len(*schemaFile) > 0 {
		err = gql.LoadSchema(*schemaFile)
		if err != nil {
			log.Fatalf("Error while loading schema:%v", err)
		}
	}
	// Initiate internal worker communication.
	ws.Connect(addrs, *workerPort)

	// Setup external communication.
	setupServer()
}
