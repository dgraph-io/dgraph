/*
 * Copyright 2017-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package server

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

var Server x.SubCommand

func init() {
	Server.Cmd = &cobra.Command{
		Use:   "server",
		Short: "Run Dgraph data server",
		Long:  "Run Dgraph data server",
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Server.Conf).Stop()
			run()
		},
	}
	Server.EnvPrefix = "DGRAPH_SERVER"

	defaults := edgraph.DefaultConfig
	flag := Server.Cmd.Flags()
	flag.StringP("postings", "p", defaults.PostingDir,
		"Directory to store posting lists.")
	flag.String("posting_tables", defaults.PostingTables,
		"Specifies how Badger LSM tree is stored. Options are loadtoram, memorymap and "+
			"fileio; which consume most to least RAM while providing best to worst read"+
			"performance respectively.")
	flag.StringP("wal", "w", defaults.WALDir,
		"Directory to store raft write-ahead logs.")
	flag.Bool("nomutations", defaults.Nomutations,
		"Don't allow mutations on this server.")

	flag.String("whitelist", defaults.WhitelistedIPs,
		"A comma separated list of IP ranges you wish to whitelist for performing admin "+
			"actions (i.e., --whitelist 127.0.0.1:127.0.0.3,0.0.0.7:0.0.0.9)")
	flag.String("export", defaults.ExportPath,
		"Folder in which to store exports.")
	flag.Int("pending_proposals", defaults.NumPendingProposals,
		"Number of pending mutation proposals. Useful for rate limiting.")
	flag.Float64("trace", defaults.Tracing,
		"The ratio of queries to trace.")
	flag.String("my", defaults.MyAddr,
		"IP_ADDRESS:PORT of this server, so other Dgraph servers can talk to this.")
	flag.StringP("zero", "z", defaults.ZeroAddr,
		"IP_ADDRESS:PORT of Dgraph zero.")
	flag.Uint64("idx", 0,
		"Optional Raft ID that this server will use to join RAFT groups.")
	flag.Uint64("sc", defaults.MaxPendingCount,
		"Max number of pending entries in wal after which snapshot is taken")
	flag.Bool("expand_edge", defaults.ExpandEdge,
		"Enables the expand() feature. This is very expensive for large data loads because it"+
			" doubles the number of mutations going on in the system.")

	flag.Float64("lru_mb", defaults.AllottedMemory,
		"Estimated memory the LRU cache can take. "+
			"Actual usage by the process would be more than specified here.")

	flag.Bool("debugmode", defaults.DebugMode,
		"enable debug mode for more debug information")

	// Useful for running multiple servers on the same machine.
	flag.IntP("port_offset", "o", 0,
		"Value added to all listening port numbers. [Internal=7080, HTTP=8080, Grpc=9080]")

	flag.Bool("bindall", true,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	flag.Bool("expose_trace", false,
		"Allow trace endpoint to be accessible from remote")

	flag.Uint64("query_edge_limit", 1e6,
		"Limit for the maximum number of edges that can be returned in a query."+
			" This is only useful for shortest path queries.")

	tlsConf.ConfigType = x.TLSServerConfig

	//Custom plugins.
	flag.String("custom_tokenizers", "",
		"Comma separated list of tokenizer plugins")

	// By default Go GRPC traces all requests.
	grpc.EnableTracing = false
}

func setupCustomTokenizers() {
	customTokenizers := Server.Conf.GetString("custom_tokenizers")
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

// really not lambda
func setupListener(addr string, port int) (listener net.Listener, reload func([]byte), err error) {
	var r func([]byte)
	laddr := fmt.Sprintf("%s:%d", addr, port)
	if !tlsConf.CertRequired {
		listener, err = net.Listen("tcp", laddr)
	} else {
		var tlsCfg *tls.Config
        // this just closes over nothing, seems wrong  
		tlsCfg, r, err = x.GenerateTLSConfigServer(tlsConf)
		if err != nil {
			return nil, nil, err
		}
		listener, err = tls.Listen("tcp", laddr, tlsCfg)
	}
	return listener, r, err
}

func serveGRPC(l net.Listener, wg *sync.WaitGroup) {
	defer wg.Done()
	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000))
	api.RegisterDgraphServer(s, &edgraph.Server{})
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

func setupServer(kchan chan []byte) {
	go worker.RunServer(bindall) // For intern.communication.

	laddr := "localhost"
	if bindall {
		laddr = "0.0.0.0"
	}

	httpListener, reloadKeysHttp, err := setupListener(laddr, httpPort())
	if err != nil {
		log.Fatal(err)
	}

	grpcListener, reloadKeysGRPC, err := setupListener(laddr, grpcPort())
	if err != nil {
		log.Fatal(err)
	}

    go func() {
        for {
            kinfo := <- kchan
            if reloadKeysHttp != nil {
                reloadKeysHttp(kinfo)
            }
            if reloadKeysGRPC != nil {
                reloadKeysGRPC(kinfo)
            }
        }
    } ()
    
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

	// Initialize the servers.
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

func run() {
	config := edgraph.Options{
		PostingDir:          Server.Conf.GetString("postings"),
		PostingTables:       Server.Conf.GetString("posting_tables"),
		WALDir:              Server.Conf.GetString("wal"),
		Nomutations:         Server.Conf.GetBool("nomutations"),
		WhitelistedIPs:      Server.Conf.GetString("whitelist"),
		AllottedMemory:      Server.Conf.GetFloat64("lru_mb"),
		ExportPath:          Server.Conf.GetString("export"),
		NumPendingProposals: Server.Conf.GetInt("pending_proposals"),
		Tracing:             Server.Conf.GetFloat64("trace"),
		MyAddr:              Server.Conf.GetString("my"),
		ZeroAddr:            Server.Conf.GetString("zero"),
		RaftId:              uint64(Server.Conf.GetInt("idx")),
		MaxPendingCount:     uint64(Server.Conf.GetInt("sc")),
		ExpandEdge:          Server.Conf.GetBool("expand_edge"),
		DebugMode:           Server.Conf.GetBool("debugmode"),
	}

	x.Config.PortOffset = Server.Conf.GetInt("port_offset")
	bindall = Server.Conf.GetBool("bindall")
    // configure 
    tlsConf.CertRequired = true;
    
	edgraph.SetConfiguration(config)
	setupCustomTokenizers()
	x.Init(edgraph.Config.DebugMode)
	x.Config.QueryEdgeLimit = cast.ToUint64(Server.Conf.GetString("query_edge_limit"))

	edgraph.InitServerState()
	defer func() {
		x.Check(edgraph.State.Dispose())
	}()

	if Server.Conf.GetBool("expose_trace") {
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
	sdCh = make(chan os.Signal, 3)
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

    keyupdate := make(chan []byte);

	// Setup external communication.
	go worker.StartRaftNodes(edgraph.State.WALstore, bindall, keyupdate)
    
    setupServer(keyupdate)
    // setup server doesn't actually return here
	worker.BlockingStop()
}
