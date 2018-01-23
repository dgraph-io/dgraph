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
	"os/exec"
	"testing"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

func TestMain(m *testing.M) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	zw, err := ioutil.TempDir("", "")
	x.Check(err)

	cmd := exec.Command("go", "install", "github.com/dgraph-io/dgraph/dgraph")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Fatalf("Could not run %q: %s", cmd.Args, string(out))
	}
	zero := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"zero",
		"-w", zw,
		"-o", "-1999",
	)
	zero.Stdout = os.Stdout
	zero.Stderr = os.Stdout
	x.Check(zero.Start())

	p, err := ioutil.TempDir("", "")
	x.Check(err)
	w, err := ioutil.TempDir("", "")
	x.Check(err)

	server := exec.Command(os.ExpandEnv("$GOPATH/bin/dgraph"),
		"server",
		"-w", w,
		"-p", p,
		"--zero", "127.0.0.1:5081",
		"--memory_mb", "2048",
	)
	server.Stdout = os.Stdout
	server.Stderr = os.Stdout
	x.Check(server.Start())
	// Wait for servers to start and connect.
	time.Sleep(5 * time.Second)
	s := m.Run()

	x.Check(zero.Process.Kill())
	x.Check(server.Process.Kill())
	x.Check(os.RemoveAll(zw))
	x.Check(os.RemoveAll(w))
	x.Check(os.RemoveAll(p))
	os.Exit(s)
}

func ExampleDgraph_Alter_dropAll() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	op := api.Operation{
		DropAll: true,
	}
	ctx := context.Background()
	if err := dg.Alter(ctx, &op); err != nil {
		log.Fatal(err)
	}

	fmt.Println(err)

	// Output: <nil>
}

func ExampleTxn_Query_variables() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type Person struct {
		Uid  string `json:"uid,omitempty"`
		Name string `json:"name,omitempty"`
	}

	op := &api.Operation{}
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

	mu := &api.Mutation{
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
	q := `query Alice($a: string){
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

	fmt.Println(string(resp.Json))
	// Output: {"me":[{"name":"Alice"}]}
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

	dc := api.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	// While setting an object if a struct has a Uid then its properties in the graph are updated
	// else a new node is created.
	// In the example below new nodes for Alice, Bob and Charlie and school are created (since they
	// dont have a Uid).
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

	op := &api.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	mu := &api.Mutation{
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
	const q = `query Me($id: string){
		me(func: uid($id)) {
			name
			age
			loc
			raw_bytes
			married
			friend @filter(eq(name, "Bob")) {
				name
				age
			}
			school {
				name
			}
		}
	}`

	variables := make(map[string]string)
	variables["$id"] = puid
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

	// R.Me would be same as the person that we set above.
	// fmt.Printf("Me: %+v\n", r.Me)

	fmt.Println(string(resp.Json))
	// Output: {"me":[{"name":"Alice","age":26,"loc":{"type":"Point","coordinates":[1.1,2]},"raw_bytes":"cmF3X2J5dGVz","married":true,"friend":[{"name":"Bob","age":24}],"school":[{"name":"Crown Public School"}]}]}

}

func ExampleTxn_Mutate_bytes() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type Person struct {
		Uid   string `json:"uid,omitempty"`
		Name  string `json:"name,omitempty"`
		Bytes []byte `json:"bytes,omitempty"`
	}

	op := &api.Operation{}
	op.Schema = `
		name: string @index(exact) .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	p := Person{
		Name:  "Alice-new",
		Bytes: []byte("raw_bytes"),
	}

	mu := &api.Mutation{
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
	q(func: eq(name, "Alice-new")) {
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

	// Output: Me: [{Uid: Name:Alice-new Bytes:[114 97 119 95 98 121 116 101 115]}]
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

	dc := api.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	op := &api.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	p := Person{
		Name: "Bob",
		Age:  24,
	}

	txn := dg.NewTxn()
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu := &api.Mutation{
		CommitNow: true,
		SetJson:   pb,
	}
	assigned, err := txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}
	bob := assigned.Uids["blank-0"]

	// While setting an object if a struct has a Uid then its properties in the graph are updated
	// else a new node is created.
	// In the example below new nodes for Alice and Charlie and school are created (since they dont
	// have a Uid).  Alice is also connected via the friend edge to an existing node Bob.
	p = Person{
		Name:    "Alice",
		Age:     26,
		Married: true,
		Raw:     []byte("raw_bytes"),
		Friends: []Person{{
			Uid: bob,
		}, {
			Name: "Charlie",
			Age:  29,
		}},
		School: []School{{
			Name: "Crown Public School",
		}},
	}

	txn = dg.NewTxn()
	mu = &api.Mutation{}
	pb, err = json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	mu.CommitNow = true
	assigned, err = txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	// Assigned uids for nodes which were created would be returned in the resp.AssignedUids map.
	puid := assigned.Uids["blank-0"]
	variables := make(map[string]string)
	variables["$id"] = puid
	const q = `query Me($id: string){
		me(func: uid($id)) {
			name
			age
			loc
			raw_bytes
			married
			friend @filter(eq(name, "Bob")) {
				name
				age
			}
			school {
				name
			}
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

	fmt.Println(string(resp.Json))
	// Output: {"me":[{"name":"Alice","age":26,"raw_bytes":"cmF3X2J5dGVz","married":true,"friend":[{"name":"Bob","age":24}],"school":[{"name":"Crown Public School"}]}]}
}

func ExampleTxn_Mutate_facets() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	// Doing a dropAll isn't required by the user. We do it here so that we can verify that the
	// example runs as expected.
	op := api.Operation{
		DropAll: true,
	}
	ctx := context.Background()
	if err := dg.Alter(ctx, &op); err != nil {
		log.Fatal(err)
	}

	op = api.Operation{}
	op.Schema = `
		name: string @index(exact) .
	`

	err = dg.Alter(ctx, &op)
	if err != nil {
		log.Fatal(err)
	}

	// This example shows example for SetObject using facets.
	type School struct {
		Name  string    `json:"name,omitempty"`
		Since time.Time `json:"school|since,omitempty"`
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

	mu := &api.Mutation{}
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
	variables := make(map[string]string)
	variables["$id"] = auid

	const q = `query Me($id: string){
        me(func: uid($id)) {
            name @facets
			friend @filter(eq(name, "Bob")) @facets {
                name
            }
            school @facets {
                name
            }

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
	// Output: Me: [{Name:Alice NameOrigin:Indonesia Friends:[{Name:Bob NameOrigin: Friends:[] Since:2009-11-10 23:00:00 +0000 UTC Family:yes Age:13 Close:true School:[]}] Since:0001-01-01 00:00:00 +0000 UTC Family: Age:0 Close:false School:[{Name:Wellington School Since:2009-11-10 23:00:00 +0000 UTC}]}]
}

func ExampleTxn_Mutate_list() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
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

	op := &api.Operation{}
	op.Schema = `
		address: [string] .
		phone_number: [int] .
	`
	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	mu := &api.Mutation{}
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

	variables := map[string]string{"$id": assigned.Uids["blank-0"]}
	const q = `
	query Me($id: string){
		me(func: uid($id)) {
			address
			phone_number
		}
	}
	`

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

	fmt.Println(string(resp.Json))
	// Output: {"me":[{"address":["Riley Street","Redfern"],"phone_number":[9876,123]}]}

}

func ExampleDeleteEdges() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	op := &api.Operation{}
	op.Schema = `
			age: int .
			married: bool .
		`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	type School struct {
		Uid  string `json:"uid"`
		Name string `json:"name@en,omitempty"`
	}

	type Person struct {
		Uid      string    `json:"uid,omitempty"`
		Name     string    `json:"name,omitempty"`
		Age      int       `json:"age,omitempty"`
		Married  bool      `json:"married,omitempty"`
		Friends  []Person  `json:"friends,omitempty"`
		Location string    `json:"loc,omitempty"`
		Schools  []*School `json:"schools,omitempty"`
	}

	// Lets add some data first.
	p := Person{
		Name:     "Alice",
		Age:      26,
		Married:  true,
		Location: "Riley Street",
		Friends: []Person{{
			Name: "Bob",
			Age:  24,
		}, {
			Name: "Charlie",
			Age:  29,
		}},
		Schools: []*School{&School{
			Name: "Crown Public School",
		}},
	}

	mu := &api.Mutation{}
	pb, err := json.Marshal(p)
	if err != nil {
		log.Fatal(err)
	}

	mu.SetJson = pb
	mu.CommitNow = true
	mu.IgnoreIndexConflict = true
	assigned, err := dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	alice := assigned.Uids["blank-0"]

	variables := make(map[string]string)
	variables["$alice"] = alice
	const q = `query Me($alice: string){
		me(func: uid($alice)) {
			name
			age
			loc
			married
			friends {
				name
				age
			}
			schools {
				name@en
			}
		}
	}`

	resp, err := dg.NewTxn().QueryWithVars(ctx, q, variables)
	if err != nil {
		log.Fatal(err)
	}

	// Now lets delete the friend and location edge from Alice
	mu = &api.Mutation{}
	client.DeleteEdges(mu, alice, "friends", "loc")

	mu.CommitNow = true
	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	resp, err = dg.NewTxn().QueryWithVars(ctx, q, variables)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me []Person `json:"me"`
	}

	var r Root
	err = json.Unmarshal(resp.Json, &r)
	fmt.Println(string(resp.Json))
	// Output: {"me":[{"name":"Alice","age":26,"married":true,"schools":[{"name@en":"Crown Public School"}]}]}
}

func ExampleTxn_Mutate_deleteNode() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
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
		Name:    "Alice",
		Age:     26,
		Married: true,
		Friends: []*Person{&Person{
			Name: "Bob",
			Age:  24,
		}, &Person{
			Name: "Charlie",
			Age:  29,
		}},
	}

	op := &api.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	mu := &api.Mutation{
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

	alice := assigned.Uids["blank-0"]
	bob := assigned.Uids["blank-1"]
	charlie := assigned.Uids["blank-2"]

	variables := make(map[string]string)
	variables["$alice"] = alice
	variables["$bob"] = bob
	variables["$charlie"] = charlie
	const q = `query Me($alice: string, $bob: string, $charlie: string){
		me(func: uid($alice)) {
			name
			age
			married
			friend {
				uid
				name
				age
			}
		}

		me2(func: uid($bob)) {
			name
			age
		}

		me3(func: uid($charlie)) {
			name
			age
		}
	}`

	resp, err := dg.NewTxn().QueryWithVars(ctx, q, variables)
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

	// Now lets try to delete Alice. This won't delete Bob and Charlie but just remove the
	// connection between Alice and them.

	// The JSON for deleting a node should be of the form {"uid": "0x123"}. If you wanted to
	// delete multiple nodes you could supply an array of objects like [{"uid": "0x321"}, {"uid":
	// "0x123"}] to DeleteJson.

	d := map[string]string{"uid": alice}
	pb, err = json.Marshal(d)
	if err != nil {
		log.Fatal(err)
	}

	mu = &api.Mutation{
		CommitNow:  true,
		DeleteJson: pb,
	}

	_, err = dg.NewTxn().Mutate(ctx, mu)
	if err != nil {
		log.Fatal(err)
	}

	resp, err = dg.NewTxn().QueryWithVars(ctx, q, variables)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Resp after deleting node: %+v\n", string(resp.Json))
	// Output: Resp after deleting node: {"me":[],"me2":[{"name":"Bob","age":24}],"me3":[{"name":"Charlie","age":29}]}
}

func ExampleTxn_Mutate_deletePredicate() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal("While trying to dial gRPC")
	}
	defer conn.Close()

	dc := api.NewDgraphClient(conn)
	dg := client.NewDgraphClient(dc)

	type Person struct {
		Uid     string   `json:"uid,omitempty"`
		Name    string   `json:"name,omitempty"`
		Age     int      `json:"age,omitempty"`
		Married bool     `json:"married,omitempty"`
		Friends []Person `json:"friend,omitempty"`
	}

	p := Person{
		Name:    "Alice",
		Age:     26,
		Married: true,
		Friends: []Person{Person{
			Name: "Bob",
			Age:  24,
		}, Person{
			Name: "Charlie",
			Age:  29,
		}},
	}

	op := &api.Operation{}
	op.Schema = `
		age: int .
		married: bool .
	`

	ctx := context.Background()
	err = dg.Alter(ctx, op)
	if err != nil {
		log.Fatal(err)
	}

	mu := &api.Mutation{
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

	alice := assigned.Uids["blank-0"]

	variables := make(map[string]string)
	variables["$id"] = alice
	const q = `query Me($id: string){
		me(func: uid($id)) {
			name
			age
			married
			friend {
				uid
				name
				age
			}
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

	op = &api.Operation{
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
	resp, err = dg.NewTxn().QueryWithVars(ctx, q, variables)
	if err != nil {
		log.Fatal(err)
	}

	r = Root{}
	err = json.Unmarshal(resp.Json, &r)
	if err != nil {
		log.Fatal(err)
	}

	// Alice should have no friends and only two attributes now.
	fmt.Printf("Response after deletion: %+v\n", r)
	// Output: Response after deletion: {Me:[{Uid: Name:Alice Age:26 Married:false Friends:[]}]}
}
