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
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net"
	"net/http"
	_ "net/http/pprof" // http profiler
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	badgerpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/graphql/admin"
	"github.com/dgraph-io/dgraph/graphql/web"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"go.opencensus.io/plugin/ocgrpc"
	otrace "go.opencensus.io/trace"
	"go.opencensus.io/zpages"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	_ "google.golang.org/grpc/encoding/gzip" // grpc compression
	"google.golang.org/grpc/health"
	hapi "google.golang.org/grpc/health/grpc_health_v1"

	_ "github.com/vektah/gqlparser/v2/validator/rules" // make gql validator init() all rules
)

const (
	tlsNodeCert = "node.crt"
	tlsNodeKey  = "node.key"
)

var (
	bindall bool

	// used for computing uptime
	startTime = time.Now()

	// Alpha is the sub-command invoked when running "dgraph alpha".
	Alpha x.SubCommand

	// need this here to refer it in admin_backup.go
	adminServer web.IServeGraphQL

	initDone uint32
)

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
	x.FillCommonFlags(flag)

	flag.StringP("postings", "p", "p", "Directory to store posting lists.")

	// Options around how to set up Badger.
	flag.String("badger.compression", "snappy",
		"[none, zstd:level, snappy] Specifies the compression algorithm and the compression"+
			"level (if applicable) for the postings directory. none would disable compression,"+
			" while zstd:1 would set zstd compression at level 1.")
	enc.RegisterFlags(flag)

	// Snapshot and Transactions.
	flag.Int("snapshot_after", 10000,
		"Create a new Raft snapshot after this many number of Raft entries. The"+
			" lower this number, the more frequent snapshot creation would be."+
			" Also determines how often Rollups would happen.")
	flag.String("abort_older_than", "5m",
		"Abort any pending transactions older than this duration. The liveness of a"+
			" transaction is determined by its last mutation.")

	flag.StringP("wal", "w", "w", "Directory to store raft write-ahead logs.")
	flag.String("whitelist", "",
		"A comma separated list of IP addresses, IP ranges, CIDR blocks, or hostnames you "+
			"wish to whitelist for performing admin actions (i.e., --whitelist 144.142.126.254,"+
			"127.0.0.1:127.0.0.3,192.168.0.0/16,host.docker.internal)")
	flag.String("export", "export", "Folder in which to store exports.")
	flag.Int("pending_proposals", 256,
		"Number of pending mutation proposals. Useful for rate limiting.")
	flag.StringP("zero", "z", fmt.Sprintf("localhost:%d", x.PortZeroGrpc),
		"Comma separated list of Dgraph zero addresses of the form IP_ADDRESS:PORT.")
	flag.Uint64("idx", 0,
		"Optional Raft ID that this Dgraph Alpha will use to join RAFT groups.")
	flag.Int("max_retries", -1,
		"Commits to disk will give up after these number of retries to prevent locking the worker"+
			" in a failed state. Use -1 to retry infinitely.")
	flag.String("auth_token", "",
		"If set, all Admin requests to Dgraph would need to have this token."+
			" The token can be passed as follows: For HTTP requests, in X-Dgraph-AuthToken header."+
			" For Grpc, in auth-token key in the context.")

	flag.String("acl_secret_file", "", "The file that stores the HMAC secret, "+
		"which is used for signing the JWT and should have at least 32 ASCII characters. "+
		"Enterprise feature.")
	flag.Duration("acl_access_ttl", 6*time.Hour, "The TTL for the access jwt. "+
		"Enterprise feature.")
	flag.Duration("acl_refresh_ttl", 30*24*time.Hour, "The TTL for the refresh jwt. "+
		"Enterprise feature.")
	flag.String("mutations", "allow",
		"Set mutation mode to allow, disallow, or strict.")

	// Useful for running multiple servers on the same machine.
	flag.IntP("port_offset", "o", 0,
		"Value added to all listening port numbers. [Internal=7080, HTTP=8080, Grpc=9080]")

	flag.Uint64("query_edge_limit", 1e6,
		"Limit for the maximum number of edges that can be returned in a query."+
			" This applies to shortest path and recursive queries.")
	flag.Uint64("normalize_node_limit", 1e4,
		"Limit for the maximum number of nodes that can be returned in a query that uses the "+
			"normalize directive.")
	flag.Uint64("mutations_nquad_limit", 1e6,
		"Limit for the maximum number of nquads that can be inserted in a mutation request")

	// TLS configurations
	flag.String("tls_dir", "", "Path to directory that has TLS certificates and keys.")
	flag.Bool("tls_use_system_ca", true, "Include System CA into CA Certs.")
	flag.String("tls_client_auth", "VERIFYIFGIVEN", "Enable TLS client authentication")

	//Custom plugins.
	flag.String("custom_tokenizers", "",
		"Comma separated list of tokenizer plugins")

	// By default Go GRPC traces all requests.
	grpc.EnableTracing = false

	flag.Bool("graphql_introspection", true, "Set to false for no GraphQL schema introspection")
	flag.Bool("graphql_debug", false, "Enable debug mode in GraphQL. This returns auth errors to clients. We do not recommend turning it on for production.")

	// Ludicrous mode
	flag.Bool("ludicrous_mode", false, "Run Dgraph in ludicrous mode.")
	flag.Int("ludicrous_concurrency", 2000, "Number of concurrent threads in ludicrous mode")

	flag.Bool("graphql_extensions", true, "Set to false if extensions not required in GraphQL response body")
	flag.Duration("graphql_poll_interval", time.Second, "polling interval for graphql subscription.")
	flag.String("graphql_lambda_url", "",
		"URL of lambda server that implements custom GraphQL JavaScript resolvers")

	// Cache flags
	flag.String("cache_percentage", "0,65,35,0",
		`Cache percentages summing up to 100 for various caches (FORMAT:
		PostingListCache,PstoreBlockCache,PstoreIndexCache,WAL).`)
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
		isIPv6 := strings.Contains(s, "::")
		tuple := strings.Split(s, ":")
		switch {
		case isIPv6 || len(tuple) == 1:
			if !strings.Contains(s, "/") {
				// string is hostname like host.docker.internal,
				// or IPv4 address like 144.124.126.254,
				// or IPv6 address like fd03:b188:0f3c:9ec4::babe:face
				ipAddr := net.ParseIP(s)
				if ipAddr != nil {
					ipRanges = append(ipRanges, x.IPRange{Lower: ipAddr, Upper: ipAddr})
				} else {
					ipAddrs, err := net.LookupIP(s)
					if err != nil {
						return nil, errors.Errorf("invalid IP address or hostname: %s", s)
					}

					for _, addr := range ipAddrs {
						ipRanges = append(ipRanges, x.IPRange{Lower: addr, Upper: addr})
					}
				}
			} else {
				// string is CIDR block like 192.168.0.0/16 or fd03:b188:0f3c:9ec4::/64
				rangeLo, network, err := net.ParseCIDR(s)
				if err != nil {
					return nil, errors.Errorf("invalid CIDR block: %s", s)
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
			switch {
			case rangeLo == nil:
				return nil, errors.Errorf("invalid IP address: %s", tuple[0])
			case rangeHi == nil:
				return nil, errors.Errorf("invalid IP address: %s", tuple[1])
			case bytes.Compare(rangeLo, rangeHi) > 0:
				return nil, errors.Errorf("inverted IP address range: %s", s)
			}
			ipRanges = append(ipRanges, x.IPRange{Lower: rangeLo, Upper: rangeHi})
		default:
			return nil, errors.Errorf("invalid IP address range: %s", s)
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
	var err error

	if _, ok := r.URL.Query()["all"]; ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		ctx := x.AttachAccessJwt(context.Background(), r)
		var resp *api.Response
		if resp, err = (&edgraph.Server{}).Health(ctx, true); err != nil {
			x.SetStatus(w, x.Error, err.Error())
			return
		}
		if resp == nil {
			x.SetStatus(w, x.ErrorNoData, "No health information available.")
			return
		}
		_, _ = w.Write(resp.Json)
		return
	}

	_, ok := r.URL.Query()["live"]
	if !ok {
		if err := x.HealthCheck(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, err = w.Write([]byte(err.Error()))
			if err != nil {
				glog.V(2).Infof("Error while writing health check response: %v", err)
			}
			return
		}
	}

	var resp *api.Response
	if resp, err = (&edgraph.Server{}).Health(context.Background(), false); err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	if resp == nil {
		x.SetStatus(w, x.ErrorNoData, "No health information available.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Json)
}

func stateHandler(w http.ResponseWriter, r *http.Request) {
	var err error
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "application/json")

	ctx := context.Background()
	ctx = x.AttachAccessJwt(ctx, r)

	var aResp *api.Response
	if aResp, err = (&edgraph.Server{}).State(ctx); err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
	if aResp == nil {
		x.SetStatus(w, x.ErrorNoData, "No state information available.")
		return
	}

	if _, err = w.Write(aResp.Json); err != nil {
		x.SetStatus(w, x.Error, err.Error())
		return
	}
}

// storeStatsHandler outputs some basic stats for data store.
func storeStatsHandler(w http.ResponseWriter, r *http.Request) {
	x.AddCorsHeaders(w)
	w.Header().Set("Content-Type", "text/html")
	x.Check2(w.Write([]byte("<pre>")))
	x.Check2(w.Write([]byte(worker.StoreStats())))
	x.Check2(w.Write([]byte("</pre>")))
}

func setupListener(addr string, port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("%s:%d", addr, port))
}

func serveGRPC(l net.Listener, tlsCfg *tls.Config, closer *z.Closer) {
	defer closer.Done()

	x.RegisterExporters(Alpha.Conf, "dgraph.alpha")

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

func serveHTTP(l net.Listener, tlsCfg *tls.Config, closer *z.Closer) {
	defer closer.Done()
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

func setupServer(closer *z.Closer) {
	go worker.RunServer(bindall) // For pb.communication.

	laddr := "localhost"
	if bindall {
		laddr = "0.0.0.0"
	}

	tlsCfg, err := x.LoadServerTLSConfig(Alpha.Conf, tlsNodeCert, tlsNodeKey)
	if err != nil {
		log.Fatalf("Failed to setup TLS: %v\n", err)
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
	http.HandleFunc("/query/", queryHandler)
	http.HandleFunc("/mutate", mutationHandler)
	http.HandleFunc("/mutate/", mutationHandler)
	http.HandleFunc("/commit", commitHandler)
	http.HandleFunc("/alter", alterHandler)
	http.HandleFunc("/health", healthCheck)
	http.HandleFunc("/state", stateHandler)

	// TODO: Figure out what this is for?
	http.HandleFunc("/debug/store", storeStatsHandler)

	introspection := Alpha.Conf.GetBool("graphql_introspection")

	// Global Epoch is a lockless synchronization mechanism for graphql service.
	// It's is just an atomic counter used by the graphql subscription to update its state.
	// It's is used to detect the schema changes and server exit.

	// Implementation for schema change:
	// The global epoch is incremented when there is a schema change.
	// Polling goroutine acquires the current epoch count as a local epoch.
	// The local epoch count is checked against the global epoch,
	// If there is change then we terminate the subscription.

	// Implementation for server exit:
	// The global epoch is set to maxUint64 while exiting the server.
	// By using this information polling goroutine terminates the subscription.
	globalEpoch := uint64(0)
	var mainServer web.IServeGraphQL
	var gqlHealthStore *admin.GraphQLHealthStore
	// Do not use := notation here because adminServer is a global variable.
	mainServer, adminServer, gqlHealthStore = admin.NewServers(introspection, &globalEpoch, closer)
	http.Handle("/graphql", mainServer.HTTPHandler())
	http.HandleFunc("/probe/graphql", func(w http.ResponseWriter, r *http.Request) {
		healthStatus := gqlHealthStore.GetHealth()
		httpStatusCode := http.StatusOK
		if !healthStatus.Healthy {
			httpStatusCode = http.StatusServiceUnavailable
		}
		w.WriteHeader(httpStatusCode)
		w.Header().Set("Content-Type", "application/json")
		x.Check2(w.Write([]byte(fmt.Sprintf(`{"status":"%s"}`, healthStatus.StatusMsg))))
	})
	http.Handle("/admin", allowedMethodsHandler(allowedMethods{
		http.MethodGet:     true,
		http.MethodPost:    true,
		http.MethodOptions: true,
	}, adminAuthHandler(adminServer.HTTPHandler())))

	http.Handle("/admin/schema", adminAuthHandler(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		adminSchemaHandler(w, r, adminServer)
	})))

	http.Handle("/admin/schema/validate", http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		schema := readRequest(w, r)
		w.Header().Set("Content-Type", "application/json")

		err := admin.SchemaValidate(string(schema))
		if err == nil {
			w.WriteHeader(http.StatusOK)
			x.SetStatus(w, "success", "Schema is valid")
			return
		}

		w.WriteHeader(http.StatusBadRequest)
		errs := strings.Split(strings.TrimSpace(err.Error()), "\n")
		x.SetStatusWithErrors(w, x.ErrorInvalidRequest, errs)
	}))

	http.Handle("/admin/shutdown", allowedMethodsHandler(allowedMethods{http.MethodGet: true},
		adminAuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			shutDownHandler(w, r, adminServer)
		}))))

	http.Handle("/admin/draining", allowedMethodsHandler(allowedMethods{
		http.MethodPut:  true,
		http.MethodPost: true,
	}, adminAuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		drainingHandler(w, r, adminServer)
	}))))

	http.Handle("/admin/export", allowedMethodsHandler(allowedMethods{http.MethodGet: true},
		adminAuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			exportHandler(w, r, adminServer)
		}))))

	http.Handle("/admin/config/cache_mb", allowedMethodsHandler(allowedMethods{
		http.MethodGet: true,
		http.MethodPut: true,
	}, adminAuthHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		memoryLimitHandler(w, r, adminServer)
	}))))

	addr := fmt.Sprintf("%s:%d", laddr, httpPort())
	glog.Infof("Bringing up GraphQL HTTP API at %s/graphql", addr)
	glog.Infof("Bringing up GraphQL HTTP admin API at %s/admin", addr)

	// Add OpenCensus z-pages.
	zpages.Handle(http.DefaultServeMux, "/z")

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/ui/keywords", keywordHandler)

	// Initialize the servers.
	admin.ServerCloser = z.NewCloser(3)
	go serveGRPC(grpcListener, tlsCfg, admin.ServerCloser)
	go serveHTTP(httpListener, tlsCfg, admin.ServerCloser)

	if Alpha.Conf.GetBool("telemetry") {
		go edgraph.PeriodicallyPostTelemetry()
	}

	go func() {
		defer admin.ServerCloser.Done()

		<-admin.ServerCloser.HasBeenClosed()
		atomic.StoreUint64(&globalEpoch, math.MaxUint64)

		// Stops grpc/http servers; Already accepted connections are not closed.
		if err := grpcListener.Close(); err != nil {
			glog.Warningf("Error while closing gRPC listener: %s", err)
		}
		if err := httpListener.Close(); err != nil {
			glog.Warningf("Error while closing HTTP listener: %s", err)
		}
	}()

	glog.Infoln("gRPC server started.  Listening on port", grpcPort())
	glog.Infoln("HTTP server started.  Listening on port", httpPort())

	atomic.AddUint32(&initDone, 1)
	admin.ServerCloser.Wait()
}

func run() {
	var err error
	if Alpha.Conf.GetBool("enable_sentry") {
		x.InitSentry(enc.EeBuild)
		defer x.FlushSentry()
		x.ConfigureSentryScope("alpha")
		x.WrapPanics()
		x.SentryOptOutNote()
	}
	bindall = Alpha.Conf.GetBool("bindall")

	totalCache := int64(Alpha.Conf.GetInt("cache_mb"))
	x.AssertTruef(totalCache >= 0, "ERROR: Cache size must be non-negative")

	cachePercentage := Alpha.Conf.GetString("cache_percentage")
	cachePercent, err := x.GetCachePercentages(cachePercentage, 4)
	x.Check(err)
	postingListCacheSize := (cachePercent[0] * (totalCache << 20)) / 100
	pstoreBlockCacheSize := (cachePercent[1] * (totalCache << 20)) / 100
	pstoreIndexCacheSize := (cachePercent[2] * (totalCache << 20)) / 100
	walCache := (cachePercent[3] * (totalCache << 20)) / 100

	ctype, clevel := x.ParseCompression(Alpha.Conf.GetString("badger.compression"))
	opts := worker.Options{
		PostingDir:                 Alpha.Conf.GetString("postings"),
		WALDir:                     Alpha.Conf.GetString("wal"),
		PostingDirCompression:      ctype,
		PostingDirCompressionLevel: clevel,
		CachePercentage:            cachePercentage,
		PBlockCacheSize:            pstoreBlockCacheSize,
		PIndexCacheSize:            pstoreIndexCacheSize,
		WalCache:                   walCache,

		MutationsMode: worker.AllowMutations,
		AuthToken:     Alpha.Conf.GetString("auth_token"),
	}

	secretFile := Alpha.Conf.GetString("acl_secret_file")
	if secretFile != "" {
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

		glog.Info("HMAC secret loaded successfully.")
	}

	switch strings.ToLower(Alpha.Conf.GetString("mutations")) {
	case "allow":
		opts.MutationsMode = worker.AllowMutations
	case "disallow":
		opts.MutationsMode = worker.DisallowMutations
	case "strict":
		opts.MutationsMode = worker.StrictMutations
	default:
		glog.Error("--mutations argument must be one of allow, disallow, or strict")
		os.Exit(1)
	}

	worker.SetConfiguration(&opts)

	ips, err := getIPsFromString(Alpha.Conf.GetString("whitelist"))
	x.Check(err)

	abortDur, err := time.ParseDuration(Alpha.Conf.GetString("abort_older_than"))
	x.Check(err)

	x.WorkerConfig = x.WorkerOptions{
		ExportPath:           Alpha.Conf.GetString("export"),
		NumPendingProposals:  Alpha.Conf.GetInt("pending_proposals"),
		ZeroAddr:             strings.Split(Alpha.Conf.GetString("zero"), ","),
		RaftId:               cast.ToUint64(Alpha.Conf.GetString("idx")),
		WhiteListedIPRanges:  ips,
		MaxRetries:           Alpha.Conf.GetInt("max_retries"),
		StrictMutations:      opts.MutationsMode == worker.StrictMutations,
		AclEnabled:           secretFile != "",
		SnapshotAfter:        Alpha.Conf.GetInt("snapshot_after"),
		AbortOlderThan:       abortDur,
		StartTime:            startTime,
		LudicrousMode:        Alpha.Conf.GetBool("ludicrous_mode"),
		LudicrousConcurrency: Alpha.Conf.GetInt("ludicrous_concurrency"),
	}
	x.WorkerConfig.Parse(Alpha.Conf)

	if x.WorkerConfig.EncryptionKey, err = enc.ReadKey(Alpha.Conf); err != nil {
		glog.Infof("unable to read key %v", err)
		return
	}

	setupCustomTokenizers()
	x.Init()
	x.Config.PortOffset = Alpha.Conf.GetInt("port_offset")
	x.Config.QueryEdgeLimit = cast.ToUint64(Alpha.Conf.GetString("query_edge_limit"))
	x.Config.NormalizeNodeLimit = cast.ToInt(Alpha.Conf.GetString("normalize_node_limit"))
	x.Config.MutationsNQuadLimit = cast.ToInt(Alpha.Conf.GetString("mutations_nquad_limit"))
	x.Config.PollInterval = Alpha.Conf.GetDuration("graphql_poll_interval")
	x.Config.GraphqlExtension = Alpha.Conf.GetBool("graphql_extensions")
	x.Config.GraphqlDebug = Alpha.Conf.GetBool("graphql_debug")
	x.Config.GraphqlLambdaUrl = Alpha.Conf.GetString("graphql_lambda_url")
	if x.Config.GraphqlLambdaUrl != "" {
		graphqlLambdaUrl, err := url.Parse(x.Config.GraphqlLambdaUrl)
		if err != nil {
			glog.Errorf("unable to parse graphql_lambda_url: %v", err)
			return
		}
		if !graphqlLambdaUrl.IsAbs() {
			glog.Errorf("expecting graphql_lambda_url to be an absolute URL, got: %s",
				graphqlLambdaUrl.String())
			return
		}
	}

	x.PrintVersion()
	glog.Infof("x.Config: %+v", x.Config)
	glog.Infof("x.WorkerConfig: %+v", x.WorkerConfig)
	glog.Infof("worker.Config: %+v", worker.Config)

	worker.InitServerState()

	if Alpha.Conf.GetBool("expose_trace") {
		// TODO: Remove this once we get rid of event logs.
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
	}
	otrace.ApplyConfig(otrace.Config{
		DefaultSampler:             otrace.ProbabilitySampler(x.WorkerConfig.Tracing),
		MaxAnnotationEventsPerSpan: 256,
	})

	// Posting will initialize index which requires schema. Hence, initialize
	// schema before calling posting.Init().
	schema.Init(worker.State.Pstore)
	posting.Init(worker.State.Pstore, postingListCacheSize)
	defer posting.Cleanup()
	worker.Init(worker.State.Pstore)

	// setup shutdown os signal handler
	sdCh := make(chan os.Signal, 3)

	defer func() {
		signal.Stop(sdCh)
		close(sdCh)
	}()
	// sigint : Ctrl-C, sigterm : kill command.
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		var numShutDownSig int
		for range sdCh {
			select {
			case <-admin.ServerCloser.HasBeenClosed():
			default:
				admin.ServerCloser.Signal()
			}
			numShutDownSig++
			glog.Infoln("Caught Ctrl-C. Terminating now (this may take a few seconds)...")

			switch {
			case atomic.LoadUint32(&initDone) < 2:
				// Forcefully kill alpha if we haven't finish server initialization.
				glog.Infoln("Stopped before initialization completed")
				os.Exit(1)
			case numShutDownSig == 3:
				glog.Infoln("Signaled thrice. Aborting!")
				os.Exit(1)
			}
		}
	}()

	updaters := z.NewCloser(4)
	go func() {
		worker.StartRaftNodes(worker.State.WALstore, bindall)
		atomic.AddUint32(&initDone, 1)

		// initialization of the admin account can only be done after raft nodes are running
		// and health check passes
		edgraph.ResetAcl(updaters)
		edgraph.RefreshAcls(updaters)
		edgraph.ResetCors(updaters)
		// Update the accepted cors origins.
		for updaters.Ctx().Err() == nil {
			origins, err := edgraph.GetCorsOrigins(updaters.Ctx())
			if err != nil {
				glog.Errorf("Error while retriving cors origins: %s", err.Error())
				continue
			}
			x.UpdateCorsOrigins(origins)
			return
		}
	}()
	// Listen for any new cors origin update.
	go listenForCorsUpdate(updaters)

	// Graphql subscribes to alpha to get schema updates. We need to close that before we
	// close alpha. This closer is for closing and waiting that subscription.
	adminCloser := z.NewCloser(1)

	setupServer(adminCloser)
	glog.Infoln("GRPC and HTTP stopped.")

	// This might not close until group is given the signal to close. So, only signal here,
	// wait for it after group is closed.
	updaters.Signal()

	worker.BlockingStop()
	glog.Infoln("worker stopped.")

	adminCloser.SignalAndWait()
	glog.Infoln("adminCloser closed.")

	worker.State.Dispose()
	x.RemoveCidFile()
	glog.Info("worker.State disposed.")

	updaters.Wait()
	glog.Infoln("updaters closed.")

	glog.Infoln("Server shutdown. Bye!")
}

// listenForCorsUpdate listen for any cors change and update the accepeted cors.
func listenForCorsUpdate(closer *z.Closer) {
	prefix := x.DataKey("dgraph.cors", 0)
	// Remove uid from the key, to get the correct prefix
	prefix = prefix[:len(prefix)-8]
	worker.SubscribeForUpdates([][]byte{prefix}, func(kvs *badgerpb.KVList) {
		// Last update contains the latest value. So, taking the last update.
		lastIdx := len(kvs.GetKv()) - 1
		kv := kvs.GetKv()[lastIdx]
		glog.Infof("Updating cors from subscription.")
		// Unmarshal the incoming posting list.
		pl := &pb.PostingList{}
		err := pl.Unmarshal(kv.GetValue())
		if err != nil {
			glog.Errorf("Unable to unmarshal the posting list for cors update %s", err)
			return
		}
		// Skip if there is no posting. Our all upsert call contains atleast one
		// posting.
		if len(pl.Postings) == 0 {
			return
		}
		origins := make([]string, 0)
		for _, posting := range pl.Postings {
			val := strings.TrimSpace(string(posting.Value))
			if val == "_STAR_ALL" {
				// If the posting list contains __STAR_ALL then it's a delete call.
				// we usually do it before updating as part of upsert. So, let's
				// ignore this update.
				continue
			}
			origins = append(origins, val)
		}
		glog.Infof("Updating cors origins: %+v", origins)
		x.UpdateCorsOrigins(origins)
	}, 1, closer)
}
