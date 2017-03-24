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
	"crypto/tls"
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
	"os/signal"
	"path"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/soheilhy/cmux"
)

var (
	gconf      = flag.String("group_conf", "", "group configuration file")
	postingDir = flag.String("p", "p", "Directory to store posting lists.")
	walDir     = flag.String("w", "w", "Directory to store raft write-ahead logs.")
	port       = flag.Int("port", 8080, "Port to run server on.")
	bindall    = flag.Bool("bindall", false,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	nomutations    = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
	tracing        = flag.Float64("trace", 0.0, "The ratio of queries to trace.")
	cpuprofile     = flag.String("cpu", "", "write cpu profile to file")
	memprofile     = flag.String("mem", "", "write memory profile to file")
	dumpSubgraph   = flag.String("dumpsg", "", "Directory to save subgraph for testing, debugging")
	uiDir          = flag.String("ui", os.Getenv("GOPATH")+"/src/github.com/dgraph-io/dgraph/dashboard/build", "Directory which contains assets for the user interface")
	finishCh       = make(chan struct{}) // channel to wait for all pending reqs to finish.
	shutdownCh     = make(chan struct{}) // channel to signal shutdown.
	pendingQueries = make(chan struct{}, 10000*runtime.NumCPU())
	// TLS configurations
	tlsEnabled       = flag.Bool("tls.on", false, "Use TLS connections with clients.")
	tlsCert          = flag.String("tls.cert", "", "Certificate file path.")
	tlsKey           = flag.String("tls.cert_key", "", "Certificate key file path.")
	tlsKeyPass       = flag.String("tls.cert_key_passphrase", "", "Certificate key passphrase.")
	tlsClientAuth    = flag.String("tls.client_auth", "", "Enable TLS client authentication")
	tlsClientCACerts = flag.String("tls.ca_certs", "", "CA Certs file path.")
	tlsSystemCACerts = flag.Bool("tls.use_system_ca", false, "Include System CA into CA Certs.")
	tlsMinVersion    = flag.String("tls.min_version", "TLS11", "TLS min version.")
	tlsMaxVersion    = flag.String("tls.max_version", "TLS12", "TLS max version.")
)

type mutationResult struct {
	edges   []*taskp.DirectedEdge
	newUids map[string]uint64
}

func stopProfiling() {
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

func convertToNQuad(ctx context.Context, mutation string) ([]*graphp.NQuad, error) {
	var nquads []*graphp.NQuad
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
		if err == rdf.ErrEmpty { // special case: comment/empty line
			continue
		} else if err != nil {
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

func convertToEdges(ctx context.Context, nquads []*graphp.NQuad) (mutationResult, error) {
	var edges []*taskp.DirectedEdge
	var mr mutationResult

	newUids := make(map[string]uint64)
	for _, nq := range nquads {
		if strings.HasPrefix(nq.Subject, "_:") {
			newUids[nq.Subject] = 0
		} else {
			// Only store xids that need to be marked as used.
			if _, err := strconv.ParseInt(nq.Subject, 0, 64); err != nil {
				uid, err := rdf.GetUid(nq.Subject)
				if err != nil {
					return mr, err
				}
				newUids[nq.Subject] = uid
			}
		}

		if len(nq.ObjectId) > 0 {
			if strings.HasPrefix(nq.ObjectId, "_:") {
				newUids[nq.ObjectId] = 0
			} else if !strings.HasPrefix(nq.ObjectId, "_uid_:") {
				uid, err := rdf.GetUid(nq.ObjectId)
				if err != nil {
					return mr, err
				}
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

	// Wrapper for a pointer to graphp.Nquad
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

func applyMutations(ctx context.Context, m *taskp.Mutations) error {
	err := worker.MutateOverNetwork(ctx, m)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while MutateOverNetwork"))
		return err
	}
	return nil
}

func convertAndApply(ctx context.Context, set []*graphp.NQuad, del []*graphp.NQuad) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var m taskp.Mutations
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
		m.Edges[i].Op = taskp.DirectedEdge_SET
	}

	if mr, err = convertToEdges(ctx, del); err != nil {
		return nil, err
	}
	for i := range mr.edges {
		edge := mr.edges[i]
		edge.Op = taskp.DirectedEdge_DEL
		m.Edges = append(m.Edges, edge)
	}

	if err := applyMutations(ctx, &m); err != nil {
		return nil, x.Wrap(err)
	}
	return allocIds, nil
}

// This function is used to run mutations for the requests from different
// language clients.
func runMutations(ctx context.Context, mu *graphp.Mutation) (map[string]uint64, error) {
	var allocIds map[string]uint64
	var err error

	if allocIds, err = convertAndApply(ctx, mu.Set, mu.Del); err != nil {
		return nil, err
	}
	return allocIds, nil
}

// This function is used to run mutations for the requests received from the
// http client.
func mutationHandler(ctx context.Context, mu *gql.Mutation) (map[string]uint64, error) {
	var set []*graphp.NQuad
	var del []*graphp.NQuad
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

	if allocIds, err = convertAndApply(ctx, set, del); err != nil {
		return nil, err
	}
	return allocIds, nil
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	if worker.HealthCheck() {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// parseQueryAndMutation handles the cases where the query parsing code can hang indefinitely.
// We allow 1 second for parsing the query; and then give up.
func parseQueryAndMutation(ctx context.Context, query string) (res gql.Result, err error) {
	x.Trace(ctx, "Query received: %v", query)
	errc := make(chan error, 1)

	go func() {
		var err error
		res, err = gql.Parse(query)
		errc <- err
	}()

	child, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	select {
	case <-child.Done():
		return res, child.Err()
	case err := <-errc:
		if err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while parsing query"))
			return res, err
		}
		x.Trace(ctx, "Query parsed")
	}
	return res, nil
}

type wrappedErr struct {
	Err     error
	Message string
}

func processRequest(ctx context.Context, gq *gql.GraphQuery,
	l *query.Latency) (*query.SubGraph, wrappedErr) {
	if gq == nil || (len(gq.UID) == 0 && gq.Func == nil) {
		return &query.SubGraph{}, wrappedErr{nil, x.Success}
	}

	sg, err := query.ToSubGraph(ctx, gq)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while conversion to internal format"))
		return &query.SubGraph{}, wrappedErr{err, x.ErrorInvalidRequest}
	}

	l.Parsing = time.Since(l.Start)
	x.Trace(ctx, "Query parsed")

	rch := make(chan error)
	go query.ProcessGraph(ctx, sg, nil, rch)
	err = <-rch
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while executing query"))
		return &query.SubGraph{}, wrappedErr{err, x.Error}
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
	return sg, wrappedErr{nil, ""}
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	if !worker.HealthCheck() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
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

	// Lets add the value of the debug query parameter to the context.
	c := context.WithValue(context.Background(), "debug", r.URL.Query().Get("debug"))
	ctx, cancel := context.WithTimeout(c, time.Minute)
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

	res, err := parseQueryAndMutation(ctx, q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var allocIds map[string]uint64
	var allocIdsStr map[string]string
	// If we have mutations, run them first.
	if res.Mutation != nil && (len(res.Mutation.Set) > 0 || len(res.Mutation.Del) > 0) {
		if allocIds, err = mutationHandler(ctx, res.Mutation); err != nil {
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

	var schemaNode []*graphp.SchemaNode
	if res.Schema != nil {
		if schemaNode, err = worker.GetSchemaOverNetwork(ctx, res.Schema); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while fetching schema"))
			x.SetStatus(w, x.Error, err.Error())
			return
		}
	}

	if len(res.Query) == 0 {
		mp := map[string]interface{}{
			"code":    x.Success,
			"message": "Done",
			"uids":    allocIdsStr,
		}
		// Either Schema or query can be specified
		if res.Schema != nil {
			mp["schema"] = schemaNode
		}
		if js, err := json.Marshal(mp); err == nil {
			w.Write(js)
		} else {
			x.SetStatus(w, "Error", "Unable to marshal map")
		}
		return
	}

	sgl, err := query.ProcessQuery(ctx, res, &l)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while Executing query"))
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	if len(*dumpSubgraph) > 0 {
		for _, sg := range sgl {
			x.Checkf(os.MkdirAll(*dumpSubgraph, 0700), *dumpSubgraph)
			s := time.Now().Format("20060102.150405.000000.gob")
			filename := path.Join(*dumpSubgraph, s)
			f, err := os.Create(filename)
			x.Checkf(err, filename)
			enc := gob.NewEncoder(f)
			x.Check(enc.Encode(sg))
			x.Checkf(f.Close(), filename)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	err = query.ToJson(&l, sgl, w, allocIdsStr)
	if err != nil {
		// since we performed w.Write in ToJson above,
		// calling WriteHeader with 500 code will be ignored.
		x.TraceError(ctx, x.Wrapf(err, "Error while converting to Json"))
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	x.Trace(ctx, "Latencies: Total: %v Parsing: %v Process: %v Json: %v",
		time.Since(l.Start), l.Parsing, l.Processing, l.Json)
}

// storeStatsHandler outputs some basic stats for data store.
func storeStatsHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<pre>"))
	w.Write([]byte(worker.StoreStats()))
	w.Write([]byte("</pre>"))
}

// handlerInit does some standard checks. Returns false if something is wrong.
func handlerInit(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "GET" {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return false
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(ip).IsLoopback() {
		x.SetStatus(w, x.ErrorUnauthorized, fmt.Sprintf("Request from IP: %v", ip))
		return false
	}
	return true
}

func shutDownHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}

	shutdownServer()
	x.SetStatus(w, x.Success, "Server is shutting down")
}

func shutdownServer() {
	x.Printf("Got clean exit request")
	stopProfiling()          // stop profiling
	shutdownCh <- struct{}{} // exit grpc and http servers.

	// wait for grpc and http servers to finish pending reqs and
	// then stop all nodes, internal grpc servers and sync all the marks
	go func() {
		defer func() { shutdownCh <- struct{}{} }()

		// wait for grpc, http and http2 servers to stop
		<-finishCh
		<-finishCh
		<-finishCh

		worker.BlockingStop()
	}()
}

func backupHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}
	ctx := context.Background()
	if err := worker.BackupOverNetwork(ctx); err != nil {
		x.SetStatus(w, err.Error(), "Backup failed.")
		return
	}
	x.SetStatus(w, x.Success, "Backup completed.")
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}
	ctx := context.Background()
	attr := r.URL.Query().Get("attr")
	if len(attr) == 0 {
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request. No attr defined.")
		return
	}
	if err := worker.RebuildIndexOverNetwork(ctx, attr); err != nil {
		x.SetStatus(w, err.Error(), "RebuildIndex failed.")
	} else {
		x.SetStatus(w, x.Success, "RebuildIndex completed.")
	}
}

// server is used to implement graphp.DgraphServer
type grpcServer struct{}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *grpcServer) Run(ctx context.Context,
	req *graphp.Request) (resp *graphp.Response, err error) {
	// we need membership information
	if !worker.HealthCheck() {
		x.Trace(ctx, "This server hasn't yet been fully initiated. Please retry later.")
		return resp, x.Errorf("Uninitiated server. Please retry later")
	}
	var allocIds map[string]uint64
	var schemaNodes []*graphp.SchemaNode
	if rand.Float64() < *tracing {
		tr := trace.New("Dgraph", "GrpcQuery")
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	resp = new(graphp.Response)
	if len(req.Query) == 0 && req.Mutation == nil {
		x.TraceError(ctx, x.Errorf("Empty query and mutation."))
		return resp, fmt.Errorf("Empty query and mutation.")
	}

	var l query.Latency
	l.Start = time.Now()
	x.Trace(ctx, "Query received: %v", req.Query)
	res, err := parseQueryAndMutation(ctx, req.Query)
	if err != nil {
		return resp, err
	}

	// If mutations are part of the query, we run them through the mutation handler
	// same as the http client.
	if res.Mutation != nil && (len(res.Mutation.Set) > 0 || len(res.Mutation.Del) > 0) {
		if allocIds, err = mutationHandler(ctx, res.Mutation); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
			return resp, err
		}
	}

	// Mutations are sent as part of the mutation object
	if req.Mutation != nil && (len(req.Mutation.Set) > 0 || len(req.Mutation.Del) > 0) {
		if allocIds, err = runMutations(ctx, req.Mutation); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while handling mutations"))
			return resp, err
		}
	}
	resp.AssignedUids = allocIds

	if req.Schema != nil && res.Schema != nil {
		return resp, x.Errorf("Multiple schema blocks found")
	}
	// Schema Block can be part of query string
	schema := res.Schema
	if schema == nil {
		schema = req.Schema
	}

	if schema != nil {
		if schemaNodes, err = worker.GetSchemaOverNetwork(ctx, schema); err != nil {
			x.TraceError(ctx, x.Wrapf(err, "Error while fetching schema"))
			return resp, err
		}
	}
	resp.Schema = schemaNodes

	sgl, err := query.ProcessQuery(ctx, res, &l)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while converting to ProtocolBuffer"))
		return resp, err
	}

	nodes, err := query.ToProtocolBuf(&l, sgl)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err, "Error while converting to ProtocolBuffer"))
		return resp, err
	}
	resp.N = nodes

	gl := new(graphp.Latency)
	gl.Parsing, gl.Processing, gl.Pb = l.Parsing.String(), l.Processing.String(),
		l.ProtocolBuffer.String()
	resp.L = gl
	return resp, err
}

type keyword struct {
	// Type could be a predicate, function etc.
	Type string `json:"type"`
	Name string `json:"name"`
}

type keywords struct {
	Keywords []keyword `json:"keywords"`
}

// Used to return a list of keywords, so that UI can show them for autocompletion.
func keywordHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	// TODO: Remove this code and replace with query from ui
	preds := schema.State().Predicates(1)
	kw := make([]keyword, 0, len(preds))
	for _, p := range preds {
		kw = append(kw, keyword{
			Type: "predicate",
			Name: p,
		})
	}
	kws := keywords{Keywords: kw}

	predefined := []string{"id", "_uid_", "after", "first", "offset", "count",
		"@facets", "@filter", "func", "anyofterms", "allofterms", "anyoftext", "alloftext", "leq", "geq", "or", "and",
		"orderasc", "orderdesc", "near", "within", "contains", "intersects"}

	for _, w := range predefined {
		kws.Keywords = append(kws.Keywords, keyword{
			Name: w,
		})
	}
	js, err := json.Marshal(kws)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(js)
}

func checkFlagsAndInitDirs() {
	if len(*cpuprofile) > 0 {
		f, err := os.Create(*cpuprofile)
		x.Check(err)
		pprof.StartCPUProfile(f)
	}

	// Create parent directories for postings, uids and mutations
	x.Check(os.MkdirAll(*postingDir, 0700))
}

func setupListener(addr string, port int) (net.Listener, error) {
	laddr := fmt.Sprintf("%s:%d", addr, port)
	if !*tlsEnabled {
		return net.Listen("tcp", laddr)
	}

	tlsCfg, err := x.GenerateTLSConfig(x.TLSHelperConfig{
		CertRequired:           *tlsEnabled,
		Cert:                   *tlsCert,
		Key:                    *tlsKey,
		KeyPassphrase:          *tlsKeyPass,
		ClientAuth:             *tlsClientAuth,
		ClientCACerts:          *tlsClientCACerts,
		UseSystemClientCACerts: *tlsSystemCACerts,
		MinVersion:             *tlsMinVersion,
		MaxVersion:             *tlsMaxVersion,
	})
	if err != nil {
		return nil, err
	}

	return tls.Listen("tcp", laddr, tlsCfg)
}

func serveGRPC(l net.Listener) {
	defer func() { finishCh <- struct{}{} }()
	s := grpc.NewServer(grpc.CustomCodec(&query.Codec{}))
	graphp.RegisterDgraphServer(s, &grpcServer{})
	err := s.Serve(l)
	log.Printf("gRpc server stopped : %s", err.Error())
	s.GracefulStop()
}

func serveHTTP(l net.Listener) {
	defer func() { finishCh <- struct{}{} }()
	srv := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 600 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}

	err := srv.Serve(l)
	log.Printf("Stopped taking more http(s) requests. Err: %s", err.Error())
	ctx, cancel := context.WithTimeout(context.Background(), 630*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	log.Printf("All http(s) requests finished.")
	if err != nil {
		log.Printf("Http(s) shutdown err: ", err.Error())
	}
}

func setupServer(che chan error) {
	go worker.RunServer(*bindall) // For internal communication.

	laddr := "localhost"
	if *bindall {
		laddr = "0.0.0.0"
	}

	l, err := setupListener(laddr, *port)
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
	http.HandleFunc("/admin/index", indexHandler)
	http.HandleFunc("/admin/shutdown", shutDownHandler)
	http.HandleFunc("/admin/backup", backupHandler)

	// UI related API's.
	http.Handle("/", http.FileServer(http.Dir(*uiDir)))
	http.HandleFunc("/ui/keywords", keywordHandler)

	// Initilize the servers.
	go serveGRPC(grpcl)
	go serveHTTP(httpl)
	go serveHTTP(http2)

	go func() {
		<-shutdownCh
		// Stops grpc/http servers; Already accepted connections are not closed.
		l.Close()
	}()

	log.Println("grpc server started.")
	log.Println("http server started.")
	log.Println("Server listening on port", *port)

	err = tcpm.Serve() // Start cmux serving. blocking call
	<-shutdownCh       // wait for shutdownServer to finish
	che <- err         // final close for main.
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

	x.Check(group.ParseGroupConfig(*gconf))
	schema.Init(ps)

	// Posting will initialize index which requires schema. Hence, initialize
	// schema before calling posting.Init().
	posting.Init(ps)
	worker.Init(ps)

	// setup shutdown os signal handler
	sdCh := make(chan os.Signal, 1)
	defer close(sdCh)
	signal.Notify(sdCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		_, ok := <-sdCh
		if ok {
			shutdownServer()
		}
	}()

	// Setup external communication.
	che := make(chan error, 1)
	go setupServer(che)
	go worker.StartRaftNodes(*walDir)

	if err := <-che; !strings.Contains(err.Error(),
		"use of closed network connection") {
		log.Fatal(err)
	}
}
