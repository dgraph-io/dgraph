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

	"github.com/dgraph-io/dgraph/query/pb"
	"github.com/dgraph-io/dgraph/x"
)

var glog = x.Log("client")
var port = flag.String("port", "9090", "Port to communicate with server")
var query = flag.String("query", "", "Query sent to the server")

func main() {
	flag.Parse()
	// TODO(pawan): Pick address for server from config
	conn, err := grpc.Dial("127.0.0.1:"+*port, grpc.WithInsecure())
	if err != nil {
		x.Err(glog, err).Fatal("DialTCPConnection")
	}
	defer conn.Close()

	c := pb.NewDGraphClient(conn)

	r, err := c.GetResponse(context.Background(), &pb.GraphRequest{Query: *query})
	if err != nil {
		x.Err(glog, err).Fatal("Error in getting response from server")
	}

	// TODO(pawan): Remove this later
	fmt.Printf("Subgraph %+v", r)

}
