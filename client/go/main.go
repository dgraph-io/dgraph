/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"flag"
	"fmt"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("client")
var ip = flag.String("ip", "127.0.0.1:8081", "Port to communicate with server")
var q = flag.String("query", "", "Query sent to the server")

func main() {
	flag.Parse()
	// TODO(pawan): Pick address for server from config
	conn, err := grpc.Dial(*ip, grpc.WithInsecure())
	if err != nil {
		x.Err(glog, err).Fatal("DialTCPConnection")
	}
	defer conn.Close()

	c := graph.NewDGraphClient(conn)

	r, err := c.Query(context.Background(), &graph.Request{Query: *q})
	if err != nil {
		x.Err(glog, err).Fatal("Error in getting response from server")
	}

	// TODO(pawan): Remove this later
	fmt.Printf("Subgraph %+v", r)

}
