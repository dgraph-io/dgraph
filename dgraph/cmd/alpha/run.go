/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package alpha

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
)

var (
	bindall bool
	config  edgraph.Options
	tlsConf x.TLSHelperConfig
)

var Alpha x.SubCommand

func init() {
	Alpha.Cmd = &cobra.Command{
		Use:   "alpha",
		Short: "Run Dgraph Alpha",
		Long: `
A Dgraph Alpha instance stores the data. Each Dgraph Alpha is responsible for
storing and serving one data group. If multiple Alphas serve the same group,
they form a Raft group and provide synchronous replication.
`,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Alpha.Conf).Stop()
			run()
		},
	}
	Alpha.EnvPrefix = "DGRAPH_ALPHA"

	flag := Alpha.Cmd.Flags()
	flag.StringVarP(&config.PostingDir, "postings", "p", "p",
		"Directory to store posting lists.")

	// Options around how to set up Badger.
	flag.StringVar(&config.BadgerTables, "badger.tables", "mmap",
		"[ram, mmap, disk] Specifies how Badger LSM tree is stored. "+
			"Option sequence consume most to least RAM while providing best to worst read "+
			"performance respectively.")
	flag.StringVar(&config.BadgerVlog, "badger.vlog", "mmap",
		"[mmap, disk] Specifies how Badger Value log is stored."+
			" mmap consumes more RAM, but provides better performance.")

	flag.StringVarP(&config.WALDir, "wal", "w", "w", "Directory to store raft write-ahead logs.")
	flag.BoolVar(&config.Nomutations, "nomutations", false, "Don't allow mutations on this server.")

	flag.StringVar(&config.WhitelistedIPs, "whitelist", "",
		"A comma separated list of IP ranges you wish to whitelist for performing admin "+
			"actions (i.e., --whitelist 127.0.0.1:127.0.0.3,0.0.0.7:0.0.0.9)")

	flag.StringVar(&worker.Config.ExportPath, "export", "export", "Folder in which to store exports.")
	flag.IntVar(&worker.Config.NumPendingProposals, "pending_proposals", 2000,
		"Number of pending mutation proposals. Useful for rate limiting.")
	flag.Float64Var(&worker.Config.Tracing, "trace", 0.0, "The ratio of queries to trace.")
	flag.StringVar(&worker.Config.MyAddr, "my", "",
		"IP_ADDRESS:PORT of this server, so other Dgraph servers can talk to this.")
	flag.StringVarP(&worker.Config.ZeroAddr, "zero", "z", fmt.Sprintf("localhost:%d", x.PortZeroGrpc),
		"IP_ADDRESS:PORT of Dgraph zero.")
	flag.Uint64Var(&worker.Config.RaftId, "idx", 0,
		"Optional Raft ID that this server will use to join RAFT groups.")
	flag.BoolVar(&worker.Config.ExpandEdge, "expand_edge", true,
		"Enables the expand() feature. This is very expensive for large data loads because it"+
			" doubles the number of mutations going on in the system.")

	flag.Float64VarP(&config.AllottedMemory, "lru_mb", "l", -1,
		"Estimated memory the LRU cache can take. "+
			"Actual usage by the process would be more than specified here.")

	flag.BoolVar(&x.Config.DebugMode, "debugmode", false,
		"Enable debug mode for more debug information.")

	// Useful for running multiple servers on the same machine.
	flag.IntVarP(&x.Config.PortOffset, "port_offset", "o", 0,
		"Value added to all listening port numbers. [Internal=7080, HTTP=8080, Grpc=9080]")

	flag.Uint64("query_edge_limit", 1e6,
		"Limit for the maximum number of edges that can be returned in a query."+
			" This is only useful for shortest path queries.")

	// TLS configurations
	x.RegisterTLSFlags(flag)
	flag.String("tls_client_auth", "VERIFYIFGIVEN", "Enable TLS client authentication")
	tlsConf.ConfigType = x.TLSServerConfig

	//Custom plugins.
	flag.String("custom_tokenizers", "",
		"Comma separated list of tokenizer plugins")

	// By default Go GRPC traces all requests.
	grpc.EnableTracing = false
}

func setupCustomTokenizers() {
	customTokenizers := Alpha.Conf.GetString("custom_tokenizers")
	if customTokenizers == "" {
		return
	}
	for _, soFile := range strings.Split(customTokenizers, ",") {
		tok.LoadCustomTokenizer(soFile)
	}
}

func httpPort() int {
	return x.Config.PortOffset + x.PortHTTP
}

func grpcPort() int {
	return x.Config.PortOffset + x.PortGrpc
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

func setupListener(addr string, port int, reload func()) (net.Listener, error) {
	if reload != nil {
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGHUP)
			for range sigChan {
				log.Println("SIGHUP signal received")
				reload()
				log.Println("TLS certificates and CAs reloaded")
			}
		}()
	}
	return net.Listen("tcp", fmt.Sprintf("%s:%d", addr, port))
}

func serveGRPC(l net.Listener, tlsCfg *tls.Config, wg *sync.WaitGroup) {
	defer wg.Done()
	opt := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000),
	}
	if tlsCfg != nil {
		opt = append(opt, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}
	s := grpc.NewServer(opt...)
	api.RegisterDgraphServer(s, &edgraph.Server{})
	err := s.Serve(l)
	log.Printf("GRPC listener canceled: %s\n", err.Error())
	s.Stop()
}

func serveHTTP(l net.Listener, tlsCfg *tls.Config, wg *sync.WaitGroup) {
	defer wg.Done()
	srv := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 600 * time.Second,
		IdleTimeout:  2 * time.Minute,
	}
	var err error
	switch {
	case tlsCfg != nil:
		srv.TLSConfig = tlsCfg
		err = srv.ServeTLS(l, "", "")
	default:
		err = srv.Serve(l)
	}
	log.Printf("Stopped taking more http(s) requests. Err: %s", err.Error())
	ctx, cancel := context.WithTimeout(context.Background(), 630*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Http(s) shutdown err: %v", err.Error())
	}
}

func setupServer() {
	go worker.RunServer(bindall) // For pb.communication.

	laddr := "localhost"
	if bindall {
		laddr = "0.0.0.0"
	}

	var (
		tlsCfg *tls.Config
		reload func()
	)
	if tlsConf.CertRequired {
		var err error
		tlsCfg, reload, err = x.GenerateTLSConfig(tlsConf)
		if err != nil {
			log.Fatalf("Failed to setup TLS: %s\n", err)
		}
	}

	httpListener, err := setupListener(laddr, httpPort(), reload)
	if err != nil {
		log.Fatal(err)
	}

	grpcListener, err := setupListener(laddr, grpcPort(), nil)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/query", queryHandler)
	http.HandleFunc("/query/", queryHandler)
	http.HandleFunc("/mutate", mutationHandler)
	http.HandleFunc("/mutate/", mutationHandler)
	http.HandleFunc("/commit/", commitHandler)
	http.HandleFunc("/abort/", abortHandler)
	http.HandleFunc("/alter", alterHandler)
	http.HandleFunc("/health", healthCheck)
	http.HandleFunc("/share", shareHandler)
	http.HandleFunc("/debug/store", storeStatsHandler)
	http.HandleFunc("/admin/shutdown", shutDownHandler)
	http.HandleFunc("/admin/export", exportHandler)
	http.HandleFunc("/admin/config/lru_mb", memoryLimitHandler)

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/ui/keywords", keywordHandler)

	// Initilize the servers.
	var wg sync.WaitGroup
	wg.Add(3)
	go serveGRPC(grpcListener, tlsCfg, &wg)
	go serveHTTP(httpListener, tlsCfg, &wg)

	go func() {
		defer wg.Done()
		<-shutdownCh
		// Stops grpc/http servers; Already accepted connections are not closed.
		grpcListener.Close()
		httpListener.Close()
	}()

	log.Println("gRPC server started.  Listening on port", grpcPort())
	log.Println("HTTP server started.  Listening on port", httpPort())
	wg.Wait()
}

var shutdownCh chan struct{}

func run() {
	bindall = Alpha.Conf.GetBool("bindall")
	x.LoadTLSConfig(&tlsConf, Alpha.Conf)
	tlsConf.ClientAuth = Alpha.Conf.GetString("tls_client_auth")

	edgraph.SetConfiguration(config)
	setupCustomTokenizers()
	x.Init()
	x.Config.QueryEdgeLimit = cast.ToUint64(Alpha.Conf.GetString("query_edge_limit"))

	x.PrintVersion()
	edgraph.InitServerState()
	defer func() {
		x.Check(edgraph.State.Dispose())
	}()

	if Alpha.Conf.GetBool("expose_trace") {
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
	}

	// Posting will initialize index which requires schema. Hence, initialize
	// schema before calling posting.Init().
	schema.Init(edgraph.State.Pstore)
	posting.Init(edgraph.State.Pstore)
	defer posting.Cleanup()
	worker.Init(edgraph.State.Pstore)

	// setup shutdown os signal handler
	sdCh := make(chan os.Signal, 3)
	shutdownCh = make(chan struct{})

	var numShutDownSig int
	defer func() {
		signal.Stop(sdCh)
		close(sdCh)
	}()
	// sigint : Ctrl-C, sigterm : kill command.
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			select {
			case _, ok := <-sdCh:
				if !ok {
					return
				}
				select {
				case <-shutdownCh:
				default:
					close(shutdownCh)
				}
				numShutDownSig++
				x.Println("Caught Ctrl-C. Terminating now (this may take a few seconds)...")
				if numShutDownSig == 3 {
					x.Println("Signaled thrice. Aborting!")
					os.Exit(1)
				}
			}
		}
	}()
	_ = numShutDownSig

	// Setup external communication.
	go worker.StartRaftNodes(edgraph.State.WALstore, bindall)
	setupServer()
	log.Println("GRPC and HTTP stopped.")
	worker.BlockingStop()
	log.Println("Server shutdown. Bye!")
}
