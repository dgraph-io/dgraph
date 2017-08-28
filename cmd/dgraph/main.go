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
	"regexp"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"

	"github.com/cockroachdb/cmux"
	"github.com/dgraph-io/dgraph/dgraph"
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
	gconf        string
	baseHttpPort int
	baseGrpcPort int
	bindall      bool

	exposeTrace  bool
	cpuprofile   string
	memprofile   string
	blockRate    int
	dumpSubgraph string

	// TLS configuration
	tlsEnabled       bool
	tlsCert          string
	tlsKey           string
	tlsKeyPass       string
	tlsClientAuth    string
	tlsClientCACerts string
	tlsSystemCACerts bool
	tlsMinVersion    string
	tlsMaxVersion    string
)

func setupConfigOpts() {
	var config dgraph.Options
	defaults := dgraph.DefaultConfig
	flag.StringVar(&config.PostingDir, "p", defaults.PostingDir,
		"Directory to store posting lists.")
	flag.StringVar(&config.PostingTables, "posting_tables", defaults.PostingTables,
		"Specifies how Badger LSM tree is stored. Options are loadtoram, memorymap and "+
			"nothing; which consume most to least RAM while providing best to worst "+
			"performance respectively.")
	flag.StringVar(&config.WALDir, "w", defaults.WALDir,
		"Directory to store raft write-ahead logs.")
	flag.BoolVar(&config.Nomutations, "nomutations", defaults.Nomutations,
		"Don't allow mutations on this server.")

	flag.IntVar(&config.BaseWorkerPort, "workerport", defaults.BaseWorkerPort,
		"Port used by worker for internal communication.")
	flag.StringVar(&config.ExportPath, "export", defaults.ExportPath,
		"Folder in which to store exports.")
	flag.IntVar(&config.NumPendingProposals, "pending_proposals", defaults.NumPendingProposals,
		"Number of pending mutation proposals. Useful for rate limiting.")
	flag.Float64Var(&config.Tracing, "trace", defaults.Tracing,
		"The ratio of queries to trace.")
	flag.StringVar(&config.GroupIds, "groups", defaults.GroupIds,
		"RAFT groups handled by this server.")
	flag.StringVar(&config.MyAddr, "my", defaults.MyAddr,
		"addr:port of this server, so other Dgraph servers can talk to this.")
	flag.StringVar(&config.PeerAddr, "peer", defaults.PeerAddr,
		"IP_ADDRESS:PORT of any healthy peer.")
	flag.Uint64Var(&config.RaftId, "idx", defaults.RaftId,
		"RAFT ID that this server will use to join RAFT groups.")
	flag.Uint64Var(&config.MaxPendingCount, "sc", defaults.MaxPendingCount,
		"Max number of pending entries in wal after which snapshot is taken")
	flag.BoolVar(&config.ExpandEdge, "expand_edge", defaults.ExpandEdge,
		"Enables the expand() feature. This is very expensive for large data loads because it"+
			" doubles the number of mutations going on in the system.")

	flag.Float64Var(&config.AllottedMemory, "memory_mb", defaults.AllottedMemory,
		"Estimated memory the process can take. Actual usage would be slightly more than specified here.")
	flag.Float64Var(&config.CommitFraction, "gentlecommit", defaults.CommitFraction,
		"Fraction of dirty posting lists to commit every few seconds.")

	flag.StringVar(&config.ConfigFile, "config", defaults.ConfigFile,
		"YAML configuration file containing dgraph settings.")
	flag.BoolVar(&config.DebugMode, "debugmode", defaults.DebugMode,
		"enable debug mode for more debug information")

	flag.BoolVar(&x.Config.Version, "version", false, "Prints the version of Dgraph")
	// Useful for running multiple servers on the same machine.
	flag.IntVar(&x.Config.PortOffset, "port_offset", 0,
		"Value added to all listening port numbers.")

	flag.StringVar(&gconf, "group_conf", "", "group configuration file")
	flag.IntVar(&baseHttpPort, "port", 8080, "Port to run HTTP service on.")
	flag.IntVar(&baseGrpcPort, "grpc_port", 9080, "Port to run gRPC service on.")
	flag.BoolVar(&bindall, "bindall", false,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	flag.BoolVar(&exposeTrace, "expose_trace", false,
		"Allow trace endpoint to be accessible from remote")
	flag.StringVar(&cpuprofile, "cpu", "", "write cpu profile to file")
	flag.StringVar(&memprofile, "mem", "", "write memory profile to file")
	flag.IntVar(&blockRate, "block", 0, "Block profiling rate")
	flag.StringVar(&dumpSubgraph, "dumpsg", "", "Directory to save subgraph for testing, debugging")
	// TLS configurations
	flag.BoolVar(&tlsEnabled, "tls.on", false, "Use TLS connections with clients.")
	flag.StringVar(&tlsCert, "tls.cert", "", "Certificate file path.")
	flag.StringVar(&tlsKey, "tls.cert_key", "", "Certificate key file path.")
	flag.StringVar(&tlsKeyPass, "tls.cert_key_passphrase", "", "Certificate key passphrase.")
	flag.StringVar(&tlsClientAuth, "tls.client_auth", "", "Enable TLS client authentication")
	flag.StringVar(&tlsClientCACerts, "tls.ca_certs", "", "CA Certs file path.")
	flag.BoolVar(&tlsSystemCACerts, "tls.use_system_ca", false, "Include System CA into CA Certs.")
	flag.StringVar(&tlsMinVersion, "tls.min_version", "TLS11", "TLS min version.")
	flag.StringVar(&tlsMaxVersion, "tls.max_version", "TLS12", "TLS max version.")

	flag.Parse()
	if !flag.Parsed() {
		log.Fatal("Unable to parse flags")
	}

	// Read from config file before setting config.
	if config.ConfigFile != "" {
		x.Println("Loading configuration from file:", config.ConfigFile)
		x.LoadConfigFromYAML(config.ConfigFile)
	}
	// Lets check version flag before we SetConfiguration because we validate AllottedMemory in
	// SetConfiguration.
	x.PrintVersionOnly()

	dgraph.SetConfiguration(config)
}

func httpPort() int {
	return x.Config.PortOffset + baseHttpPort
}

func grpcPort() int {
	return x.Config.PortOffset + baseGrpcPort
}

func setupProfiling() {
	if len(cpuprofile) > 0 {
		f, err := os.Create(cpuprofile)
		x.Check(err)
		pprof.StartCPUProfile(f)
	}
	runtime.SetBlockProfileRate(blockRate)
}

func stopProfiling() {
	// Stop the CPU profiling that was initiated.
	if len(cpuprofile) > 0 {
		pprof.StopCPUProfile()
	}

	// Write memory profile before exit.
	if len(memprofile) > 0 {
		f, err := os.Create(memprofile)
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

func healthCheck(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if err := x.HealthCheck(); err == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	if err := x.HealthCheck(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		x.SetStatus(w, x.ErrorServiceUnavailable, err.Error())
		return
	}

	x.PendingQueries.Add(1)
	x.NumQueries.Add(1)
	defer x.PendingQueries.Add(-1)

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
	ctx = context.WithValue(ctx, "mutation_allowed", !dgraph.Config.Nomutations)

	if rand.Float64() < worker.Config.Tracing {
		tr := trace.New("Dgraph", "Query")
		tr.SetMaxEvents(1000)
		defer tr.Finish()
		ctx = trace.NewContext(ctx, tr)
	}

	invalidRequest := func(err error, msg string) {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while reading query: %+v", err)
		}
		x.SetStatus(w, x.ErrorInvalidRequest, msg)
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

	if dgraph.Config.DebugMode {
		fmt.Printf("Received query: %+v\n", q)
	}
	parseStart := time.Now()
	parsed, err := dgraph.ParseQueryAndMutation(ctx, gql.Request{
		Str:       q,
		Variables: map[string]string{},
		Http:      true,
	})
	l.Parsing += time.Since(parseStart)
	if err != nil {
		x.SetStatus(w, x.ErrorInvalidRequest, err.Error())
		return
	}

	var cancel context.CancelFunc
	// set timeout if schema mutation not present
	if parsed.Mutation == nil || len(parsed.Mutation.Schema) == 0 {
		// If schema mutation is not present
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}

	// After execution starts according to the GraphQL spec data key must be returned. It would be
	// null if any error is encountered, else non-null.
	var res query.ExecuteResult
	var queryRequest = query.QueryRequest{Latency: &l, GqlQuery: &parsed}
	if res, err = queryRequest.ProcessWithMutation(ctx); err != nil {
		switch errors.Cause(err).(type) {
		case *query.InvalidRequestError:
			x.SetStatusWithData(w, x.ErrorInvalidRequest, err.Error())
		default: // internalError or other
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while handling mutations: %+v", err)
			}
			x.SetStatusWithData(w, x.Error, err.Error())
		}
		return
	}

	var addLatency bool
	// If there is an error parsing, then addLatency would remain false.
	addLatency, _ = strconv.ParseBool(r.URL.Query().Get("latency"))
	debug, _ := strconv.ParseBool(r.URL.Query().Get("debug"))
	addLatency = addLatency || debug

	newUids := query.ConvertUidsToHex(res.Allocations)
	if len(parsed.Query) == 0 {
		schemaRes := map[string]interface{}{}
		mp := map[string]interface{}{}
		if parsed.Mutation != nil {
			mp["code"] = x.Success
			mp["message"] = "Done"
			mp["uids"] = newUids
		}
		// Either Schema or query can be specified
		if parsed.Schema != nil {
			if len(res.SchemaNode) == 0 {
				mp["schema"] = json.RawMessage("{}")
			} else {
				sort.Slice(res.SchemaNode, func(i, j int) bool {
					return res.SchemaNode[i].Predicate < res.SchemaNode[j].Predicate
				})
				js, err := json.Marshal(res.SchemaNode)
				if err != nil {
					x.SetStatusWithData(w, x.Error, "Unable to marshal schema")
					return
				}
				mp["schema"] = json.RawMessage(string(js))
			}
		}
		schemaRes["data"] = mp
		if addLatency {
			e := query.Extensions{
				Latency: l.ToMap(),
			}
			schemaRes["extensions"] = e
		}
		if js, err := json.Marshal(schemaRes); err == nil {
			w.Write(js)
		} else {
			x.SetStatusWithData(w, x.Error, "Unable to marshal schema")
		}
		return
	}

	if len(dumpSubgraph) > 0 {
		for _, sg := range res.Subgraphs {
			x.Checkf(os.MkdirAll(dumpSubgraph, 0700), dumpSubgraph)
			s := time.Now().Format("20060102.150405.000000.gob")
			filename := path.Join(dumpSubgraph, s)
			f, err := os.Create(filename)
			x.Checkf(err, filename)
			enc := gob.NewEncoder(f)
			x.Check(enc.Encode(sg))
			x.Checkf(f.Close(), filename)
		}
	}

	err = query.ToJson(&l, res.Subgraphs, w,
		query.ConvertUidsToHex(res.Allocations), addLatency)
	if err != nil {
		// since we performed w.Write in ToJson above,
		// calling WriteHeader with 500 code will be ignored.
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error while converting to JSON: %+v", err)
		}
		x.SetStatusWithData(w, x.Error, err.Error())
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
	qHash := fmt.Sprintf("%x", sha256.Sum256(query))
	return []*protos.NQuad{
		{Subject: "_:share", Predicate: "_share_", ObjectValue: val(string(query))},
		{Subject: "_:share", Predicate: "_share_hash_", ObjectValue: val(qHash)},
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
	newUids, err := query.AssignUids(ctx, nquads)
	if err != nil {
		fail()
		return
	}
	if mr, err = query.ToInternal(ctx, nquads, nil, newUids); err != nil {
		fail()
		return
	}
	var linearized bool = false // TODO: Uh, parse this out of the request somehow.
	if err = query.ApplyMutations(ctx, linearized, &protos.Mutations{Edges: mr.Edges}); err != nil {
		fail()
		return
	}
	tempMap := query.StripBlankNode(newUids)
	allocIdsStr := query.ConvertUidsToHex(tempMap)
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
	if r.Method != http.MethodGet {
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
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"code": "Success", "message": "Server is shutting down"}`))
}

func shutdownServer() {
	x.Printf("Got clean exit request")
	stopProfiling()                       // stop profiling
	dgraph.State.ShutdownCh <- struct{}{} // exit grpc and http servers.

	// wait for grpc and http servers to finish pending reqs and
	// then stop all nodes, internal grpc servers and sync all the marks
	go func() {
		defer func() { dgraph.State.ShutdownCh <- struct{}{} }()

		// wait for grpc, http and http2 servers to stop
		<-dgraph.State.FinishCh
		<-dgraph.State.FinishCh
		<-dgraph.State.FinishCh

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
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"code": "Success", "message": "Export completed."}`))
}

func memoryLimitHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		memoryLimitGetHandler(w, r)
	case http.MethodPut:
		memoryLimitPutHandler(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func memoryLimitPutHandler(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	memoryMB, err := strconv.ParseFloat(string(body), 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if memoryMB < dgraph.MinAllottedMemory {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "memory_mb must be at least %.0f\n", dgraph.MinAllottedMemory)
		return
	}

	posting.Config.Mu.Lock()
	posting.Config.AllottedMemory = memoryMB
	posting.Config.Mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func memoryLimitGetHandler(w http.ResponseWriter, r *http.Request) {
	posting.Config.Mu.Lock()
	memoryMB := posting.Config.AllottedMemory
	posting.Config.Mu.Unlock()

	if _, err := fmt.Fprintln(w, memoryMB); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func hasGraphOps(mu *protos.Mutation) bool {
	return len(mu.Set) > 0 || len(mu.Del) > 0 || len(mu.Schema) > 0
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
		uiDir = path.Join(gopath, "src/github.com/dgraph-io/dgraph/dashboard/build")
	}
}

func setupListener(addr string, port int) (listener net.Listener, err error) {
	var reload func()
	laddr := fmt.Sprintf("%s:%d", addr, port)
	if !tlsEnabled {
		listener, err = net.Listen("tcp", laddr)
	} else {
		var tlsCfg *tls.Config
		tlsCfg, reload, err = x.GenerateTLSConfig(x.TLSHelperConfig{
			ConfigType:   x.TLSServerConfig,
			CertRequired: tlsEnabled,
			Cert:         tlsCert,

			ClientAuth:             tlsClientAuth,
			ClientCACerts:          tlsClientCACerts,
			UseSystemClientCACerts: tlsSystemCACerts,
			MinVersion:             tlsMinVersion,
			MaxVersion:             tlsMaxVersion,
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
	defer func() { dgraph.State.FinishCh <- struct{}{} }()
	s := grpc.NewServer(grpc.CustomCodec(&query.Codec{}),
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000))
	protos.RegisterDgraphServer(s, &dgraph.Server{})
	err := s.Serve(l)
	log.Printf("gRpc server stopped : %s", err.Error())
	s.GracefulStop()
}

func serveHTTP(l net.Listener) {
	defer func() { dgraph.State.FinishCh <- struct{}{} }()
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
	go worker.RunServer(bindall) // For internal communication.

	laddr := "localhost"
	if bindall {
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
	http.HandleFunc("/admin/config/memory_mb", memoryLimitHandler)

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
		<-dgraph.State.ShutdownCh
		// Stops grpc/http servers; Already accepted connections are not closed.
		grpcListener.Close()
		httpListener.Close()
	}()

	log.Println("gRPC server started.  Listening on port", grpcPort())
	log.Println("HTTP server started.  Listening on port", httpPort())

	err = httpMux.Serve()     // Start cmux serving. blocking call
	<-dgraph.State.ShutdownCh // wait for shutdownServer to finish
	che <- err                // final close for main.
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)

	setupConfigOpts() // flag.Parse is called here.
	x.Init()

	setupProfiling()

	dgraph.State = dgraph.NewServerState()
	defer func() {
		x.Check(dgraph.State.Dispose())
	}()

	if exposeTrace {
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
	}

	x.Checkf(group.ParseGroupConfig(gconf), "While parsing group config.")

	// Posting will initialize index which requires schema. Hence, initialize
	// schema before calling posting.Init().
	schema.Init(dgraph.State.Pstore)
	posting.Init(dgraph.State.Pstore)
	worker.Config.InMemoryComm = false
	worker.Init(dgraph.State.Pstore)

	// setup shutdown os signal handler
	sdCh := make(chan os.Signal, 3)
	var numShutDownSig int
	defer close(sdCh)
	// sigint : Ctrl-C, sigterm : kill command.
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			select {
			case _, ok := <-sdCh:
				if !ok {
					return
				}
				numShutDownSig++
				x.Println("Caught Ctrl-C. Terminating now (this may take a few seconds)...")
				if numShutDownSig == 1 {
					shutdownServer()
				} else if numShutDownSig == 3 {
					x.Println("Signaled thrice. Aborting!")
					os.Exit(1)
				}
			}
		}
	}()
	_ = numShutDownSig

	// Setup external communication.
	che := make(chan error, 1)
	go setupServer(che)
	go worker.StartRaftNodes(dgraph.State.WALstore, bindall)

	if err := <-che; !strings.Contains(err.Error(),
		"use of closed network connection") {
		log.Fatal(err)
	}
}
