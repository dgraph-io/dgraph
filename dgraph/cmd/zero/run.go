/*
 * Copyright 2017-2023 Dgraph Labs, Inc. and Contributors
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
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.opencensus.io/plugin/ocgrpc"
	otrace "go.opencensus.io/trace"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/ee/audit"
	"github.com/dgraph-io/dgraph/ee/enc"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

type Options struct {
	Raft              *z.SuperFlag
	Telemetry         *z.SuperFlag
	Limit             *z.SuperFlag
	Bindall           bool
	PortOffset        int
	NumReplicas       int
	Peer              string
	W                 string
	RebalanceInterval time.Duration
	TlsClientConfig   *tls.Config
	Audit             *x.LoggerConf
	LimiterConfig     *x.LimiterConf
}

var Opts Options

// Zero is the sub-command used to start Zero servers.
var Zero x.SubCommand

func init() {
	Zero.Cmd = &cobra.Command{
		Use:   "zero",
		Short: "Run Dgraph Zero management server ",
		Long: `
A Dgraph Zero instance manages the Dgraph cluster.  Typically, a single Zero
instance is sufficient for the cluster; however, one can run multiple Zero
instances to achieve high-availability.
`,
		Run: func(cmd *cobra.Command, args []string) {
			defer x.StartProfile(Zero.Conf).Stop()
			run()
		},
		Annotations: map[string]string{"group": "core"},
	}
	Zero.EnvPrefix = "DGRAPH_ZERO"
	Zero.Cmd.SetHelpTemplate(x.NonRootTemplate)

	flag := Zero.Cmd.Flags()
	x.FillCommonFlags(flag)
	// --tls SuperFlag
	x.RegisterServerTLSFlags(flag)

	flag.IntP("port_offset", "o", 0,
		"Value added to all listening port numbers. [Grpc=5080, HTTP=6080]")
	flag.Int("replicas", 1, "How many Dgraph Alpha replicas to run per data shard group."+
		" The count includes the original shard.")
	flag.String("peer", "", "Address of another dgraphzero server.")
	flag.StringP("wal", "w", "zw", "Directory storing WAL.")
	flag.Duration("rebalance_interval", 8*time.Minute, "Interval for trying a predicate move.")
	flag.String("enterprise_license", "", "Path to the enterprise license file.")
	flag.String("cid", "", "Cluster ID")

	flag.String("limit", worker.ZeroLimitsDefaults, z.NewSuperFlagHelp(worker.ZeroLimitsDefaults).
		Head("Limit options").
		Flag("uid-lease",
			`The maximum number of UIDs that can be leased by namespace (except default namespace)
			in an interval specified by refill-interval. Set it to 0 to remove limiting.`).
		Flag("refill-interval",
			"The interval after which the tokens for UID lease are replenished.").
		Flag("disable-admin-http",
			"Turn on/off the administrative endpoints exposed over Zero's HTTP port.").
		String())

	flag.String("raft", RaftDefaults, z.NewSuperFlagHelp(RaftDefaults).
		Head("Raft options").
		Flag("idx",
			"Provides an optional Raft ID that this Alpha would use to join Raft groups.").
		Flag("learner",
			`Make this Zero a "learner" node. In learner mode, this Zero will not participate `+
				"in Raft elections. This can be used to achieve a read-only replica.").
		String())

	flag.String("audit", worker.AuditDefaults, z.NewSuperFlagHelp(worker.AuditDefaults).
		Head("Audit options").
		Flag("output",
			`[stdout, /path/to/dir] This specifies where audit logs should be output to.
			"stdout" is for standard output. You can also specify the directory where audit logs
			will be saved. When stdout is specified as output other fields will be ignored.`).
		Flag("compress",
			"Enables the compression of old audit logs.").
		Flag("encrypt-file",
			"The path to the key file to be used for audit log encryption.").
		Flag("days",
			"The number of days audit logs will be preserved.").
		Flag("size",
			"The audit log max size in MB after which it will be rolled over.").
		String())
}

func setupListener(addr string, port int, kind string) (listener net.Listener, err error) {
	laddr := fmt.Sprintf("%s:%d", addr, port)
	glog.Infof("Setting up %s listener at: %v\n", kind, laddr)
	return net.Listen("tcp", laddr)
}

func SetOptions(opts Options) {
	Opts = opts
}

type State struct {
	node *node
	rs   *conn.RaftServer
	zero *Server
}

func (st *State) serveGRPC(conf *viper.Viper, l net.Listener, store *raftwal.DiskStorage) {
	x.RegisterExporters(conf, "dgraph.zero")
	grpcOpts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000),
		grpc.StatsHandler(&ocgrpc.ServerHandler{}),
		grpc.UnaryInterceptor(audit.AuditRequestGRPC),
	}

	tlsConf, err := x.LoadServerTLSConfigForInternalPort(conf)
	x.Check(err)
	if tlsConf != nil {
		grpcOpts = append(grpcOpts, grpc.Creds(credentials.NewTLS(tlsConf)))
	}
	s := grpc.NewServer(grpcOpts...)

	nodeId := Opts.Raft.GetUint64("idx")
	rc := pb.RaftContext{
		Id:        nodeId,
		Addr:      x.WorkerConfig.MyAddr,
		Group:     0,
		IsLearner: Opts.Raft.GetBool("learner"),
	}
	m := conn.NewNode(&rc, store, Opts.TlsClientConfig)

	// Zero followers should not be forwarding proposals to the leader, to avoid txn commits which
	// were calculated in a previous Zero leader.
	m.Cfg.DisableProposalForwarding = true
	st.rs = conn.NewRaftServer(m)

	st.node = &node{Node: m, ctx: context.Background(), closer: z.NewCloser(1)}
	st.zero = &Server{NumReplicas: Opts.NumReplicas, Node: st.node, tlsClientConfig: Opts.TlsClientConfig}
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
	telemetry := z.NewSuperFlag(Zero.Conf.GetString("telemetry")).MergeAndCheckDefault(
		x.TelemetryDefaults)
	if telemetry.GetBool("sentry") {
		x.InitSentry(enc.EeBuild)
		defer x.FlushSentry()
		x.ConfigureSentryScope("zero")
		x.WrapPanics()
		x.SentryOptOutNote()
	}

	x.PrintVersion()
	tlsConf, err := x.LoadClientTLSConfigForInternalPort(Zero.Conf)
	x.Check(err)

	raft := z.NewSuperFlag(Zero.Conf.GetString("raft")).MergeAndCheckDefault(
		RaftDefaults)
	auditConf := audit.GetAuditConf(Zero.Conf.GetString("audit"))
	limit := z.NewSuperFlag(Zero.Conf.GetString("limit")).MergeAndCheckDefault(
		worker.ZeroLimitsDefaults)
	limitConf := &x.LimiterConf{
		UidLeaseLimit: limit.GetUint64("uid-lease"),
		RefillAfter:   limit.GetDuration("refill-interval"),
	}
	SetOptions(Options{
		Telemetry:         telemetry,
		Raft:              raft,
		Limit:             limit,
		Bindall:           Zero.Conf.GetBool("bindall"),
		PortOffset:        Zero.Conf.GetInt("port_offset"),
		NumReplicas:       Zero.Conf.GetInt("replicas"),
		Peer:              Zero.Conf.GetString("peer"),
		W:                 Zero.Conf.GetString("wal"),
		RebalanceInterval: Zero.Conf.GetDuration("rebalance_interval"),
		TlsClientConfig:   tlsConf,
		Audit:             auditConf,
		LimiterConfig:     limitConf,
	})
	glog.Infof("Setting Config to: %+v", Opts)
	x.WorkerConfig.Parse(Zero.Conf)

	if !enc.EeBuild && Zero.Conf.GetString("enterprise_license") != "" {
		log.Fatalf("ERROR: enterprise_license option cannot be applied to OSS builds. ")
	}

	if Opts.NumReplicas < 0 || Opts.NumReplicas%2 == 0 {
		log.Fatalf("ERROR: Number of replicas must be odd for consensus. Found: %d",
			Opts.NumReplicas)
	}

	if Zero.Conf.GetBool("expose_trace") {
		// TODO: Remove this once we get rid of event logs.
		trace.AuthRequest = func(req *http.Request) (any, sensitive bool) {
			return true, true
		}
	}

	if Opts.Audit != nil {
		wd, err := filepath.Abs(Opts.W)
		x.Check(err)
		ad, err := filepath.Abs(Opts.Audit.Output)
		x.Check(err)
		x.AssertTruef(ad != wd,
			"WAL directory and Audit output cannot be the same ('%s').", Opts.Audit.Output)
	}

	if Opts.RebalanceInterval <= 0 {
		log.Fatalf("ERROR: Rebalance interval must be greater than zero. Found: %d",
			Opts.RebalanceInterval)
	}

	grpc.EnableTracing = false
	otrace.ApplyConfig(otrace.Config{
		DefaultSampler: otrace.ProbabilitySampler(Zero.Conf.GetFloat64("trace"))})

	addr := "localhost"
	if Opts.Bindall {
		addr = "0.0.0.0"
	}
	if x.WorkerConfig.MyAddr == "" {
		x.WorkerConfig.MyAddr = fmt.Sprintf("localhost:%d", x.PortZeroGrpc+Opts.PortOffset)
	}

	nodeId := Opts.Raft.GetUint64("idx")
	if nodeId == 0 {
		log.Fatalf("ERROR: raft.idx flag cannot be 0. Please set idx to a unique positive integer.")
	}
	grpcListener, err := setupListener(addr, x.PortZeroGrpc+Opts.PortOffset, "grpc")
	x.Check(err)
	httpListener, err := setupListener(addr, x.PortZeroHTTP+Opts.PortOffset, "http")
	x.Check(err)

	// Create and initialize write-ahead log.
	x.Checkf(os.MkdirAll(Opts.W, 0700), "Error while creating WAL dir.")
	store := raftwal.Init(Opts.W)
	store.SetUint(raftwal.RaftId, nodeId)
	store.SetUint(raftwal.GroupId, 0) // All zeros have group zero.

	// Initialize the servers.
	var st State
	st.InitServers(Zero.Conf, grpcListener, httpListener, store)

	st.RegisterHttpHandlers(limit)

	// This must be here. It does not work if placed before Grpc init.
	st.InitAndStartNode()

	if Opts.Telemetry.GetBool("reports") {
		go st.PeriodicallyPostTelemetry()
	}

	sdCh := make(chan os.Signal, 1)
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// handle signals
	go st.HandleSignals(sdCh)

	st.HandleShutDown(sdCh, grpcListener, httpListener)

	st.MonitorMetrics()

	st.Wait()

	st.Close(store)
}
