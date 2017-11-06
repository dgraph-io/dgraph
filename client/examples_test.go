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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

// TODO (pawan) - Change this to use SetObject
// func ExampleReq_SetQueryWithVariables() {
// 	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
// 	x.Checkf(err, "While trying to dial gRPC")
// 	defer conn.Close()
//
// 	clientDir, err := ioutil.TempDir("", "client_")
// 	x.Check(err)
// 	defer os.RemoveAll(clientDir)
//
// 	dgraphClient := client.NewDgraphClient(
// 		[]*grpc.ClientConn{conn}, client.DefaultOptions, clientDir)
//
// 	req := client.Req{}
//
// 	alice, err := dgraphClient.NodeXid("alice", false)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	e := alice.Edge("name")
// 	e.SetValueString("Alice")
// 	err = req.Set(e)
// 	x.Check(err)
//
// 	e = alice.Edge("falls.in")
// 	e.SetValueString("Rabbit hole")
// 	err = req.Set(e)
// 	x.Check(err)
//
// 	req.SetQuery(`mutation { schema { name: string @index(exact) . } }`)
// 	resp, err := dgraphClient.Run(context.Background(), &req)
// 	if err != nil {
// 		log.Fatalf("Error in getting response from server, %s", err)
// 	}
//
// 	req = client.Req{}
// 	variables := make(map[string]string)
// 	variables["$a"] = "Alice"
// 	req.SetQueryWithVariables(`{
// 		me(func: eq(name, $a)) {
// 			name
// 			falls.in
// 		}
// 	}`, variables)
//
// 	resp, err = dgraphClient.Run(context.Background(), &req)
// 	if err != nil {
// 		log.Fatalf("Error in getting response from server, %s", err)
// 	}
//
// 	type Alice struct {
// 		Name         string `json:"name"`
// 		WhatHappened string `json:"falls.in"`
// 	}
//
// 	type Res struct {
// 		Root Alice `json:"me"`
// 	}
//
// 	var r Res
// 	err = client.Unmarshal(resp.N, &r)
// 	x.Check(err)
// 	fmt.Printf("Alice: %+v\n\n", r.Root)
// 	err = dgraphClient.Close()
// 	x.Check(err)
// }

func ExampleDgraph_DropAll() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	clientDir, err := ioutil.TempDir("", "client_")
	x.Checkf(err, "While creating temp dir")
	defer os.RemoveAll(clientDir)

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	op := protos.Operation{
		DropAll: true,
	}
	ctx := context.Background()
	x.Check(dg.Alter(ctx, &op))
}

// TODO (pawan) - Change this to use SetObject
//func ExampleEdge_SetValueBytes() {
//	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
//	x.Checkf(err, "While trying to dial gRPC")
//	defer conn.Close()
//
//	clientDir, err := ioutil.TempDir("", "client_")
//	x.Check(err)
//	defer os.RemoveAll(clientDir)
//
//	dgraphClient := client.NewDgraphClient(
//		[]*grpc.ClientConn{conn}, client.DefaultOptions, clientDir)
//
//	req := client.Req{}
//
//	alice, err := dgraphClient.NodeBlank("alice")
//	if err != nil {
//		log.Fatal(err)
//	}
//	e := alice.Edge("name")
//	e.SetValueString("Alice")
//	err = req.Set(e)
//	x.Check(err)
//
//	e = alice.Edge("somestoredbytes")
//	err = e.SetValueBytes([]byte(`\xbd\xb2\x3d\xbc\x20\xe2\x8c\x98`))
//	x.Check(err)
//	err = req.Set(e)
//	x.Check(err)
//
//	req.SetQuery(`mutation {
//	schema {
//		name: string @index(exact) .
//	}
//}
//{
//	q(func: eq(name, "Alice")) {
//		name
//		somestoredbytes
//	}
//}`)
//
//	resp, err := dgraphClient.Run(context.Background(), &req)
//	if err != nil {
//		log.Fatalf("Error in getting response from server, %s", err)
//	}
//
//	type Alice struct {
//		Name      string `json:"name"`
//		ByteValue []byte `json:"somestoredbytes"`
//	}
//
//	type Res struct {
//		Root Alice `json:"q"`
//	}
//
//	var r Res
//	err = client.Unmarshal(resp.N, &r)
//	x.Check(err)
//	fmt.Printf("Alice: %+v\n\n", r.Root)
//	err = dgraphClient.Close()
//	x.Check(err)
//}

func ExampleUnmarshal() {
	type School struct {
		Name string `json:"name,omitempty"`
	}

	type Person struct {
		Uid     string   `json:"_uid_,omitempty"`
		Name    string   `json:"name,omitempty"`
		Age     int      `json:"age,omitempty"`
		Married bool     `json:"married,omitempty"`
		Raw     []byte   `json:"raw_bytes",omitempty`
		Friends []Person `json:"friend,omitempty"`
		School  []School `json:"school,omitempty"`
	}

	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	// While setting an object if a struct has a Uid then its properties in the graph are updated
	// else a new node is created.
	// In the example below new nodes for Alice and Charlie and school are created (since they dont
	// have a Uid).  Alice is also connected via the friend edge to an existing node with Uid
	// 1000(Bob).  We also set Name and Age values for this node with Uid 1000.

	p := Person{
		Name:    "Alice",
		Age:     26,
		Married: true,
		Raw:     []byte("raw_bytes"),
		Friends: []Person{{
			Uid:  "1000",
			Name: "Bob",
			Age:  24,
		}, {
			Name: "Charlie",
			Age:  29,
		}},
		School: []School{{
			Name: "Crown Public School",
		}},
	}

	op := &protos.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	txn := dg.NewTxn()
	mu := &protos.Mutation{}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	mu.CommitImmediately = true
	assigned, err := txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	// Assigned uids for nodes which were created would be returned in the resp.AssignedUids map.
	puid := assigned.Uids["blank-0"]
	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			_uid_
			name
			age
			loc
			raw_bytes
			married
			friend {
				_uid_
				name
				age
			}
			school {
				name
			}
		}
	}`, puid)

	resp, err := dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me []Person `json:"me"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Me: %+v\n", r.Me)
}

// func ExampleUnmarshal_facetsUpdate() {
// 	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
// 	x.Checkf(err, "While trying to dial gRPC")
// 	defer conn.Close()
//
// 	clientDir, err := ioutil.TempDir("", "client_")
// 	x.Check(err)
// 	defer os.RemoveAll(clientDir)
//
// 	dgraphClient := client.NewDgraphClient([]*grpc.ClientConn{conn})
//
// 	req := client.Req{}
//
// 	req.SetQuery(`
// mutation {
// 	schema {
// 		name: string @index .
// 	}
// 	set {
// 		_:person1 <name> "Alex" .
// 		_:person2 <name> "Beatie" .
// 		_:person3 <name> "Chris" .
// 		_:person4 <name> "David" .
//
// 		_:person1 <friend> _:person2 (close=true).
// 		_:person1 <friend> _:person3 (close=false).
// 		_:person1 <friend> _:person4 (close=true).
// 	}
// }
// {
// 	friends(func: eq(name, "Alex")) {
// 		_uid_
// 		name
// 		friend @facets {
// 			_uid_
// 			name
// 		}
// 	}
// }`)
//
// 	// Run the request in the Dgraph server.  The mutations are added, then
// 	// the query is exectuted.
// 	resp, err := dgraphClient.Run(context.Background(), &req)
// 	if err != nil {
// 		log.Fatalf("Error in getting response from server, %s", err)
// 	}
//
// 	// Unmarshal the response into a custom struct
//
// 	type friendFacets struct {
// 		Close bool `json:"close"`
// 	}
//
// 	// A type representing information in the graph.
// 	type person struct {
// 		ID           uint64        `json:"_uid_"` // record the UID for our update
// 		Name         string        `json:"name"`
// 		Friends      []*person     `json:"friend"` // Unmarshal with pointers to structs
// 		FriendFacets *friendFacets `json:"@facets"`
// 	}
//
// 	// A helper type matching the query root.
// 	type friends struct {
// 		Root person `json:"friends"`
// 	}
//
// 	var f friends
// 	err = client.Unmarshal(resp.N, &f)
// 	if err != nil {
// 		log.Fatal("Couldn't unmarshal response : ", err)
// 	}
//
// 	req = client.Req{}
//
// 	// Now update the graph.
// 	// for the close friends, add the reverse edge and note in a facet when we did this.
// 	for _, p := range f.Root.Friends {
// 		if p.FriendFacets.Close {
// 			n := dgraphClient.NodeUid(p.ID)
// 			e := n.ConnectTo("friend", dgraphClient.NodeUid(f.Root.ID))
// 			e.AddFacet("since", time.Now().Format(time.RFC3339))
// 			req.Set(e)
// 		}
// 	}
//
// 	resp, err = dgraphClient.Run(context.Background(), &req)
// 	if err != nil {
// 		log.Fatalf("Error in getting response from server, %s", err)
// 	}
//
// 	err = dgraphClient.Close()
// 	x.Check(err)
// }

// TODO - Change this to use SetObject
// func ExampleEdge_SetValueGeoJson() {
// 	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
// 	x.Checkf(err, "While trying to dial gRPC")
// 	defer conn.Close()
//
// 	clientDir, err := ioutil.TempDir("", "client_")
// 	x.Check(err)
// 	defer os.RemoveAll(clientDir)
//
// 	dgraphClient := client.NewDgraphClient(
// 		[]*grpc.ClientConn{conn}, client.DefaultOptions, clientDir)
//
// 	req := client.Req{}
//
// 	alice, err := dgraphClient.NodeBlank("alice")
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	e := alice.Edge("name")
// 	e.SetValueString("Alice")
// 	err = req.Set(e)
// 	x.Check(err)
//
// 	e = alice.Edge("loc")
// 	err = e.SetValueGeoJson(`{"Type":"Point", "Coordinates":[1.1,2.0]}`)
// 	x.Check(err)
// 	err = req.Set(e)
// 	x.Check(err)
//
// 	e = alice.Edge("city")
// 	err = e.SetValueGeoJson(`{
// 		"Type":"Polygon",
// 		"Coordinates":[[[0.0,0.0], [2.0,0.0], [2.0, 2.0], [0.0, 2.0], [0.0, 0.0]]]
// 	}`)
// 	x.Check(err)
// 	err = req.Set(e)
// 	x.Check(err)
//
// 	req.SetQuery(`mutation {
// 	schema {
// 		name: string @index(exact) .
// 	}
// }
// {
// 	q(func: eq(name, "Alice")) {
// 		name
// 		loc
// 		city
// 	}
// }`)
//
// 	resp, err := dgraphClient.Run(context.Background(), &req)
// 	if err != nil {
// 		log.Fatalf("Error in getting response from server, %s", err)
// 	}
//
// 	type Alice struct {
// 		Name string `json:"name"`
// 		Loc  []byte `json:"loc"`
// 		City []byte `json:"city"`
// 	}
//
// 	type Res struct {
// 		Root Alice `json:"q"`
// 	}
//
// 	var r Res
// 	err = client.Unmarshal(resp.N, &r)
// 	x.Check(err)
// 	fmt.Printf("Alice: %+v\n\n", r.Root)
// 	loc, err := wkb.Unmarshal(r.Root.Loc)
// 	x.Check(err)
// 	city, err := wkb.Unmarshal(r.Root.City)
// 	x.Check(err)
//
// 	fmt.Printf("Loc: %+v\n\n", loc)
// 	fmt.Printf("City: %+v\n\n", city)
// 	err = dgraphClient.Close()
// 	x.Check(err)
// }

func ExampleReq_SetObject() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type School struct {
		Name string `json:"name@en,omitempty"`
	}

	// If omitempty is not set, then edges with empty values (0 for int/float, "" for string, false
	// for bool) would be created for values not specified explicitly.

	type Person struct {
		Uid     string    `json:"_uid_,omitempty"`
		Name    string    `json:"name,omitempty"`
		Age     int       `json:"age,omitempty"`
		Married bool      `json:"married,omitempty"`
		Raw     []byte    `json:"raw_bytes",omitempty`
		Friends []Person  `json:"friend,omitempty"`
		School  []*School `json:"school,omitempty"`
	}

	// While setting an object if a struct has a Uid then its properties in the graph are updated
	// else a new node is created.
	// In the example below new nodes for Alice and Charlie and school are created (since they dont
	// have a Uid).  Alice is also connected via the friend edge to an existing node with Uid
	// 1000(Bob).  We also set Name and Age values for this node with Uid 1000.

	p := Person{
		Name:    "Alice",
		Age:     26,
		Married: true,
		Raw:     []byte("raw_bytes"),
		Friends: []Person{{
			Uid:  "1000",
			Name: "Bob",
			Age:  24,
		}, {
			Name: "Charlie",
			Age:  29,
		}},
		School: []*School{&School{
			Name: "Crown Public School",
		}},
	}

	op := &protos.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	txn := dg.NewTxn()
	mu := &protos.Mutation{}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	mu.CommitImmediately = true
	assigned, err := txn.Mutate(ctx, mu)

	// Assigned uids for nodes which were created would be returned in the resp.AssignedUids map.
	puid := assigned.Uids["blank-0"]
	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			_uid_
			name
			age
			loc
			raw_bytes
			married
			friend {
				_uid_
				name
				age
			}
			school {
				name@en
			}
		}
	}`, puid)

	resp, err := dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me []Person `json:"me"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Me: %+v\n", r.Me)
	// R.Me would be same as the person that we set above.
}

func ExampleReq_SetObject_facets(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	// This example shows example for SetObject using facets.
	type friendFacet struct {
		Since  time.Time `json:"since"`
		Family string    `json:"family"`
		Age    float64   `json:"age"`
		Close  bool      `json:"close"`
	}

	type nameFacets struct {
		Origin string `json:"origin"`
	}

	type schoolFacet struct {
		Since time.Time `json:"since"`
	}

	type School struct {
		Name   string      `json:"name"`
		Facets schoolFacet `json:"@facets"`
	}

	type Person struct {
		Name       string      `json:"name"`
		NameFacets nameFacets  `json:"name@facets"`
		Facets     friendFacet `json:"@facets"`
		Friends    []Person    `json:"friend"`
		School     School      `json:"school"`
	}

	ti := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	p := Person{
		Name: "Alice",
		NameFacets: nameFacets{
			Origin: "Indonesia",
		},
		Friends: []Person{
			Person{
				Name: "Bob",
				Facets: friendFacet{
					Since:  ti,
					Family: "yes",
					Age:    13,
					Close:  true,
				},
			},
			Person{
				Name: "Charlie",
				Facets: friendFacet{
					Family: "maybe",
					Age:    16,
				},
			},
		},
		School: School{
			Name: "Wellington School",
			Facets: schoolFacet{
				Since: ti,
			},
		},
	}

	ctx := context.Background()
	mu := &protos.Mutation{}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	mu.CommitImmediately = true
	assigned, err := dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	auid := assigned.Uids["blank-0"]

	q := fmt.Sprintf(`
    {

        me(func: uid(%v)) {
            name @facets
            friend @facets {
                name
            }
            school @facets {
                name
            }

        }
    }`, auid)

	resp, err := dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me Person `json:"me"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Me: %+v\n", r.Me)
}

func ExampleReq_SetObject_list(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)
	// This example shows example for SetObject for predicates with list type.
	type Person struct {
		Uid         string   `json:"_uid_"`
		Address     []string `json:"address"`
		PhoneNumber []int64  `json:"phone_number"`
	}

	p := Person{
		Address:     []string{"Redfern", "Riley Street"},
		PhoneNumber: []int64{9876, 123},
	}

	op := &protos.Operation{}
	op.Schema = `
		address: [string] .
		phone_number: [int] .
	`
	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	mu := &protos.Mutation{}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	mu.CommitImmediately = true
	assigned, err := dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	uid := assigned.Uids["blank-0"]

	q := fmt.Sprintf(`
	{
		me(func: uid(%s)) {
			_uid_
			address
			phone_number
		}
	}
	`, uid)

	resp, err := dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me []Person `json:"me"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Me: %+v\n", r.Me)
}

func ExampleReq_DeleteObject_edges() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type School struct {
		Uid  string `json:"_uid_"`
		Name string `json:"name@en,omitempty"`
	}

	type Person struct {
		Uid      string    `json:"_uid_,omitempty"`
		Name     string    `json:"name,omitempty"`
		Age      int       `json:"age,omitempty"`
		Married  bool      `json:"married,omitempty"`
		Friends  []Person  `json:"friend,omitempty"`
		Location string    `json:"loc,omitempty"`
		School   []*School `json:"school,omitempty"`
	}

	// Lets add some data first.
	p := Person{
		Uid:     "1000",
		Name:    "Alice",
		Age:     26,
		Married: true,
		Friends: []Person{{
			Uid:  "1001",
			Name: "Bob",
			Age:  24,
		}, {
			Uid:  "1002",
			Name: "Charlie",
			Age:  29,
		}},
		School: []*School{&School{
			Uid:  "1003",
			Name: "Crown Public School",
		}},
	}

	op := &protos.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	mu := &protos.Mutation{}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	mu.CommitImmediately = true
	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	q := fmt.Sprintf(`{
		me(func: uid(1000)) {
			_uid_
			name
			age
			loc
			married
			friend {
				_uid_
				name
				age
			}
			school {
				_uid_
				name@en
			}
		}

		me2(func: uid(1001)) {
			_uid_
			name
			age
		}

		me3(func: uid(1003)) {
			_uid_
			name@en
		}

		me4(func: uid(1002)) {
			_uid_
			name
			age
		}
	}`)

	resp, err := dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Now lets delete the edge between Alice and Bob.
	// Also lets delete the location for Alice.
	p2 := Person{
		Uid:      "1000",
		Location: "",
		Friends:  []Person{Person{Uid: "1001"}},
	}

	mu = &protos.Mutation{}
	pb, err = json.Marshal(p2)
	if err != nil {
		log.Fatal(err)
	}

	mu.DeleteJson = pb
	mu.CommitImmediately = true
	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	resp, err = dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me  []Person `json:"me"`
		Me2 []Person `json:"me2"`
		Me3 []School `json:"me3"`
		Me4 []Person `json:"me4"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	fmt.Printf("Resp: %+v\n", r)
}

func ExampleReq_DeleteObject_node() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	clientDir, err := ioutil.TempDir("", "client_")
	x.Check(err)
	defer os.RemoveAll(clientDir)

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	// In this test we check S * * deletion.
	type Person struct {
		Uid     uint64    `json:"_uid_,omitempty"`
		Name    string    `json:"name,omitempty"`
		Age     int       `json:"age,omitempty"`
		Married bool      `json:"married,omitempty"`
		Friends []*Person `json:"friend,omitempty"`
	}

	p := Person{
		Uid:     1000,
		Name:    "Alice",
		Age:     26,
		Married: true,
		Friends: []*Person{&Person{
			Uid:  1001,
			Name: "Bob",
			Age:  24,
		}, &Person{
			Uid:  1002,
			Name: "Charlie",
			Age:  29,
		}},
	}

	op := &protos.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	mu := &protos.Mutation{
		CommitImmediately: true,
	}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	q := fmt.Sprintf(`{
		me(func: uid(1000)) {
			_uid_
			name
			age
			married
			friend {
				_uid_
				name
				age
			}
		}

		me2(func: uid(1001)) {
			_uid_
			name
			age
		}

		me3(func: uid(1002)) {
			_uid_
			name
			age
		}
	}`)

	resp, err := dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me  Person `json:"me"`
		Me2 Person `json:"me2"`
		Me3 Person `json:"me3"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	fmt.Printf("Resp after SetObject: %+v\n", r)

	// Now lets try to delete Alice. This won't delete Bob and Charlie but just remove the
	// connection between Alice and them.
	p2 := Person{
		Uid: 1000,
	}

	mu = &protos.Mutation{
		CommitImmediately: true,
	}
	pb, err = json.Marshal(p2)
	if err != nil {
		log.Fatal(err)
	}

	mu.DeleteJson = pb
	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	resp, err = dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Resp after deleting node: %+v\n", r)
}

func ExampleReq_DeleteObject_predicate() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type Person struct {
		Uid     string   `json:"_uid_,omitempty"`
		Name    string   `json:"name,omitempty"`
		Age     int      `json:"age,omitempty"`
		Married bool     `json:"married,omitempty"`
		Friends []Person `json:"friend,omitempty"`
	}

	p := Person{
		Uid:     "1000",
		Name:    "Alice",
		Age:     26,
		Married: true,
		Friends: []Person{Person{
			Uid:  "1001",
			Name: "Bob",
			Age:  24,
		}, Person{
			Uid:  "1002",
			Name: "Charlie",
			Age:  29,
		}},
	}

	op := &protos.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	mu := &protos.Mutation{
		CommitImmediately: true,
	}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	q := fmt.Sprintf(`{
		me(func: uid(1000)) {
			_uid_
			name
			age
			married
			friend {
				_uid_
				name
				age
			}
		}
	}`)

	resp, err := dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me []Person `json:"me"`
	}
	var r Root
	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Response after SetObject: %+v\n\n", r)

	op = &protos.Operation{
		DropAttr: "friend",
	}
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	op.DropAttr = "married"
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	// Also lets run the query again to verify that predicate data was deleted.
	resp, err = dg.NewTxn().Query(ctx, q, nil)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}
	// Alice should have no friends and only two attributes now.
	fmt.Printf("Response after deletion: %+v\n", r)
}
