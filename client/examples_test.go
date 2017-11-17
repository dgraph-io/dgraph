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
	"log"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"google.golang.org/grpc"
)

func ExampleTxn_Query_variables() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type Person struct {
		Uid  string `json:"uid,omitempty"`
		Name string `json:"name,omitempty"`
	}

	op := &protos.Operation{}
	op.Schema = `
		name: string @index(exact) .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	p := Person{
		Name: "Alice",
	}

	mu := &protos.Mutation{
		CommitNow: true,
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

	variables := make(map[string]string)
	variables["$a"] = "Alice"
	q := `{
		me(func: eq(name, $a)) {
			name
		}
	}`

	resp, err := dg.NewTxn().QueryWithVars(ctx, q, variables)
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

func ExampleDgraph_Alter_dropAll() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	op := protos.Operation{
		DropAll: true,
	}
	ctx := context.Background()
	if err := dg.Alter(ctx, &op); err != nil {
		log.Fatal(err)
	}
}

func ExampleTxn_Mutate() {
	type School struct {
		Name string `json:"name,omitempty"`
	}

	type loc struct {
		Type   string    `json:"type,omitempty"`
		Coords []float64 `json:"coordinates,omitempty"`
	}

	// If omitempty is not set, then edges with empty values (0 for int/float, "" for string, false
	// for bool) would be created for values not specified explicitly.

	type Person struct {
		Uid      string   `json:"uid,omitempty"`
		Name     string   `json:"name,omitempty"`
		Age      int      `json:"age,omitempty"`
		Married  bool     `json:"married,omitempty"`
		Raw      []byte   `json:"raw_bytes,omitempty"`
		Friends  []Person `json:"friend,omitempty"`
		Location loc      `json:"loc,omitempty"`
		School   []School `json:"school,omitempty"`
	}

	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
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
		Location: loc{
			Type:   "Point",
			Coords: []float64{1.1, 2},
		},
		Raw: []byte("raw_bytes"),
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

	mu := &protos.Mutation{
		CommitNow: true,
	}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	assigned, err := dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	// Assigned uids for nodes which were created would be returned in the resp.AssignedUids map.
	puid := assigned.Uids["blank-0"]
	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			uid
			name
			age
			loc
			raw_bytes
			married
			friend {
				uid
				name
				age
			}
			school {
				name
			}
		}
	}`, puid)

	resp, err := dg.NewTxn().Query(ctx, q)
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

func ExampleTxn_Mutate_bytes() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type Person struct {
		Uid   string `json:"uid,omitempty"`
		Name  string `json:"name,omitempty"`
		Bytes []byte `json:"bytes,omitempty"`
	}

	op := &protos.Operation{}
	op.Schema = `
		name: string @index(exact) .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	p := Person{
		Name:  "Alice",
		Bytes: []byte("raw_bytes"),
	}

	mu := &protos.Mutation{
		CommitNow: true,
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

	q := `{
	q(func: eq(name, "Alice")) {
		uid
		name
		bytes
	}
}`

	resp, err := dg.NewTxn().Query(ctx, q)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me []Person `json:"q"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Me: %+v\n", r.Me)
}

func ExampleTxn_Query_unmarshal() {
	type School struct {
		Name string `json:"name,omitempty"`
	}

	type Person struct {
		Uid     string   `json:"uid,omitempty"`
		Name    string   `json:"name,omitempty"`
		Age     int      `json:"age,omitempty"`
		Married bool     `json:"married,omitempty"`
		Raw     []byte   `json:"raw_bytes,omitempty"`
		Friends []Person `json:"friend,omitempty"`
		School  []School `json:"school,omitempty"`
	}

	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
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
	mu.CommitNow = true
	assigned, err := txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	// Assigned uids for nodes which were created would be returned in the resp.AssignedUids map.
	puid := assigned.Uids["blank-0"]
	q := fmt.Sprintf(`{
		me(func: uid(%s)) {
			uid
			name
			age
			loc
			raw_bytes
			married
			friend {
				uid
				name
				age
			}
			school {
				name
			}
		}
	}`, puid)

	resp, err := dg.NewTxn().Query(ctx, q)
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

func ExampleTxn_Mutate_facets(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	// This example shows example for SetObject using facets.
	type School struct {
		Name  string    `json:"name,omitempty"`
		Since time.Time `json:"school:since,omitempty"`
	}

	type Person struct {
		Name       string   `json:"name,omitempty"`
		NameOrigin string   `json:"name|origin,omitempty"`
		Friends    []Person `json:"friend,omitempty"`

		// These are facets on the friend edge.
		Since  time.Time `json:"friend|since,omitempty"`
		Family string    `json:"friend|family,omitempty"`
		Age    float64   `json:"friend|age,omitempty"`
		Close  bool      `json:"friend|close,omitempty"`

		School []School `json:"school,omitempty"`
	}

	ti := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	p := Person{
		Name:       "Alice",
		NameOrigin: "Indonesia",
		Friends: []Person{
			Person{
				Name:   "Bob",
				Since:  ti,
				Family: "yes",
				Age:    13,
				Close:  true,
			},
			Person{
				Name:   "Charlie",
				Family: "maybe",
				Age:    16,
			},
		},
		School: []School{School{
			Name:  "Wellington School",
			Since: ti,
		}},
	}

	ctx := context.Background()
	mu := &protos.Mutation{}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	mu.CommitNow = true
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

	resp, err := dg.NewTxn().Query(ctx, q)
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

func ExampleTxn_Mutate_list(t *testing.T) {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)
	// This example shows example for SetObject for predicates with list type.
	type Person struct {
		Uid         string   `json:"uid"`
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
	mu.CommitNow = true
	assigned, err := dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	uid := assigned.Uids["blank-0"]

	q := fmt.Sprintf(`
	{
		me(func: uid(%s)) {
			uid
			address
			phone_number
		}
	}
	`, uid)

	resp, err := dg.NewTxn().Query(ctx, q)
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

func ExampleTxn_Mutate_delete() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type School struct {
		Uid  string `json:"uid"`
		Name string `json:"name@en,omitempty"`
	}

	type Person struct {
		Uid      string    `json:"uid,omitempty"`
		Name     string    `json:"name,omitempty"`
		Age      int       `json:"age,omitempty"`
		Married  bool      `json:"married,omitempty"`
		Friends  []Person  `json:"friend,omitempty"`
		Location string    `json:"loc,omitempty"`
		School   []*School `json:"school,omitempty"`
	}

	// Lets add some data first.
	p := Person{
		Uid:      "1000",
		Name:     "Alice",
		Age:      26,
		Married:  true,
		Location: "Riley Street",
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
	mu.CommitNow = true
	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	q := fmt.Sprintf(`{
		me(func: uid(1000)) {
			uid
			name
			age
			loc
			married
			friend {
				uid
				name
				age
			}
			school {
				uid
				name@en
			}
		}

		me2(func: uid(1001)) {
			uid
			name
			age
		}

		me3(func: uid(1003)) {
			uid
			name@en
		}

		me4(func: uid(1002)) {
			uid
			name
			age
		}
	}`)

	resp, err := dg.NewTxn().Query(ctx, q)
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
	mu.CommitNow = true
	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	resp, err = dg.NewTxn().Query(ctx, q)
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

func ExampleTxn_Mutate_deleteNode() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	// In this test we check S * * deletion.
	type Person struct {
		Uid     string    `json:"uid,omitempty"`
		Name    string    `json:"name,omitempty"`
		Age     int       `json:"age,omitempty"`
		Married bool      `json:"married,omitempty"`
		Friends []*Person `json:"friend,omitempty"`
	}

	p := Person{
		Uid:     "1000",
		Name:    "Alice",
		Age:     26,
		Married: true,
		Friends: []*Person{&Person{
			Uid:  "1001",
			Name: "Bob",
			Age:  24,
		}, &Person{
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
		CommitNow: true,
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
			uid
			name
			age
			married
			friend {
				uid
				name
				age
			}
		}

		me2(func: uid(1001)) {
			uid
			name
			age
		}

		me3(func: uid(1002)) {
			uid
			name
			age
		}
	}`)

	resp, err := dg.NewTxn().Query(ctx, q)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me  []Person `json:"me"`
		Me2 []Person `json:"me2"`
		Me3 []Person `json:"me3"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	fmt.Printf("Resp after SetObject: %+v\n", r)

	// Now lets try to delete Alice. This won't delete Bob and Charlie but just remove the
	// connection between Alice and them.
	p2 := Person{
		Uid: "1000",
	}

	mu = &protos.Mutation{
		CommitNow: true,
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

	resp, err = dg.NewTxn().Query(ctx, q)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Resp after deleting node: %+v\n", r)
}

func ExampleTxn_Mutate_deletePredicate() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := protos.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type Person struct {
		Uid     string   `json:"uid,omitempty"`
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
		CommitNow: true,
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
			uid
			name
			age
			married
			friend {
				uid
				name
				age
			}
		}
	}`)

	resp, err := dg.NewTxn().Query(ctx, q)
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
	resp, err = dg.NewTxn().Query(ctx, q)
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
