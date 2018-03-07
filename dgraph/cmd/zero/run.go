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

package zero

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/dgraph-io/badger"
	bopts "github.com/dgraph-io/badger/options"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/x"
	"github.com/spf13/cobra"
)

type options struct {
	bindall           bool
	myAddr            string
	portOffset        int
	nodeId            uint64
	numReplicas       int
	peer              string
	w                 string
	rebalanceInterval time.Duration
}

var opts options

var Zero x.SubCommand

func init() {
	Zero.Cmd = &cobra.Command{
		Use:   "zero",
		Short: "Run Dgraph zero server",
		Long: `
A Dgraph zero instance manages the Dgraph cluster.  Typically, a single Zero
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
	flag.Bool("bindall", true,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	flag.String("my", "",
		"addr:port of this server, so other Dgraph servers can talk to this.")
	flag.IntP("port_offset", "o", 0,
		"Value added to all listening port numbers. [Grpc=5080, HTTP=6080]")
	flag.Uint64("idx", 1, "Unique node index for this server.")
	flag.Int("replicas", 1, "How many replicas to run per data shard."+
		" The count includes the original shard.")
	flag.String("peer", "", "Address of another dgraphzero server.")
	flag.StringP("wal", "w", "zw", "Directory storing WAL.")
	flag.Duration("rebalance_interval", 8*time.Minute, "Interval for trying a predicate move.")
}

func setupListener(addr string, port int, kind string) (listener net.Listener, err error) {
	laddr := fmt.Sprintf("%s:%d", addr, port)
	fmt.Printf("Setting up %s listener at: %v\n", kind, laddr)
	return net.Listen("tcp", laddr)
}

type state struct {
	node *node
	rs   *conn.RaftServer
	zero *Server
}

func (st *state) serveGRPC(l net.Listener, wg *sync.WaitGroup) {
	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000))

	rc := intern.RaftContext{Id: opts.nodeId, Addr: opts.myAddr, Group: 0}
	m := conn.NewNode(&rc)
	st.rs = &conn.RaftServer{Node: m}

	st.node = &node{Node: m, ctx: context.Background(), stop: make(chan struct{})}
	st.zero = &Server{NumReplicas: opts.numReplicas, Node: st.node}
	st.zero.Init()
	st.node.server = st.zero

	intern.RegisterZeroServer(s, st.zero)
	intern.RegisterRaftServer(s, st.rs)

	go func() {
		defer wg.Done()
		err := s.Serve(l)
		log.Printf("gRpc server stopped : %s", err.Error())
		st.node.stop <- struct{}{}

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
			log.Printf("Stopping grpc gracefully is taking longer than %v."+
				" Force stopping now. Pending RPCs will be abandoned.", timeout)
			s.Stop()
		}
	}()
}

func run() {
	opts = options{
		bindall:           Zero.Conf.GetBool("bindall"),
		myAddr:            Zero.Conf.GetString("my"),
		portOffset:        Zero.Conf.GetInt("port_offset"),
		nodeId:            uint64(Zero.Conf.GetInt("idx")),
		numReplicas:       Zero.Conf.GetInt("replicas"),
		peer:              Zero.Conf.GetString("peer"),
		w:                 Zero.Conf.GetString("wal"),
		rebalanceInterval: Zero.Conf.GetDuration("rebalance_interval"),
	}

	grpc.EnableTracing = false

	addr := "localhost"
	if opts.bindall {
		addr = "0.0.0.0"
	}
	if len(opts.myAddr) == 0 {
		opts.myAddr = fmt.Sprintf("localhost:%d", x.PortZeroGrpc+opts.portOffset)
	}
	grpcListener, err := setupListener(addr, x.PortZeroGrpc+opts.portOffset, "grpc")
	if err != nil {
		log.Fatal(err)
	}
	httpListener, err := setupListener(addr, x.PortZeroHTTP+opts.portOffset, "http")
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	// Initialize the servers.
	var st state
	st.serveGRPC(grpcListener, &wg)
	st.serveHTTP(httpListener, &wg)

	http.HandleFunc("/state", st.getState)
	http.HandleFunc("/removeNode", st.removeNode)
	http.HandleFunc("/moveTablet", st.moveTablet)

	// Open raft write-ahead log and initialize raft node.
	x.Checkf(os.MkdirAll(opts.w, 0700), "Error while creating WAL dir.")
	kvOpt := badger.DefaultOptions
	kvOpt.SyncWrites = true
	kvOpt.Dir = opts.w
	kvOpt.ValueDir = opts.w
	kvOpt.TableLoadingMode = bopts.MemoryMap
	kv, err := badger.OpenManaged(kvOpt)
	x.Checkf(err, "Error while opening WAL store")
	defer kv.Close()
	wal := raftwal.Init(kv, opts.nodeId)
	x.Check(st.node.initAndStartNode(wal))

	sdCh := make(chan os.Signal, 1)
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		defer wg.Done()
		<-sdCh
		fmt.Println("Shutting down...")
		// Close doesn't close already opened connections.
		httpListener.Close()
		grpcListener.Close()
		close(st.zero.shutDownCh)
		st.node.trySnapshot(0)
	}()

	fmt.Println("Running Dgraph zero...")
	wg.Wait()
	fmt.Println("All done.")
}
