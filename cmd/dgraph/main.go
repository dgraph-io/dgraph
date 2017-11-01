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
	"crypto/tls"
	"flag"
	"fmt"
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
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/dgraph"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

var (
	baseHttpPort int
	baseGrpcPort int
	bindall      bool

	exposeTrace bool
	cpuprofile  string
	memprofile  string
	blockRate   int

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

	customTokenizers string
)

func setupConfigOpts() {
	var config dgraph.Options
	defaults := dgraph.DefaultConfig
	flag.StringVar(&config.PostingDir, "p", defaults.PostingDir,
		"Directory to store posting lists.")
	flag.StringVar(&config.PostingTables, "posting_tables", defaults.PostingTables,
		"Specifies how Badger LSM tree is stored. Options are loadtoram, memorymap and "+
			"fileio; which consume most to least RAM while providing best to worst "+
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
	flag.StringVar(&config.MyAddr, "my", defaults.MyAddr,
		"IP_ADDRESS:PORT of this server, so other Dgraph servers can talk to this.")
	flag.StringVar(&config.ZeroAddr, "zero", defaults.ZeroAddr,
		"IP_ADDRESS:PORT of Dgraph zero.")
	flag.Uint64Var(&config.RaftId, "idx", 0,
		"Optional Raft ID that this server will use to join RAFT groups.")
	flag.Uint64Var(&config.MaxPendingCount, "sc", defaults.MaxPendingCount,
		"Max number of pending entries in wal after which snapshot is taken")
	flag.BoolVar(&config.ExpandEdge, "expand_edge", defaults.ExpandEdge,
		"Enables the expand() feature. This is very expensive for large data loads because it"+
			" doubles the number of mutations going on in the system.")

	flag.Float64Var(&config.AllottedMemory, "memory_mb", defaults.AllottedMemory,
		"Estimated memory the process can take. "+
			"Actual usage would be slightly more than specified here.")

	flag.StringVar(&config.ConfigFile, "config", defaults.ConfigFile,
		"YAML configuration file containing dgraph settings.")
	flag.BoolVar(&config.DebugMode, "debugmode", defaults.DebugMode,
		"enable debug mode for more debug information")

	flag.BoolVar(&x.Config.Version, "version", false, "Prints the version of Dgraph")
	// Useful for running multiple servers on the same machine.
	flag.IntVar(&x.Config.PortOffset, "port_offset", 0,
		"Value added to all listening port numbers.")

	flag.IntVar(&baseHttpPort, "port", 8080, "Port to run HTTP service on.")
	flag.IntVar(&baseGrpcPort, "grpc_port", 9080, "Port to run gRPC service on.")
	flag.BoolVar(&bindall, "bindall", false,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	flag.BoolVar(&exposeTrace, "expose_trace", false,
		"Allow trace endpoint to be accessible from remote")
	flag.StringVar(&cpuprofile, "cpu", "", "write cpu profile to file")
	flag.StringVar(&memprofile, "mem", "", "write memory profile to file")
	flag.IntVar(&blockRate, "block", 0, "Block profiling rate")
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
	//Custom plugins.
	flag.StringVar(&customTokenizers, "custom_tokenizers", "",
		"Comma separated list of tokenizer plugins")

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
	if x.Config.Version {
		x.PrintVersionOnly()
	}

	dgraph.SetConfiguration(config)
	setupCustomTokenizers()
}

func setupCustomTokenizers() {
	if customTokenizers == "" {
		return
	}
	for _, soFile := range strings.Split(customTokenizers, ",") {
		tok.LoadCustomTokenizer(soFile)
	}
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

func healthCheck(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	if err := x.HealthCheck(); err == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

// storeStatsHandler outputs some basic stats for data store.
func storeStatsHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<pre>"))
	w.Write([]byte(worker.StoreStats()))
	w.Write([]byte("</pre>"))
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

func serveGRPC(l net.Listener, wg *sync.WaitGroup) {
	defer wg.Done()
	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000))
	protos.RegisterDgraphServer(s, &dgraph.Server{})
	err := s.Serve(l)
	log.Printf("gRpc server stopped : %s", err.Error())
	s.GracefulStop()
}

func serveHTTP(l net.Listener, wg *sync.WaitGroup) {
	defer wg.Done()
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

func setupServer() {
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

	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/mutate", mutationHandler)
	http.HandleFunc("/alter", alterHandler)
	http.HandleFunc("/health", healthCheck)
	// http.HandleFunc("/share", shareHandler)
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
	var wg sync.WaitGroup
	wg.Add(3)
	go serveGRPC(grpcListener, &wg)
	go serveHTTP(httpListener, &wg)

	go func() {
		defer wg.Done()
		<-sdCh
		// Stops grpc/http servers; Already accepted connections are not closed.
		grpcListener.Close()
		httpListener.Close()
	}()

	log.Println("gRPC server started.  Listening on port", grpcPort())
	log.Println("HTTP server started.  Listening on port", httpPort())
	wg.Wait()
}

var sdCh chan os.Signal

func main() {
	rand.Seed(time.Now().UnixNano())

	// Setting a higher number here allows more disk I/O calls to be scheduled, hence considerably
	// improving throughput. The extra CPU overhead is almost negligible in comparison. The
	// benchmark notes are located in badger-bench/randread.
	runtime.GOMAXPROCS(128)

	setupConfigOpts() // flag.Parse is called here.
	x.Init(dgraph.Config.DebugMode)

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

	// Posting will initialize index which requires schema. Hence, initialize
	// schema before calling posting.Init().
	schema.Init(dgraph.State.Pstore)
	posting.Init(dgraph.State.Pstore)
	worker.Config.InMemoryComm = false
	worker.Init(dgraph.State.Pstore)

	// setup shutdown os signal handler
	sdCh = make(chan os.Signal, 3)
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
	go worker.StartRaftNodes(dgraph.State.WALstore, bindall)
	setupServer()
	worker.BlockingStop()
}
