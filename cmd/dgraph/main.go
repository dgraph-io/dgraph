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
	"bytes"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/soheilhy/cmux"
)

var (
	gconf      = flag.String("group_conf", "", "group configuration file")
	postingDir = flag.String("p", "p", "Directory to store posting lists.")
	walDir     = flag.String("w", "w", "Directory to store raft write-ahead logs.")
	port       = flag.Int("port", 8080, "Port to run server on.")
	numcpu     = flag.Int("cores", runtime.NumCPU(),
		"Number of cores to be used by the process")
	nomutations  = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
	tracing      = flag.Float64("trace", 0.5, "The ratio of queries to trace.")
	schemaFile   = flag.String("schema", "", "Path to schema file")
	cpuprofile   = flag.String("cpu", "", "write cpu profile to file")
	memprofile   = flag.String("mem", "", "write memory profile to file")
	dumpSubgraph = flag.String("dumpsg", "", "Directory to save subgraph for testing, debugging")

	closeCh        = make(chan struct{})
	pendingQueries = make(chan struct{}, 10000*runtime.NumCPU())
)

type mutationResult struct {
	edges   []*task.DirectedEdge
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

func convertToNQuad(ctx context.Context, mutation string) ([]*graph.NQuad, error) {
	var nquads []*graph.NQuad
	r := strings.NewReader(mutation)
	reader := bufio.NewReader(r)
	x.Trace(ctx, "Converting to NQuad")

	var strBuf bytes.Buffer
	var err error
	for {
		err = x.ReadLine(reader, &strBuf)
		if err != nil {
			break
		}
		ln := strings.Trim(strBuf.String(), " \t")
		if len(ln) == 0 {
			continue
		}
		nq, err := rdf.Parse(ln)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while parsing RDF"))
			return nquads, err
		}
		nquads = append(nquads, &nq)
	}
	if err != io.EOF {
		return nquads, err
	}
	return nquads, nil
}

func convertToEdges(ctx context.Context, nquads []*graph.NQuad) (mutationResult, error) {
	var edges []*task.DirectedEdge
	var mr mutationResult

	newUids := make(map[string]uint64)
	for _, nq := range nquads {
		if strings.HasPrefix(nq.Subject, "_:") {
			newUids[nq.Subject] = 0
		} else if !strings.HasPrefix(nq.Subject, "_uid_:") {
			uid, err := rdf.GetUid(nq.Subject)
			x.Check(err)
			newUids[nq.Subject] = uid
		}

		if len(nq.ObjectId) > 0 {
			if strings.HasPrefix(nq.ObjectId, "_:") {
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
			x.TraceError(ctx, x.Wrapf(err, "Error while AssignUidsOverNetwork for newUids: %v", newUids))
			return mr, err
		}
	}

	// Wrapper for a pointer to graph.Nquad
	var wnq rdf.NQuad
	for _, nq := range nquads {
		// Get edges from nquad using newUids.
		wnq = rdf.NQuad{nq}
		edge, err := wnq.ToEdgeUsing(newUids)
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while converting to edge: %v", nq))
			return mr, err
		}
		edges = append(edges, edge)
	}

	resultUids := make(map[string]uint64)
	// Strip out _: prefix from the blank node keys.
	for k, v := range newUids {
		if strings.HasPrefix(k, "_:") {
			resultUids[k[2:]] = v
		}
	}

	mr = mutationResult{
		edges:   edges,
		newUids: resultUids,
	}
	return mr, nil
}

func applyMutations(ctx context.Context, m *task.Mutations) error {
	err := worker.MutateOverNetwork(ctx, m)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while MutateOverNetwork"))
		return err
	}
	return nil
}

func convertAndApply(ctx context.Context, set []*graph.NQuad, del []*graph.NQuad) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var m task.Mutations
	var err error
	var mr mutationResult

	if *nomutations {
		return nil, fmt.Errorf("Mutations are forbidden on this server.")
	}

	if mr, err = convertToEdges(ctx, set); err != nil {
		return nil, err
	}
	m.Edges, allocIds = mr.edges, mr.newUids
	for i := range m.Edges {
		m.Edges[i].Op = task.DirectedEdge_SET
	}

	if mr, err = convertToEdges(ctx, del); err != nil {
		return nil, err
	}
	for i := range mr.edges {
		edge := mr.edges[i]
		edge.Op = task.DirectedEdge_DEL
		m.Edges = append(m.Edges, edge)
	}

	if err := applyMutations(ctx, &m); err != nil {
		return nil, x.Wrap(err)
	}
	return allocIds, nil
}

// This function is used to run mutations for the requests from different
// language clients.
func runMutations(ctx context.Context, mu *graph.Mutation) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var err error

	if err = validateTypes(mu.Set); err != nil {
		return nil, x.Wrap(err)
	}
	if err = validateTypes(mu.Del); err != nil {
		return nil, x.Wrap(err)
	}

	if allocIds, err = convertAndApply(ctx, mu.Set, mu.Del); err != nil {
		return nil, err
	}
	return allocIds, nil
}

// This function is used to run mutations for the requests received from the
// http client.
func mutationHandler(ctx context.Context, mu *gql.Mutation) (map[string]uint64, error) {
	var set []*graph.NQuad
	var del []*graph.NQuad
	var allocIds map[string]uint64
	var err error

	if len(mu.Set) > 0 {
		if set, err = convertToNQuad(ctx, mu.Set); err != nil {
			return nil, x.Wrap(err)
		}
	}

	if len(mu.Del) > 0 {
		if del, err = convertToNQuad(ctx, mu.Del); err != nil {
			return nil, x.Wrap(err)
		}
	}

	if err = validateTypes(set); err != nil {
		return nil, x.Wrap(err)
	}
	if err = validateTypes(del); err != nil {
		return nil, x.Wrap(err)
	}

	if allocIds, err = convertAndApply(ctx, set, del); err != nil {
		return nil, err
	}
	return allocIds, nil
}

func getVal(t types.TypeID, val *graph.Value) interface{} {
	switch t {
	case types.StringID:
		return val.GetStrVal()
	case types.Int32ID:
		return val.GetIntVal()
	case types.FloatID:
		return val.GetDoubleVal()
	case types.BoolID:
		return val.GetBoolVal()
	case types.BinaryID:
		return val.GetBytesVal()
	case types.GeoID:
		return val.GetGeoVal()
	case types.DateID:
		return val.GetDateVal()
	case types.DateTimeID:
		return val.GetDatetimeVal()
	}
	return val.GetStrVal()
}

// validateTypes checks for predicate types present in the schema and validates if the
// input value is of the correct type
func validateTypes(nquads []*graph.NQuad) error {
	for i := range nquads {
		nquad := nquads[i]
		if t, err := schema.TypeOf(nquad.Predicate); err == nil && t.IsScalar() {
			schemaType := t
			typeID := types.TypeID(nquad.ObjectType)
			if typeID == types.StringID {
				// Storage type was unspecified in the RDF, so we convert the data to the schema
				// type.
				v := types.ValueForType(schemaType)
				src := types.Val{types.StringID, []byte(nquad.ObjectValue.GetStrVal())}
				err := types.Convert(src, &v) // v.UnmarshalText(nquad.ObjectValue)
				if err != nil {
					return err
				}
				b := types.ValueForType(types.BinaryID)
				err = types.Marshal(v, &b)
				if err != nil {
					return err
				}
				nquad.ObjectValue = &graph.Value{&graph.Value_BytesVal{b.Value.([]byte)}}
				nquad.ObjectType = int32(schemaType)

			} else if typeID != schemaType {
				v := types.ValueForType(schemaType)
				src := types.ValueForType(typeID)
				src.Value = getVal(typeID, nquad.ObjectValue)
				err := types.Convert(src, &v)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	// Add a limit on how many pending queries can be run in the system.
	pendingQueries <- struct{}{}
	defer func() { <-pendingQueries }()

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
	req, err := ioutil.ReadAll(r.Body)
	q := string(req)
	if err != nil || len(q) == 0 {
		x.TraceError(ctx, x.Wrapf(err, "Error while reading query"))
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
		return
	}

	x.Trace(ctx, "Query received: %v", q)
	gq, mu, err := gql.Parse(q)

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

	if gq == nil || (gq.UID == 0 && gq.Func == nil && len(gq.XID) == 0) {
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

func shutDownHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(ip).IsLoopback() {
		x.SetStatus(w, x.ErrorUnauthorized, fmt.Sprintf("Request from IP: %v", ip))
		return
	}

	exitWithProfiles()
	x.SetStatus(w, x.ErrorOk, "Server has been shutdown")
}

func backupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(ip).IsLoopback() {
		x.SetStatus(w, x.ErrorUnauthorized,
			fmt.Sprintf("Request received from IP: %v. Only requests from localhost are allowed.", ip))
		return
	}

	ctx := context.Background()
	err = worker.BackupOverNetwork(ctx)
	if err != nil {
		x.SetStatus(w, err.Error(), "Backup failed.")
	} else {
		x.SetStatus(w, x.ErrorOk, "Backup completed.")
	}
}

// server is used to implement graph.DgraphServer
type grpcServer struct{}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *grpcServer) Run(ctx context.Context,
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
	x.Checkf(s.Serve(l), "Error while serving gRpc request")
}

func serveHTTP(l net.Listener) {
	srv := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second,
		// TODO(Ashwin): Add idle timeout while switching to Go 1.8.
	}
	x.Checkf(srv.Serve(l), "Error while serving http request")
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

	http.HandleFunc("/health", healthCheck)
	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/debug/store", storeStatsHandler)
	http.HandleFunc("/admin/shutdown", shutDownHandler)
	http.HandleFunc("/admin/backup", backupHandler)
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

	// All the writes to posting store should be synchronous. We use batched writers
	// for posting lists, so the cost of sync writes is amortized.
	ps, err := store.NewSyncStore(*postingDir)
	x.Checkf(err, "Error initializing postings store")
	defer ps.Close()

	if len(*schemaFile) > 0 {
		err = schema.Parse(*schemaFile)
		x.Checkf(err, "Error while loading schema: %s", *schemaFile)
	}
	// Posting will initialize index which requires schema. Hence, initialize
	// schema before calling posting.Init().
	posting.Init(ps)
	worker.Init(ps)
	x.Check(group.ParseGroupConfig(*gconf))

	// Setup external communication.
	che := make(chan error, 1)
	go setupServer(che)
	go worker.StartRaftNodes(*walDir)

	if err := <-che; !strings.Contains(err.Error(),
		"use of closed network connection") {
		log.Fatal(err)
	}
}
