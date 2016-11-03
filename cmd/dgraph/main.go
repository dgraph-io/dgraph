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
	"encoding/gob"
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
	"path"
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
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/soheilhy/cmux"
)

var (
	postingDir = flag.String("p", "p", "Directory to store posting lists.")
	walDir     = flag.String("w", "w", "Directory to store raft write-ahead logs.")
	port       = flag.Int("port", 8080, "Port to run server on.")
	numcpu     = flag.Int("cores", runtime.NumCPU(),
		"Number of cores to be used by the process")
	nomutations  = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
	shutdown     = flag.Bool("shutdown", false, "Allow client to send shutdown signal.")
	tracing      = flag.Float64("trace", 0.5, "The ratio of queries to trace.")
	schemaFile   = flag.String("schema", "", "Path to schema file")
	cpuprofile   = flag.String("cpu", "", "write cpu profile to file")
	memprofile   = flag.String("mem", "", "write memory profile to file")
	dumpSubgraph = flag.String("dumpsg", "", "Directory to save subgraph for testing, debugging")

	closeCh = make(chan struct{})
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
	// To exit the server after the response is returned.
	closeCh <- struct{}{}
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
	x.Trace(ctx, "Converting to NQuad")

	// Scanning the mutation string, one line at a time.
	for scanner.Scan() {
		ln := strings.Trim(scanner.Text(), " \t")
		if len(ln) == 0 {
			continue
		}
		nq, err := rdf.Parse(ln)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while parsing RDF"))
			return nquads, err
		}
		nquads = append(nquads, nq)
	}
	return nquads, nil
}

func convertToEdges(ctx context.Context, nquads []rdf.NQuad) (mutationResult, error) {
	var edges []x.DirectedEdge
	var mr mutationResult

	newUids := make(map[string]uint64)
	for _, nq := range nquads {
		if strings.HasPrefix(nq.Subject, "_new_:") {
			newUids[nq.Subject] = 0
		} else if !strings.HasPrefix(nq.Subject, "_uid_:") {
			uid, err := rdf.GetUid(nq.Subject)
			x.Check(err)
			newUids[nq.Subject] = uid
		}

		if len(nq.ObjectId) > 0 {
			if strings.HasPrefix(nq.ObjectId, "_new_:") {
				newUids[nq.ObjectId] = 0
			} else if !strings.HasPrefix(nq.ObjectId, "_uid_:") {
				uid, err := rdf.GetUid(nq.ObjectId)
				x.Check(err)
				newUids[nq.ObjectId] = uid
			}
		}
	}

	if len(newUids) > 0 {
		if err := worker.AssignUidsOverNetwork(ctx, newUids); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while GetOrAssignUidsOverNetwork"))
			return mr, err
		}
	}

	for _, nq := range nquads {
		// Get edges from nquad using newUids.
		edge, err := nq.ToEdgeUsing(newUids)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while converting to edge: %v", nq))
			return mr, err
		}
		edges = append(edges, edge)
	}

	resultUids := make(map[string]uint64)
	// Strip out _new_: prefix from the keys.
	for k, v := range newUids {
		if strings.HasPrefix(k, "_new_:") {
			resultUids[k[6:]] = v
		}
	}

	mr = mutationResult{
		edges:   edges,
		newUids: resultUids,
	}
	return mr, nil
}

func applyMutations(ctx context.Context, m x.Mutations) error {
	err := worker.MutateOverNetwork(ctx, m)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while MutateOverNetwork"))
		return err
	}
	return nil
}

func mutationToNQuad(nq []*graph.NQuad) ([]rdf.NQuad, error) {
	resp := make([]rdf.NQuad, 0, len(nq))

	for _, n := range nq {
		nq := rdf.NQuad{
			Subject:   n.Sub,
			Predicate: n.Pred,
			ObjectId:  n.ObjId,
			Label:     n.Label,
		}
		v, err := typeValueFromNQuad(n)
		if err != nil {
			return resp, err
		}
		if v != nil {
			nq.ObjectValue, _ = v.MarshalBinary()
			nq.ObjectType = byte(v.Type().ID())
		}
		resp = append(resp, nq)
	}
	return resp, nil
}

func typeValueFromNQuad(nq *graph.NQuad) (types.Value, error) {
	if nq.Value == nil || nq.Value.Val == nil {
		return nil, nil
	}
	switch v := nq.Value.Val.(type) {
	case *graph.Value_BytesVal:
		b := types.Bytes(v.BytesVal)
		return &b, nil
	case *graph.Value_IntVal:
		i := types.Int32(v.IntVal)
		return &i, nil
	case *graph.Value_StrVal:
		s := types.String(v.StrVal)
		return &s, nil
	case *graph.Value_BoolVal:
		b := types.Bool(v.BoolVal)
		return &b, nil
	case *graph.Value_DoubleVal:
		f := types.Float(v.DoubleVal)
		return &f, nil
	case *graph.Value_GeoVal:
		var geom types.Geo
		err := geom.UnmarshalBinary(v.GeoVal)
		if err != nil {
			return nil, err
		}
		return &geom, nil

	case nil:
		log.Fatalf("Val being nil is already handled")
		return nil, nil
	default:
		// Unknown type
		return nil, x.Errorf("Unknown value type %T", v)
	}
}

func convertAndApply(ctx context.Context, set []rdf.NQuad, del []rdf.NQuad) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var m x.Mutations
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

	set, _ := mutationToNQuad(mu.Set)
	del, _ := mutationToNQuad(mu.Del)

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
	m := set
	for _, nquad := range del {
		m = append(m, nquad)
	}
	if err = validateTypes(m); err != nil {
		return nil, x.Wrap(err)
	}
	if allocIds, err = convertAndApply(ctx, set, del); err != nil {
		return nil, err
	}
	return allocIds, nil
}

// validateTypes checks for predicate types present in the schema and validates if the
// input value is of the correct type
func validateTypes(nquads []rdf.NQuad) error {
	for _, nquad := range nquads {
		if t := schema.TypeOf(nquad.Predicate); t != nil && t.IsScalar() {
			schemaType := t.(types.Scalar)
			typeID := types.TypeID(nquad.ObjectType)
			if typeID == types.BytesID {
				// Storage type was unspecified in the RDF, so we convert the data to the schema
				// type.
				v := types.ValueForType(schemaType.ID())
				err := v.UnmarshalText(nquad.ObjectValue)
				if err != nil {
					return err
				}
				nquad.ObjectValue, err = v.MarshalBinary()
				if err != nil {
					return err
				}
				nquad.ObjectType = byte(schemaType.ID())
			} else if typeID != schemaType.ID() {
				v := types.ValueForType(typeID)
				err := v.UnmarshalBinary(nquad.ObjectValue)
				if err != nil {
					return err
				}
				if _, err := schemaType.Convert(v); err != nil {
					return err
				}
			}
		}
	}
	return nil
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
		x.TraceError(ctx, x.Wrapf(err, "Error while reading query"))
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
		x.TraceError(ctx, x.Wrapf(err, "Error while parsing query"))
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	var allocIds map[string]uint64
	var allocIdsStr map[string]string
	// If we have mutations, run them first.
	if mu != nil && (len(mu.Set) > 0 || len(mu.Del) > 0) {
		if allocIds, err = mutationHandler(ctx, mu); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
			x.SetStatus(w, x.Error, err.Error())
			return
		}
		// convert the new UIDs to hex string.
		allocIdsStr = make(map[string]string)
		for k, v := range allocIds {
			allocIdsStr[k] = fmt.Sprintf("%#x", v)
		}
	}

	if gq == nil || (gq.UID == 0 && len(gq.XID) == 0) {
		mp := map[string]interface{}{
			"code":    x.ErrorOk,
			"message": "Done",
			"uids":    allocIdsStr,
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
		x.TraceError(ctx, x.Wrapf(err, "Error while conversion o internal format"))
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}
	l.Parsing = time.Since(l.Start)
	x.Trace(ctx, "Query parsed")

	rch := make(chan error)
	go query.ProcessGraph(ctx, sg, nil, rch)
	err = <-rch
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while executing query"))
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	l.Processing = time.Since(l.Start) - l.Parsing
	x.Trace(ctx, "Graph processed")

	if len(*dumpSubgraph) > 0 {
		x.Checkf(os.MkdirAll(*dumpSubgraph, 0700), *dumpSubgraph)
		s := time.Now().Format("20060102.150405.000000.gob")
		filename := path.Join(*dumpSubgraph, s)
		f, err := os.Create(filename)
		x.Checkf(err, filename)
		enc := gob.NewEncoder(f)
		x.Check(enc.Encode(sg))
		x.Checkf(f.Close(), filename)
	}

	js, err := sg.ToJSON(&l)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while converting to Json"))
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	x.Trace(ctx, "Latencies: Total: %v Parsing: %v Process: %v Json: %v",
		time.Since(l.Start), l.Parsing, l.Processing, l.Json)

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

// storeStatsHandler outputs some basic stats for data store.
func storeStatsHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<pre>"))
	w.Write([]byte(worker.StoreStats()))
	w.Write([]byte("</pre>"))
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
		x.TraceError(ctx, x.Errorf("Empty query and mutation."))
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
		x.TraceError(ctx, x.Wrapf(err, "Error while parsing query"))
		return resp, err
	}

	// If mutations are part of the query, we run them through the mutation handler
	// same as the http client.
	if mu != nil && (len(mu.Set) > 0 || len(mu.Del) > 0) {
		if allocIds, err = mutationHandler(ctx, mu); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
			return resp, err
		}
	}

	// If mutations are sent as part of the mutation object in the request we run
	// them here.
	if req.Mutation != nil && (len(req.Mutation.Set) > 0 || len(req.Mutation.Del) > 0) {
		if allocIds, err = runMutations(ctx, req.Mutation); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
			return resp, err
		}
	}
	resp.AssignedUids = allocIds

	if gq == nil || (gq.UID == 0 && len(gq.XID) == 0) {
		return resp, err
	}

	sg, err := query.ToSubGraph(ctx, gq)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while conversion to internal format"))
		return resp, err
	}
	l.Parsing = time.Since(l.Start)
	x.Trace(ctx, "Query parsed")

	rch := make(chan error)
	go query.ProcessGraph(ctx, sg, nil, rch)
	err = <-rch
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while executing query"))
		return resp, err
	}
	l.Processing = time.Since(l.Start) - l.Parsing
	x.Trace(ctx, "Graph processed")

	node, err := sg.ToProtocolBuffer(&l)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while converting to ProtocolBuffer"))
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
		x.Check(err)
		pprof.StartCPUProfile(f)
	}

	prev := runtime.GOMAXPROCS(numCpus)
	log.Printf("num_cpu: %v. prev_maxprocs: %v. Set max procs to num cpus",
		numCpus, prev)
	// Create parent directories for postings, uids and mutations
	x.Check(os.MkdirAll(*postingDir, 0700))
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

func setupServer(che chan error) {
	go worker.RunServer() // For internal communication.

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
	http.HandleFunc("/debug/store", storeStatsHandler)
	// Initilize the servers.
	go serveGRPC(grpcl)
	go serveHTTP(httpl)
	go serveHTTP(http2)

	go func() {
		<-closeCh
		// Stops listening further but already accepted connections are not closed.
		l.Close()
	}()

	log.Println("grpc server started.")
	log.Println("http server started.")
	log.Println("Server listening on port", *port)

	// Start cmux serving.
	che <- tcpm.Serve()
}

func main() {
	rand.Seed(time.Now().UnixNano())
	x.Init()
	checkFlagsAndInitDirs()

	ps, err := store.NewStore(*postingDir)
	x.Checkf(err, "Error initializing postings store")
	defer ps.Close()

	if len(*schemaFile) > 0 {
		err = schema.Parse(*schemaFile)
		if err != nil {
			log.Fatalf("Error while loading schema:%v", err)
		}
	}
	// Posting will initialize index which requires schema. Hence, initialize
	// schema before calling posting.Init().
	posting.Init(ps)
	worker.Init(ps)

	// Setup external communication.
	che := make(chan error, 1)
	go setupServer(che)
	go worker.StartRaftNodes(*walDir)

	if err := <-che; !strings.Contains(err.Error(),
		"use of closed network connection") {
		log.Fatal(err)
	}
}
