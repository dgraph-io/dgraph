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
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	bindall = flag.Bool("bindall", false,
		"Use 0.0.0.0 instead of localhost to bind to all addresses on local machine.")
	myAddr = flag.String("my", "",
		"addr:port of this server, so other Dgraph servers can talk to this.")
	port        = flag.Int("port", 8888, "Port to run Dgraph zero on.")
	numReplicas = flag.Int("replicas", 1, "How many replicas to run per data shard."+
		" The count includes the original shard.")
)

func setupListener(addr string, port int) (listener net.Listener, err error) {
	laddr := fmt.Sprintf("%s:%d", addr, port)
	return net.Listen("tcp", laddr)
}

func serveGRPC(l net.Listener, wg *sync.WaitGroup) {
	defer wg.Done()

	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(x.GrpcMaxSize),
		grpc.MaxSendMsgSize(x.GrpcMaxSize),
		grpc.MaxConcurrentStreams(1000))
	server := &Server{NumReplicas: *numReplicas}
	protos.RegisterZeroServer(s, server)
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

func main() {
	rand.Seed(time.Now().UnixNano())
	flag.Parse()

	grpc.EnableTracing = false
	addr := "localhost"
	if *bindall {
		addr = "0.0.0.0"
	}
	httpListener, err := setupListener(addr, *port)
	if err != nil {
		log.Fatal(err)
	}
	grpcListener, err := setupListener(addr, *port+1)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	// Initilize the servers.
	go serveGRPC(grpcListener, &wg)
	go serveHTTP(httpListener, &wg)

	sdCh := make(chan os.Signal, 1)
	signal.Notify(sdCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		defer wg.Done()
		<-sdCh
		fmt.Println("Shutting down...")
		go httpListener.Close()
		go grpcListener.Close()
	}()

	fmt.Println("Running Dgraph zero...")
	wg.Wait()
	fmt.Println("All done.")
}
