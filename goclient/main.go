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
	"context"
	"flag"
	"fmt"
	"log"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/goclient/client"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraphgoclient/geo"
)

var ip = flag.String("ip", "127.0.0.1:8080", "Port to communicate with server")

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()

	conn, err := grpc.Dial(*ip, grpc.WithInsecure())
	if err != nil {
		log.Fatal("DialTCPConnection")
	}
	defer conn.Close()

	c := graph.NewDgraphClient(conn)

	req := client.Req{}

	loc, err := geo.ValueFromJson(`{"type":"Point","coordinates":[-122.2207184,37.72129059]}`)
	if err != nil {
		log.Fatal(err)
	}

	if err := req.AddMutation(graph.NQuad{
		Subject:     "alice",
		Predicate:   "birthday",
		ObjectValue: loc,
	}, client.SET); err != nil {
		log.Fatal(err)
	}
	resp, err := c.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

	req = client.Req{}
	req.SetQuery("{ me(_xid_: alice) { birthday } }")
	resp, err = c.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

	fmt.Println(resp)
}
