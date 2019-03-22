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
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof" // http profiler
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dgraph-io/badger/y"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"go.opencensus.io/exporter/jaeger"
	"go.opencensus.io/plugin/ocgrpc"
	otrace "go.opencensus.io/trace"
	"go.opencensus.io/zpages"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // grpc compression
	"google.golang.org/grpc/health"
	hapi "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	tlsNodeCert = "node.crt"
	tlsNodeKey  = "node.key"
)

var (
	bindall bool
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

	// If you change any of the flags below, you must also update run() to call Alpha.Conf.Get
	// with the flag name so that the values are picked up by Cobra/Viper's various config inputs
	// (e.g, config file, env vars, cli flags, etc.)
	flag := Alpha.Cmd.Flags()
	flag.Bool("enterprise_features", false, "Enable Dgraph enterprise features. "+
		"If you set this to true, you agree to the Dgraph Community License.")
	flag.StringP("postings", "p", "p", "Directory to store posting lists.")

	// Options around how to set up Badger.
	flag.String("badger.tables", "mmap",
		"[ram, mmap, disk] Specifies how Badger LSM tree is stored. "+
			"Option sequence consume most to least RAM while providing best to worst read "+
			"performance respectively.")
	flag.String("badger.vlog", "mmap",
		"[mmap, disk] Specifies how Badger Value log is stored."+
			" mmap consumes more RAM, but provides better performance.")

	// OpenCensus flags.
	flag.Float64("trace", 1.0, "The ratio of queries to trace.")
	flag.String("jaeger.collector", "", "Send opencensus traces to Jaeger.")

	flag.StringP("wal", "w", "w", "Directory to store raft write-ahead logs.")
	flag.String("whitelist", "",
		"A comma separated list of IP ranges you wish to whitelist for performing admin "+
			"actions (i.e., --whitelist 127.0.0.1:127.0.0.3,0.0.0.7:0.0.0.9)")
	flag.String("export", "export", "Folder in which to store exports.")
	flag.Int("pending_proposals", 256,
		"Number of pending mutation proposals. Useful for rate limiting.")
	flag.String("my", "",
		"IP_ADDRESS:PORT of this Dgraph Alpha, so other Dgraph Alphas can talk to this.")
	flag.StringP("zero", "z", fmt.Sprintf("localhost:%d", x.PortZeroGrpc),
		"IP_ADDRESS:PORT of a Dgraph Zero.")
	flag.Uint64("idx", 0,
		"Optional Raft ID that this Dgraph Alpha will use to join RAFT groups.")
	flag.Bool("expand_edge", true,
		"Enables the expand() feature. This is very expensive for large data loads because it"+
			" doubles the number of mutations going on in the system.")
	flag.Int("max_retries", -1,
		"Commits to disk will give up after these number of retries to prevent locking the worker"+
			" in a failed state. Use -1 to retry infinitely.")
	flag.String("auth_token", "",
		"If set, all Alter requests to Dgraph would need to have this token."+
			" The token can be passed as follows: For HTTP requests, in X-Dgraph-AuthToken header."+
			" For Grpc, in auth-token key in the context.")

	flag.String("acl_secret_file", "", "The file that stores the HMAC secret, "+
		"which is used for signing the JWT and should have at least 32 ASCII characters. "+
		"Enterprise feature.")
	flag.Duration("acl_access_ttl", 6*time.Hour, "The TTL for the access jwt. "+
		"Enterprise feature.")
	flag.Duration("acl_refresh_ttl", 30*24*time.Hour, "The TTL for the refresh jwt. "+
		"Enterprise feature.")
	flag.Duration("acl_cache_ttl", 30*time.Second, "The interval to refresh the acl cache. "+
		"Enterprise feature.")
	flag.Float64P("lru_mb", "l", -1,
		"Estimated memory the LRU cache can take. "+
			"Actual usage by the process would be more than specified here.")
	flag.Bool("debugmode", false,
		"Enable debug mode for more debug information.")
	flag.String("mutations", "allow",
		"Set mutation mode to allow, disallow, or strict.")

	// Useful for running multiple servers on the same machine.
	flag.IntP("port_offset", "o", 0,
		"Value added to all listening port numbers. [Internal=7080, HTTP=8080, Grpc=9080]")

	flag.Uint64("query_edge_limit", 1e6,
		"Limit for the maximum number of edges that can be returned in a query."+
			" This applies to shortest path and recursive queries.")

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

// Parses a comma-delimited list of IP addresses, IP ranges, CIDR blocks, or hostnames
// and returns a slice of []IPRange.
//
// e.g. "144.142.126.222:144.142.126.244,144.142.126.254,192.168.0.0/16,host.docker.internal"
func getIPsFromString(str string) ([]x.IPRange, error) {
	if str == "" {
		return []x.IPRange{}, nil
	}

	var ipRanges []x.IPRange
	rangeStrings := strings.Split(str, ",")

	for _, s := range rangeStrings {
		isIPv6 := strings.Index(s, "::") >= 0
		tuple := strings.Split(s, ":")
		switch {
		case isIPv6 || len(tuple) == 1:
			if strings.Index(s, "/") < 0 {
				// string is hostname like host.docker.internal,
				// or IPv4 address like 144.124.126.254,
				// or IPv6 address like fd03:b188:0f3c:9ec4::babe:face
				ipAddr := net.ParseIP(s)
				if ipAddr != nil {
					ipRanges = append(ipRanges, x.IPRange{Lower: ipAddr, Upper: ipAddr})
				} else {
					ipAddrs, err := net.LookupIP(s)
					if err != nil {
						return nil, fmt.Errorf("invalid IP address or hostname: %s", s)
					}

					for _, addr := range ipAddrs {
						ipRanges = append(ipRanges, x.IPRange{Lower: addr, Upper: addr})
					}
				}
			} else {
				// string is CIDR block like 192.168.0.0/16 or fd03:b188:0f3c:9ec4::/64
				rangeLo, network, err := net.ParseCIDR(s)
				if err != nil {
					return nil, fmt.Errorf("invalid CIDR block: %s", s)
				}

				addrLen, maskLen := len(rangeLo), len(network.Mask)
				rangeHi := make(net.IP, len(rangeLo))
				copy(rangeHi, rangeLo)
				for i := 1; i <= maskLen; i++ {
					rangeHi[addrLen-i] |= ^network.Mask[maskLen-i]
				}

				ipRanges = append(ipRanges, x.IPRange{Lower: rangeLo, Upper: rangeHi})
			}
		case len(tuple) == 2:
			// string is range like a.b.c.d:w.x.y.z
			rangeLo := net.ParseIP(tuple[0])
			rangeHi := net.ParseIP(tuple[1])
			if rangeLo == nil {
				return nil, fmt.Errorf("invalid IP address: %s", tuple[0])
			} else if rangeHi == nil {
				return nil, fmt.Errorf("invalid IP address: %s", tuple[1])
			} else if bytes.Compare(rangeLo, rangeHi) > 0 {
				return nil, fmt.Errorf("inverted IP address range: %s", s)
			}
			ipRanges = append(ipRanges, x.IPRange{Lower: rangeLo, Upper: rangeHi})
		default:
			return nil, fmt.Errorf("invalid IP address range: %s", s)
		}
	}

	return ipRanges, nil
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
				glog.Infoln("SIGHUP signal received")
				reload()
				glog.Infoln("TLS certificates and CAs reloaded")
			}
		}()
	}
	return net.Listen("tcp", fmt.Sprintf("%s:%d", addr, port))
}

func serveGRPC(l net.Listener, tlsCfg *tls.Config, wg *sync.WaitGroup) {
	defer wg.Done()

	if collector := Alpha.Conf.GetString("jaeger.collector"); len(collector) > 0 {
		// Port details: https://www.jaegertracing.io/docs/getting-started/
		// Default collectorEndpointURI := "http://localhost:14268"
		je, err := jaeger.NewExporter(jaeger.Options{
			Endpoint:    collector,
			ServiceName: "dgraph.alpha",
		})
		if err != nil {
			log.Fatalf("Failed to create the Jaeger exporter: %v", err)
		}
		// And now finally register it as a Trace Exporter
		otrace.RegisterExporter(je)
	}
	// Exclusively for stats, metrics, etc. Not for tracing.
	// var views = append(ocgrpc.DefaultServerViews, ocgrpc.DefaultClientViews...)
	// if err := view.Register(views...); err != nil {
	// 	glog.Fatalf("Unable to register OpenCensus stats: %v", err)
	// }

	opt := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000),
		grpc.StatsHandler(&ocgrpc.ServerHandler{}),
	}
	if tlsCfg != nil {
		opt = append(opt, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	s := grpc.NewServer(opt...)
	api.RegisterDgraphServer(s, &edgraph.Server{})
	hapi.RegisterHealthServer(s, health.NewServer())
	err := s.Serve(l)
	glog.Errorf("GRPC listener canceled: %v\n", err)
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
	glog.Errorf("Stopped taking more http(s) requests. Err: %v", err)
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
			log.Fatalf("Failed to setup TLS: %v\n", err)
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

	// TODO: Figure out what this is for?
	http.HandleFunc("/debug/store", storeStatsHandler)

	http.HandleFunc("/admin/shutdown", shutDownHandler)
	http.HandleFunc("/admin/export", exportHandler)
	http.HandleFunc("/admin/config/lru_mb", memoryLimitHandler)

	// Add OpenCensus z-pages.
	zpages.Handle(http.DefaultServeMux, "/z")

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

	glog.Infoln("gRPC server started.  Listening on port", grpcPort())
	glog.Infoln("HTTP server started.  Listening on port", httpPort())
	wg.Wait()
}

var shutdownCh chan struct{}

func run() {
	bindall = Alpha.Conf.GetBool("bindall")

	opts := edgraph.Options{
		BadgerTables: Alpha.Conf.GetString("badger.tables"),
		BadgerVlog:   Alpha.Conf.GetString("badger.vlog"),

		PostingDir: Alpha.Conf.GetString("postings"),
		WALDir:     Alpha.Conf.GetString("wal"),

		MutationsMode:  edgraph.AllowMutations,
		AuthToken:      Alpha.Conf.GetString("auth_token"),
		AllottedMemory: Alpha.Conf.GetFloat64("lru_mb"),
	}

	secretFile := Alpha.Conf.GetString("acl_secret_file")
	if secretFile != "" {
		if !Alpha.Conf.GetBool("enterprise_features") {
			glog.Fatalf("You must enable Dgraph enterprise features with the " +
				"--enterprise_features option in order to use ACL.")
		}

		hmacSecret, err := ioutil.ReadFile(secretFile)
		if err != nil {
			glog.Fatalf("Unable to read HMAC secret from file: %v", secretFile)
		}
		if len(hmacSecret) < 32 {
			glog.Fatalf("The HMAC secret file should contain at least 256 bits (32 ascii chars)")
		}

		opts.HmacSecret = hmacSecret
		opts.AccessJwtTtl = Alpha.Conf.GetDuration("acl_access_ttl")
		opts.RefreshJwtTtl = Alpha.Conf.GetDuration("acl_refresh_ttl")
		opts.AclRefreshInterval = Alpha.Conf.GetDuration("acl_cache_ttl")

		glog.Info("HMAC secret loaded successfully.")
	}

	switch strings.ToLower(Alpha.Conf.GetString("mutations")) {
	case "allow":
		opts.MutationsMode = edgraph.AllowMutations
	case "disallow":
		opts.MutationsMode = edgraph.DisallowMutations
	case "strict":
		opts.MutationsMode = edgraph.StrictMutations
	default:
		glog.Error("--mutations argument must be one of allow, disallow, or strict")
		os.Exit(1)
	}

	edgraph.SetConfiguration(opts)

	ips, err := getIPsFromString(Alpha.Conf.GetString("whitelist"))
	x.Check(err)
	x.WorkerConfig = x.WorkerOptions{
		ExportPath:          Alpha.Conf.GetString("export"),
		NumPendingProposals: Alpha.Conf.GetInt("pending_proposals"),
		Tracing:             Alpha.Conf.GetFloat64("trace"),
		MyAddr:              Alpha.Conf.GetString("my"),
		ZeroAddr:            Alpha.Conf.GetString("zero"),
		RaftId:              cast.ToUint64(Alpha.Conf.GetString("idx")),
		ExpandEdge:          Alpha.Conf.GetBool("expand_edge"),
		WhiteListedIPRanges: ips,
		MaxRetries:          Alpha.Conf.GetInt("max_retries"),
		StrictMutations:     opts.MutationsMode == edgraph.StrictMutations,
		AclEnabled:          secretFile != "",
	}

	x.LoadTLSConfig(&tlsConf, Alpha.Conf, tlsNodeCert, tlsNodeKey)
	tlsConf.ClientAuth = Alpha.Conf.GetString("tls_client_auth")

	setupCustomTokenizers()
	x.Init()
	x.Config.DebugMode = Alpha.Conf.GetBool("debugmode")
	x.Config.PortOffset = Alpha.Conf.GetInt("port_offset")
	x.Config.QueryEdgeLimit = cast.ToUint64(Alpha.Conf.GetString("query_edge_limit"))

	x.PrintVersion()
	edgraph.InitServerState()
	defer func() {
		edgraph.State.Dispose()
		glog.Info("Finished disposing server state.")
	}()

	if Alpha.Conf.GetBool("expose_trace") {
		// TODO: Remove this once we get rid of event logs.
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
	}
	otrace.ApplyConfig(otrace.Config{
		DefaultSampler:             otrace.ProbabilitySampler(x.WorkerConfig.Tracing),
		MaxAnnotationEventsPerSpan: 64,
	})

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
				glog.Infoln("Caught Ctrl-C. Terminating now (this may take a few seconds)...")
				if numShutDownSig == 3 {
					glog.Infoln("Signaled thrice. Aborting!")
					os.Exit(1)
				}
			}
		}
	}()
	_ = numShutDownSig

	// Setup external communication.
	aclCloser := y.NewCloser(1)
	go func() {
		worker.StartRaftNodes(edgraph.State.WALstore, bindall)
		// initialization of the admin account can only be done after raft nodes are running
		// and health check passes
		edgraph.ResetAcl()
		edgraph.RefreshAcls(aclCloser)
	}()

	setupServer()
	glog.Infoln("GRPC and HTTP stopped.")
	aclCloser.SignalAndWait()
	worker.BlockingStop()
	glog.Infoln("Server shutdown. Bye!")
}
