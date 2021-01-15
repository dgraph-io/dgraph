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

package zero

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/dgraph-io/dgraph/ee/audit"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.opencensus.io/plugin/ocgrpc"
	otrace "go.opencensus.io/trace"
	"go.opencensus.io/zpages"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

type options struct {
	bindall           bool
	portOffset        int
	nodeId            uint64
	numReplicas       int
	peer              string
	w                 string
	rebalanceInterval time.Duration
	tlsClientConfig   *tls.Config
}

var opts options

// Zero is the sub-command used to start Zero servers.
var Zero x.SubCommand

func init() {
	Zero.Cmd = &cobra.Command{
		Use:   "zero",
		Short: "Run Dgraph Zero",
		Long: `
A Dgraph Zero instance manages the Dgraph cluster.  Typically, a single Zero
instance is sufficient for the cluster; however, one can run multiple Zero
instances to achieve high-availability.
`,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Zero.Conf).Stop()
			run()
		},
	}
	Zero.EnvPrefix = "DGRAPH_ZERO"

	flag := Zero.Cmd.Flags()
	x.FillCommonFlags(flag)

	flag.IntP("port_offset", "o", 0,
		"Value added to all listening port numbers. [Grpc=5080, HTTP=6080]")
	flag.Uint64("idx", 1, "Unique node index for this server. idx cannot be 0.")
	flag.Int("replicas", 1, "How many replicas to run per data shard."+
		" The count includes the original shard.")
	flag.String("peer", "", "Address of another dgraphzero server.")
	flag.StringP("wal", "w", "zw", "Directory storing WAL.")
	flag.Duration("rebalance_interval", 8*time.Minute, "Interval for trying a predicate move.")
	flag.String("enterprise_license", "", "Path to the enterprise license file.")

	flag.Bool("audit_enabled", false, "Set to true to enable audit logs.")
	flag.String("audit_dir", "za", "Set path to directory where to save the audit logs.")

	// TLS configurations
	x.RegisterServerTLSFlags(flag)
}

func setupListener(addr string, port int, kind string) (listener net.Listener, err error) {
	laddr := fmt.Sprintf("%s:%d", addr, port)
	glog.Infof("Setting up %s listener at: %v\n", kind, laddr)
	return net.Listen("tcp", laddr)
}

type state struct {
	node *node
	rs   *conn.RaftServer
	zero *Server
}

func (st *state) serveGRPC(l net.Listener, store *raftwal.DiskStorage) {
	x.RegisterExporters(Zero.Conf, "dgraph.zero")
	grpcOpts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000),
		grpc.StatsHandler(&ocgrpc.ServerHandler{}),
		grpc.UnaryInterceptor(audit.AuditRequestGRPC),
	}

	tlsConf, err := x.LoadServerTLSConfigForInternalPort(Zero.Conf)
	x.Check(err)
	if tlsConf != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsConf)))
	}
	s := grpc.NewServer(grpcOpts...)

	rc := pb.RaftContext{Id: opts.nodeId, Addr: x.WorkerConfig.MyAddr, Group: 0}
	m := conn.NewNode(&rc, store, opts.tlsClientConfig)

	// Zero followers should not be forwarding proposals to the leader, to avoid txn commits which
	// were calculated in a previous Zero leader.
	m.Cfg.DisableProposalForwarding = true
	st.rs = conn.NewRaftServer(m)

	st.node = &node{Node: m, ctx: context.Background(), closer: z.NewCloser(1)}
	st.zero = &Server{NumReplicas: opts.numReplicas, Node: st.node, tlsClientConfig: opts.tlsClientConfig}
	st.zero.Init()
	st.node.server = st.zero

	pb.RegisterZeroServer(s, st.zero)
	pb.RegisterRaftServer(s, st.rs)

	go func() {
		defer st.zero.closer.Done()
		err := s.Serve(l)
		glog.Infof("gRPC server stopped : %v", err)

		// Attempt graceful stop (waits for pending RPCs), but force a stop if
		// it doesn't happen in a reasonable amount of time.
		done := make(chan struct{})
		const timeout = 5 * time.Second
		go func() {
			s.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(timeout):
			glog.Infof("Stopping grpc gracefully is taking longer than %v."+
				" Force stopping now. Pending RPCs will be abandoned.", timeout)
			s.Stop()
		}
	}()
}

func run() {
	if Zero.Conf.GetBool("enable_sentry") {
		x.InitSentry(enc.EeBuild)
		defer x.FlushSentry()
		x.ConfigureSentryScope("zero")
		x.WrapPanics()
		x.SentryOptOutNote()
	}

	x.PrintVersion()
	tlsConf, err := x.LoadClientTLSConfigForInternalPort(Zero.Conf)
	x.Check(err)
	opts = options{
		bindall:           Zero.Conf.GetBool("bindall"),
		portOffset:        Zero.Conf.GetInt("port_offset"),
		nodeId:            uint64(Zero.Conf.GetInt("idx")),
		numReplicas:       Zero.Conf.GetInt("replicas"),
		peer:              Zero.Conf.GetString("peer"),
		w:                 Zero.Conf.GetString("wal"),
		rebalanceInterval: Zero.Conf.GetDuration("rebalance_interval"),
		tlsClientConfig:   tlsConf,
	}
	glog.Infof("Setting Config to: %+v", opts)

	if opts.nodeId == 0 {
		log.Fatalf("ERROR: idx flag cannot be 0. Please try again with idx as a positive integer")
	}
	x.WorkerConfig.Parse(Zero.Conf)

	if !enc.EeBuild && Zero.Conf.GetString("enterprise_license") != "" {
		log.Fatalf("ERROR: enterprise_license option cannot be applied to OSS builds. ")
	}

	if opts.numReplicas < 0 || opts.numReplicas%2 == 0 {
		log.Fatalf("ERROR: Number of replicas must be odd for consensus. Found: %d",
			opts.numReplicas)
	}

	if Zero.Conf.GetBool("expose_trace") {
		// TODO: Remove this once we get rid of event logs.
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
	}

	wd, err := filepath.Abs(opts.w)
	x.Check(err)
	ad, err := filepath.Abs(Zero.Conf.GetString("audit_dir"))
	x.Check(err)
	x.AssertTruef(ad != wd, "WAL and Audit directory cannot be the same ('%s').",
		Zero.Conf.Get("audit_dir"))


	if opts.rebalanceInterval <= 0 {
		log.Fatalf("ERROR: Rebalance interval must be greater than zero. Found: %d",
			opts.rebalanceInterval)
	}

	grpc.EnableTracing = false
	otrace.ApplyConfig(otrace.Config{
		DefaultSampler: otrace.ProbabilitySampler(Zero.Conf.GetFloat64("trace"))})

	addr := "localhost"
	if opts.bindall {
		addr = "0.0.0.0"
	}
	if x.WorkerConfig.MyAddr == "" {
		x.WorkerConfig.MyAddr = fmt.Sprintf("localhost:%d", x.PortZeroGrpc+opts.portOffset)
	}

	grpcListener, err := setupListener(addr, x.PortZeroGrpc+opts.portOffset, "grpc")
	x.Check(err)
	httpListener, err := setupListener(addr, x.PortZeroHTTP+opts.portOffset, "http")
	x.Check(err)

	// Create and initialize write-ahead log.
	x.Checkf(os.MkdirAll(opts.w, 0700), "Error while creating WAL dir.")
	store := raftwal.Init(opts.w)
	store.SetUint(raftwal.RaftId, opts.nodeId)
	store.SetUint(raftwal.GroupId, 0) // All zeros have group zero.

	// Initialize the servers.
	var st state
	st.serveGRPC(grpcListener, store)

	tlsCfg, err := x.LoadServerTLSConfig(Zero.Conf)
	x.Check(err)
	go x.StartListenHttpAndHttps(httpListener, tlsCfg, st.zero.closer)

	http.HandleFunc("/health", st.pingResponse)
	http.Handle("/state", audit.AuditRequestHttp(http.HandlerFunc(st.getState)))
	http.Handle("/removeNode", audit.AuditRequestHttp(http.HandlerFunc(st.removeNode)))
	http.Handle("/moveTablet", audit.AuditRequestHttp(http.HandlerFunc(st.moveTablet)))
	http.Handle("/assign", audit.AuditRequestHttp(http.HandlerFunc(st.assign)))
	http.Handle("/enterpriseLicense",
		audit.AuditRequestHttp(http.HandlerFunc(st.applyEnterpriseLicense)))
	http.HandleFunc("/jemalloc", x.JemallocHandler)
	zpages.Handle(http.DefaultServeMux, "/z")

	audit.InitAuditorIfNecessary(Zero.Conf, func() bool {
		if st.zero == nil || st.zero.state == nil {
			return false
		}
		return st.zero.state.GetLicense().GetEnabled()
	})

	// This must be here. It does not work if placed before Grpc init.
	x.Check(st.node.initAndStartNode())

	if Zero.Conf.GetBool("telemetry") {
		go st.zero.periodicallyPostTelemetry()
	}

	sdCh := make(chan os.Signal, 1)
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// handle signals
	go func() {
		var sigCnt int
		for sig := range sdCh {
			glog.Infof("--- Received %s signal", sig)
			sigCnt++
			if sigCnt == 1 {
				signal.Stop(sdCh)
				st.zero.closer.Signal()
			} else if sigCnt == 3 {
				glog.Infof("--- Got interrupt signal 3rd time. Aborting now.")
				os.Exit(1)
			} else {
				glog.Infof("--- Ignoring interrupt signal.")
			}
		}
	}()

	st.zero.closer.AddRunning(1)

	go func() {
		defer st.zero.closer.Done()
		<-st.zero.closer.HasBeenClosed()
		glog.Infoln("Shutting down...")
		close(sdCh)
		// Close doesn't close already opened connections.

		// Stop all HTTP requests.
		_ = httpListener.Close()
		// Stop Raft.
		st.node.closer.SignalAndWait()
		// Stop all internal requests.
		_ = grpcListener.Close()

		x.RemoveCidFile()
	}()

	st.zero.closer.AddRunning(1)
	go x.MonitorMemoryMetrics(st.zero.closer)

	glog.Infoln("Running Dgraph Zero...")
	st.zero.closer.Wait()
	glog.Infoln("Closer closed.")

	err = store.Close()
	glog.Infof("Raft WAL closed with err: %v\n", err)
	st.zero.orc.close()
	glog.Infoln("All done. Goodbye!")
	audit.Close()
	glog.Infoln("audit logs are closed")
}
