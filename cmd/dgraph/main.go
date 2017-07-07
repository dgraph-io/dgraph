/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"crypto/sha256"
	"crypto/tls"
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
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dgraph-io/badger"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"

	"github.com/cockroachdb/cmux"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

var (
	gconf        = flag.String("group_conf", "", "group configuration file")
	postingDir   = flag.String("p", "p", "Directory to store posting lists.")
	walDir       = flag.String("w", "w", "Directory to store raft write-ahead logs.")
	baseHttpPort = flag.Int("port", 8080, "Port to run HTTP service on.")
	baseGrpcPort = flag.Int("grpc_port", 9080, "Port to run gRPC service on.")
	bindall      = flag.Bool("bindall", false,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	nomutations = flag.Bool("nomutations", false, "Don't allow mutations on this server.")
	exposeTrace = flag.Bool("expose_trace", false,
		"Allow trace endpoint to be accessible from remote")
	cpuprofile   = flag.String("cpu", "", "write cpu profile to file")
	memprofile   = flag.String("mem", "", "write memory profile to file")
	blockRate    = flag.Int("block", 0, "Block profiling rate")
	dumpSubgraph = flag.String("dumpsg", "", "Directory to save subgraph for testing, debugging")
	numPending   = flag.Int("pending", 1000,
		"Number of pending queries. Useful for rate limiting.")
	finishCh       = make(chan struct{}) // channel to wait for all pending reqs to finish.
	shutdownCh     = make(chan struct{}) // channel to signal shutdown.
	pendingQueries chan struct{}
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

func httpPort() int {
	return *x.PortOffset + *baseHttpPort
}

func grpcPort() int {
	return *x.PortOffset + *baseGrpcPort
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

func isMutationAllowed(ctx context.Context) bool {
	if !*nomutations {
		return true
	}
	shareAllowed, ok := ctx.Value("_share_").(bool)
	if !ok || !shareAllowed {
		return false
	}
	return true
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	if err := x.HealthCheck(); err == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// parseQueryAndMutation handles the cases where the query parsing code can hang indefinitely.
// We allow 1 second for parsing the query; and then give up.
func parseQueryAndMutation(ctx context.Context, r gql.Request) (res gql.Result, err error) {
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Query received: %v", r.Str)
	}
	errc := make(chan error, 1)

	go func() {
		var err error
		res, err = gql.Parse(r)
		errc <- err
	}()

	child, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	select {
	case <-child.Done():
		return res, child.Err()
	case err := <-errc:
		if err != nil {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while parsing query: %+v", err)
			}
			return res, err
		}
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Query parsed")
		}
	}
	return res, nil
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	if err := x.HealthCheck(); err != nil {
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
		w.WriteHeader(http.StatusBadRequest)
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}

	// Lets add the value of the debug query parameter to the context.
	ctx := context.WithValue(context.Background(), "debug", r.URL.Query().Get("debug"))
	ctx = context.WithValue(ctx, "mutation_allowed", !*nomutations)

	if rand.Float64() < *worker.Tracing {
		tr := trace.New("Dgraph", "Query")
		tr.SetMaxEvents(1000)
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	invalidRequest := func(err error, msg string) {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while reading query: %+v", err)
		}
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
	}

	var l query.Latency
	l.Start = time.Now()
	defer r.Body.Close()
	req, err := ioutil.ReadAll(r.Body)
	q := string(req)
	if err != nil || len(q) == 0 {
		invalidRequest(err, "Error while reading query")
		return
	}

	parseStart := time.Now()
	parsed, err := parseQueryAndMutation(ctx, gql.Request{
		Str:       q,
		Variables: map[string]string{},
		Http:      true,
	})
	l.Parsing += time.Since(parseStart)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var cancel context.CancelFunc
	// set timeout if schema mutation not present
	if parsed.Mutation == nil || len(parsed.Mutation.Schema) == 0 {
		// If schema mutation is not present
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}

	var res query.ExecuteResult
	var queryRequest = query.QueryRequest{Latency: &l, GqlQuery: &parsed}
	if res, err = queryRequest.ProcessWithMutation(ctx); err != nil {
		switch errors.Cause(err).(type) {
		case *query.InvalidRequestError:
			invalidRequest(err, err.Error())
		default: // internalError or other
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while handling mutations: %+v", err)
			}
			x.SetStatus(w, x.Error, err.Error())
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var addLatency bool
	// If there is an error parsing, then addLatency would remain false.
	addLatency, _ = strconv.ParseBool(r.URL.Query().Get("latency"))
	debug, _ := strconv.ParseBool(r.URL.Query().Get("debug"))
	addLatency = addLatency || debug

	newUids := query.ConvertUidsToHex(res.Allocations)
	if len(parsed.Query) == 0 {
		mp := map[string]interface{}{}
		if parsed.Mutation != nil {
			mp["code"] = x.Success
			mp["message"] = "Done"
			mp["uids"] = newUids
		}
		// Either Schema or query can be specified
		if parsed.Schema != nil {
			js, err := json.Marshal(res.SchemaNode)
			if err != nil {
				x.SetStatus(w, "Error", "Unable to marshal schema")
			}
			mp["schema"] = json.RawMessage(string(js))
			if addLatency {
				mp["server_latency"] = l.ToMap()
			}
		}
		if js, err := json.Marshal(mp); err == nil {
			w.Write(js)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			x.SetStatus(w, "Error", "Unable to marshal map")
		}
		return
	}

	if len(*dumpSubgraph) > 0 {
		for _, sg := range res.Subgraphs {
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
	err = query.ToJson(&l, res.Subgraphs, w,
		query.ConvertUidsToHex(res.Allocations), addLatency)
	if err != nil {
		// since we performed w.Write in ToJson above,
		// calling WriteHeader with 500 code will be ignored.
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while converting to JSON: %+v", err)
		}
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Latencies: Total: %v Parsing: %v Process: %v Json: %v",
			time.Since(l.Start), l.Parsing, l.Processing, l.Json)
	}
}

// NewSharedQueryNQuads returns nquads with query and hash.
func NewSharedQueryNQuads(query []byte) []*protos.NQuad {
	val := func(s string) *protos.Value {
		return &protos.Value{&protos.Value_DefaultVal{s}}
	}
	qHash := fmt.Sprintf("\"%x\"", sha256.Sum256(query))
	return []*protos.NQuad{
		{Subject: "<_:share>", Predicate: "<_share_>", ObjectValue: val(string(query))},
		{Subject: "<_:share>", Predicate: "<_share_hash_>", ObjectValue: val(qHash)},
	}
}

// shareHandler allows to share a query between users.
func shareHandler(w http.ResponseWriter, r *http.Request) {
	var mr query.InternalMutation
	var err error
	var rawQuery []byte

	w.Header().Set("Content-Type", "application/json")
	addCorsHeaders(w)
	if r.Method != "POST" {
		x.SetStatus(w, x.ErrorInvalidMethod, "Invalid method")
		return
	}
	ctx := context.Background()
	defer r.Body.Close()
	if rawQuery, err = ioutil.ReadAll(r.Body); err != nil || len(rawQuery) == 0 {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while reading the stringified query payload: %+v", err)
		}
		x.SetStatus(w, x.ErrorInvalidRequest, "Invalid request encountered.")
		return
	}

	fail := func() {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error: %+v", err)
		}
		x.SetStatus(w, x.Error, err.Error())
	}
	nquads := gql.WrapNQ(NewSharedQueryNQuads(rawQuery), protos.DirectedEdge_SET)
	if mr, err = query.ToInternal(ctx, nquads, nil); err != nil {
		fail()
		return
	}
	if err = query.ApplyMutations(ctx, &protos.Mutations{Edges: mr.Edges}); err != nil {
		fail()
		return
	}
	allocIdsStr := query.ConvertUidsToHex(mr.NewUids)
	payload := map[string]interface{}{
		"code":    x.Success,
		"message": "Done",
		"uids":    allocIdsStr,
	}

	if res, err := json.Marshal(payload); err == nil {
		w.Write(res)
	} else {
		x.SetStatus(w, "Error", "Unable to marshal map")
	}
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

func exportHandler(w http.ResponseWriter, r *http.Request) {
	if !handlerInit(w, r) {
		return
	}
	ctx := context.Background()
	if err := worker.ExportOverNetwork(ctx); err != nil {
		x.SetStatus(w, err.Error(), "Export failed.")
		return
	}
	x.SetStatus(w, x.Success, "Export completed.")
}

func hasGraphOps(mu *protos.Mutation) bool {
	return len(mu.Set) > 0 || len(mu.Del) > 0 || len(mu.Schema) > 0
}

// server is used to implement protos.DgraphServer
type grpcServer struct{}

// This method is used to execute the query and return the response to the
// client as a protocol buffer message.
func (s *grpcServer) Run(ctx context.Context,
	req *protos.Request) (resp *protos.Response, err error) {
	// we need membership information
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Request rejected %v", err)
		}
		return resp, err
	}
	pendingQueries <- struct{}{}
	defer func() { <-pendingQueries }()
	if ctx.Err() != nil {
		return resp, ctx.Err()
	}

	if rand.Float64() < *worker.Tracing {
		tr := trace.New("Dgraph", "GrpcQuery")
		tr.SetMaxEvents(1000)
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	// Sanitize the context of the keys used for internal purposes only
	ctx = context.WithValue(ctx, "_share_", nil)
	ctx = context.WithValue(ctx, "mutation_allowed", isMutationAllowed(ctx))

	resp = new(protos.Response)
	emptyMutation := len(req.Mutation.GetSet()) == 0 && len(req.Mutation.GetDel()) == 0 &&
		len(req.Mutation.GetSchema()) == 0
	if len(req.Query) == 0 && emptyMutation {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Empty query and mutation.")
		}
		return resp, fmt.Errorf("empty query and mutation.")
	}

	var l query.Latency
	l.Start = time.Now()
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("Query received: %v, variables: %v", req.Query, req.Vars)
	}
	res, err := parseQueryAndMutation(ctx, gql.Request{
		Str:       req.Query,
		Variables: req.Vars,
		Http:      false,
	})
	if err != nil {
		return resp, err
	}

	var cancel context.CancelFunc
	// set timeout if schema mutation not present
	if res.Mutation == nil || len(res.Mutation.Schema) == 0 {
		// If schema mutation is not present
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}

	if req.Schema != nil && res.Schema != nil {
		return resp, x.Errorf("Multiple schema blocks found")
	}
	// Schema Block and Mutation can be part of query string or request
	if res.Mutation == nil {
		res.Mutation = &gql.Mutation{Set: req.Mutation.Set, Del: req.Mutation.Del}
	}
	if res.Schema == nil {
		res.Schema = req.Schema
	}

	var queryRequest = query.QueryRequest{
		Latency:      &l,
		GqlQuery:     &res,
		SchemaUpdate: req.Mutation.Schema,
	}
	var er query.ExecuteResult
	if er, err = queryRequest.ProcessWithMutation(ctx); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while processing query: %+v", err)
		}
		return resp, x.Wrap(err)
	}
	resp.AssignedUids = er.Allocations
	resp.Schema = er.SchemaNode

	nodes, err := query.ToProtocolBuf(&l, er.Subgraphs)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while converting to protocol buffer: %+v", err)
		}
		return resp, err
	}
	resp.N = nodes

	gl := new(protos.Latency)
	gl.Parsing, gl.Processing, gl.Pb = l.Parsing.String(), l.Processing.String(),
		l.ProtocolBuffer.String()
	resp.L = gl
	return resp, err
}

func (s *grpcServer) CheckVersion(ctx context.Context, c *protos.Check) (v *protos.Version,
	err error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("request rejected %v", err)
		}
		return v, err
	}

	v = new(protos.Version)
	v.Tag = x.Version()
	return v, nil
}

func (s *grpcServer) AssignUids(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	if err := x.HealthCheck(); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("request rejected %v", err)
		}
		return &protos.AssignedIds{}, err
	}
	return worker.AssignUidsOverNetwork(ctx, num)
}

func bestEffortGopath() (string, bool) {
	if path, ok := os.LookupEnv("GOPATH"); ok {
		return path, true
	}
	var homevar string
	switch runtime.GOOS {
	case "windows":
		// The Golang issue https://github.com/golang/go/issues/17262 says
		// USERPROFILE, _not_ HOMEDRIVE + HOMEPATH is used.
		homevar = "USERPROFILE"
	case "plan9":
		homevar = "home"
	default:
		homevar = "HOME"
	}
	if homepath, ok := os.LookupEnv(homevar); ok {
		return path.Join(homepath, "go"), true
	}
	return "", false
}

var uiDir string

func init() {
	// uiDir can also be set through -ldflags while doing a release build. In that
	// case it points to usr/local/share/dgraph/assets where we store assets for
	// the user. In other cases, it should point to the build directory within the repository.
	flag.StringVar(&uiDir, "ui", uiDir, "Directory which contains assets for the user interface")
	if uiDir == "" {
		gopath, _ := bestEffortGopath()
		uiDir = gopath + "/src/github.com/dgraph-io/dgraph/dashboard/build"
	}
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

func setupListener(addr string, port int) (listener net.Listener, err error) {
	var reload func()
	laddr := fmt.Sprintf("%s:%d", addr, port)
	if !*tlsEnabled {
		listener, err = net.Listen("tcp", laddr)
	} else {
		var tlsCfg *tls.Config
		tlsCfg, reload, err = x.GenerateTLSConfig(x.TLSHelperConfig{
			ConfigType:             x.TLSServerConfig,
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
		listener, err = tls.Listen("tcp", laddr, tlsCfg)
	}
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGHUP)
		for range sigChan {
			log.Println("SIGHUP signal received")
			if reload != nil {
				reload()
				log.Println("TLS certificates and CAs reloaded")
			}
		}
	}()
	return listener, err
}

func serveGRPC(l net.Listener) {
	defer func() { finishCh <- struct{}{} }()
	s := grpc.NewServer(grpc.CustomCodec(&query.Codec{}),
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize))
	protos.RegisterDgraphServer(s, &grpcServer{})
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
		log.Printf("Http(s) shutdown err: %v", err.Error())
	}
}

func setupServer(che chan error) {
	// By default Go GRPC traces all requests.
	grpc.EnableTracing = false
	go worker.RunServer(*bindall) // For internal communication.

	laddr := "localhost"
	if *bindall {
		laddr = "0.0.0.0"
	}

	httpListener, err := setupListener(laddr, httpPort())
	if err != nil {
		log.Fatal(err)
	}

	grpcListener, err := setupListener(laddr, grpcPort())
	if err != nil {
		log.Fatal(err)
	}

	httpMux := cmux.New(httpListener)
	httpl := httpMux.Match(cmux.HTTP1Fast())
	http2 := httpMux.Match(cmux.HTTP2())

	http.HandleFunc("/health", healthCheck)
	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/share", shareHandler)
	http.HandleFunc("/debug/store", storeStatsHandler)
	http.HandleFunc("/admin/shutdown", shutDownHandler)
	http.HandleFunc("/admin/export", exportHandler)

	// UI related API's.
	// Share urls have a hex string as the shareId. So if
	// our url path matches it, we wan't to serve index.html.
	reg := regexp.MustCompile(`\/0[xX][0-9a-fA-F]+`)
	http.Handle("/", homeHandler(http.FileServer(http.Dir(uiDir)), reg))
	http.HandleFunc("/ui/keywords", keywordHandler)

	// Initilize the servers.
	go serveGRPC(grpcListener)
	go serveHTTP(httpl)
	go serveHTTP(http2)

	go func() {
		<-shutdownCh
		// Stops grpc/http servers; Already accepted connections are not closed.
		grpcListener.Close()
		httpListener.Close()
	}()

	log.Println("gRPC server started.  Listening on port", grpcPort())
	log.Println("HTTP server started.  Listening on port", httpPort())

	err = httpMux.Serve() // Start cmux serving. blocking call
	<-shutdownCh          // wait for shutdownServer to finish
	che <- err            // final close for main.
}

func main() {
	rand.Seed(time.Now().UnixNano())
	x.Init()
	if *exposeTrace {
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
	}
	checkFlagsAndInitDirs()
	runtime.SetBlockProfileRate(*blockRate)
	pendingQueries = make(chan struct{}, *numPending)

	pd, err := filepath.Abs(*postingDir)
	x.Check(err)
	wd, err := filepath.Abs(*walDir)
	x.Check(err)
	x.AssertTruef(pd != wd, "Posting and WAL directory cannot be the same.")

	// All the writes to posting store should be synchronous. We use batched writers
	// for posting lists, so the cost of sync writes is amortized.
	opt := badger.DefaultOptions
	opt.SyncWrites = true
	opt.Dir = *postingDir
	opt.ValueDir = *postingDir
	ps, err := badger.NewKV(&opt)
	x.Checkf(err, "Error while creating badger KV posting store")
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
	// sigint : Ctrl-C, sigquit : Ctrl-\ (backslash), sigterm : kill command.
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
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
