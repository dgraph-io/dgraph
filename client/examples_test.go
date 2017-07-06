/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
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

package client_test

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc"
)

func Node(val string, c *client.Dgraph) string {
	if uid, err := strconv.ParseUint(val, 0, 64); err == nil {
		return c.NodeUid(uid).String()
	}
	if strings.HasPrefix(val, "_:") {
		n, err := c.NodeBlank(val[2:])
		if err != nil {
			log.Fatal("Error while converting to node: %v", err)
		}
		return n.String()
	}
	n, err := c.NodeXid(val, false)
	if err != nil {
		log.Fatal("Error while converting to node: %v", err)
	}
	return n.String()
}

func ExampleReq_AddMutation() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts)

	req := client.Req{}
	person1, err := dgraphClient.NodeBlank("person1")
	if err != nil {
		log.Fatal(err)
	}

	// Creating a person node, and adding a name attribute to it.
	e := person1.Edge("name")
	e.SetValueString("Steven Spielberg")
	req.Set(e)
	e = person1.Edge("salary")
	e.SetValueFloat(13333.6161)
	req.Set(e)

	resp, err := dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_BatchMutation() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts)

	person1, err := dgraphClient.NodeBlank("person1")
	if err != nil {
		log.Fatal(err)
	}

	// Creating a person node, and adding a name attribute to it.
	e := person1.Edge("name")
	e.SetValueString("Steven Spielberg")
	dgraphClient.BatchSet(e)
	e = person1.Edge("salary")
	e.SetValueFloat(13333.6161)
	dgraphClient.BatchSet(e)

	dgraphClient.BatchEnd()
}

func ExampleReq_AddMutation_facets() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts)

	req := client.Req{}
	person1, err := dgraphClient.NodeXid("person1", false)
	if err != nil {
		log.Fatal(err)
	}
	e := person1.Edge("name")
	e.SetValueString("Steven Spielberg")
	e.AddFacet("since", "2006-01-02T15:04:05")
	e.AddFacet("alias", `"Steve"`)

	req.Set(e)

	person2, err := dgraphClient.NodeXid("person2", false)
	if err != nil {
		log.Fatal(err)
	}
	e = person2.Edge("name")
	e.SetValueString("William Jones")
	req.Set(e)

	e = person1.ConnectTo("friend", person2)
	e.AddFacet("close", "true")
	req.Set(e)

	req.SetQuery(`{
		me(id: person1) {
			name @facets
			friend @facets {
				name
			}
		}
	}`)

	resp, err := dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_AddMutation_schema() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts)

	req := client.Req{}
	// Doing mutation and setting schema, then getting schema.
	req.SetQuery(`
mutation {
 schema {
  name: string @index .
  release_date: date @index .
 }
}

schema {}
`)
	resp, err := dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_SetQuery() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts)

	req := client.Req{}
	alice, err := dgraphClient.NodeXid("alice", false)
	if err != nil {
		log.Fatal(err)
	}
	e := alice.Edge("name")
	e.SetValueString("Alice")
	req.Set(e)

	e = alice.Edge("falls.in")
	e.SetValueString("Rabbit hole")
	req.Set(e)

	req.SetQuery(`{
		me(id: alice) {
			name
			falls.in
		}
	}`)
	resp, err := dgraphClient.Run(context.Background(), &req)
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_BatchFlushing() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts)

	queryTXT := `{
	me(func: eq(_xid_, "http://someexample.com/StevenSpielberg")) {
		_xid_
		name @facets
		friend @facets {
			_xid_
			name
		}
	}
}`

	// set schema on _xid_ - Dgraph will do this automatically soon
	schemaUpdate, err := schema.Parse("_xid_: string @index(exact) .")
	if err != nil {
		log.Fatal("Error while parsing schema: ", err)
	}

	if err = dgraphClient.AddSchema(*schemaUpdate[0]); err != nil {    
		log.Fatal("While adding schema to batch ", err)
	}

	// --- request 1 ---
	req := client.Req{}
	person1, err := dgraphClient.NodeXid("http://someexample.com/StevenSpielberg", true)		// creates an XID batch in the background
	if err != nil {
		log.Fatal(err)
	}
	e := person1.Edge("name")
	e.SetValueString("Steven Spielberg")
	e.AddFacet("since", "2006-01-02T15:04:05")
	e.AddFacet("alias", `"Steve"`)

	req.Set(e)

	person2, err := dgraphClient.NodeBlank("person2")
	if err != nil {
		log.Fatal(err)
	}
	e = person2.Edge("name")
	e.SetValueString("William Jones")
	req.Set(e)

	e = person1.ConnectTo("friend", person2)
	e.AddFacet("close", "true")
	req.Set(e)

	req.SetQuery(queryTXT)

	dgraphClient.BatchFlush()					// flush the XIDs before query
	time.Sleep(10 * time.Second)				// workaround until index issue is fixed

	//fmt.Print("running request\n")
	resp, err := dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Println("-------- Query 1 --------")
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
	fmt.Println()
	fmt.Println()
	

	
	// --- request 2 ---
	req = client.Req{}
	dgraphClient.PurgeBlankNodes();		// so can reuse names
	stevenspielberg, err := dgraphClient.NodeXid("http://someexample.com/StevenSpielberg", true)	// retrieves existing XID
	stevenspalding, err := dgraphClient.NodeXid("http://someexample.com/StevenSpalding", true)		// creates an XID batch in the background
	if err != nil {
		log.Fatal(err)
	}
	e = stevenspalding.Edge("name")
	e.SetValueString("Steven Spalding")
	e.AddFacet("since", "2016-01-02T15:04:05")
	e.AddFacet("alias", `"Steve"`)

	req.Set(e)
	person2, err = dgraphClient.NodeBlank("person2")
	if err != nil {
		log.Fatal(err)
	}
	e = person2.Edge("name")
	e.SetValueString("William Jonhnson")
	req.Set(e)

	e = person1.ConnectTo("friend", person2)
	e.AddFacet("close", "true")
	req.Set(e)

	e = stevenspielberg.ConnectTo("friend", stevenspalding)
	req.Set(e)

	req.SetQuery(queryTXT)


	dgraphClient.BatchFlush()					// flush the XIDs before query


	time.Sleep(10 * time.Second)

	resp, err = dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Println("-------- Query 2 --------")
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))

	dgraphClient.BatchEnd()
}

func ExampleReq_SetQueryWithVariables() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts)

	req := client.Req{}
	variables := make(map[string]string)
	variables["$a"] = "3"
	req.SetQueryWithVariables(`
		query test ($a: int = 1) {
			me(id: 0x01) {
				name
				gender
				friend(first: $a) {
					name
				}
			}
		}`, variables)
	resp, err := dgraphClient.Run(context.Background(), &req)
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}
