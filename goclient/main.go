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
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"context"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/goclient/client"
	"github.com/dgraph-io/dgraph/goclient/geo"
	"github.com/dgraph-io/dgraph/query/graph"
)

var ip = flag.String("ip", "127.0.0.1:8080", "Port to communicate with server")
var json = flag.String("json", "", "Json file to upload")

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()

	if *json != "" {
		uploadJSON(*json)
		return
	}

	conn, err := grpc.Dial(*ip, grpc.WithInsecure())
	if err != nil {
		log.Fatal("DialTCPConnection")
	}
	defer conn.Close()

	c := graph.NewDgraphClient(conn)

	req := client.NewRequest()
	if err := req.SetMutation("alice", "name", "", client.Str("Alice"), ""); err != nil {
		log.Fatal(err)
	}
	if err := req.SetMutation("alice", "falls.in", "", client.Str("rabbithole"), ""); err != nil {
		log.Fatal(err)
	}
	if err := req.SetMutation("alice", "age", "", client.Int(13), ""); err != nil {
		log.Fatal(err)
	}
	loc, err := geo.ValueFromJson(`{"type":"Point","coordinates":[-122.2207184,37.72129059]}`)
	if err != nil {
		log.Fatal(err)
	}
	if err := req.SetMutation("alice", "location", "", loc, ""); err != nil {
		log.Fatal(err)
	}

	resp, err := c.Query(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

	req = client.NewRequest()
	req.SetQuery("{ me(_xid_: alice) { name age falls.in location } }")
	resp, err = c.Query(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

	fmt.Println("alice", resp)

	req = client.NewRequest()
	if err := req.DelMutation("alice", "name", "", client.Str("Alice"), ""); err != nil {
		log.Fatal(err)
	}
	resp, err = c.Query(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}

	req = client.NewRequest()
	req.SetQuery("{ me(_xid_: alice) { name falls.in } }")
	resp, err = c.Query(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Println("alice", resp)
}

func uploadJSON(json string) {
	f, err := os.Open(json)
	if err != nil {
		log.Fatalf("Error opening file %s: %v", json, err)
	}
	defer f.Close()

	conn, err := grpc.Dial(*ip, grpc.WithInsecure())
	if err != nil {
		log.Fatal("DialTCPConnection")
	}
	defer conn.Close()

	var r io.Reader
	r = f
	c := graph.NewDgraphClient(conn)

	if strings.HasSuffix(json, ".gz") {
		r, err = gzip.NewReader(f)
		if err != nil {
			log.Fatalf("Error reading gzip file %s: %v", json, err)
		}
	}
	geo.Upload(c, r)
}
