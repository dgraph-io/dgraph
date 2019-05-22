/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/x"
	"google.golang.org/grpc"
)

var alpha = flag.String("alpha", "localhost:9080", "Dgraph alpha addr")
var insert = flag.Bool("add", false, "Insert")

func main() {
	flag.Parse()

	// Setup dgraph client
	ctx := context.Background()
	conn, err := grpc.Dial(*alpha, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	pc := api.NewDgraphClient(conn)
	c := dgo.NewDgraphClient(pc)

	// Ingest
	if *insert {
		TestInsert3Quads(ctx, c)
	} else {
		TestQuery3Quads(ctx, c)
	}
}

func TestInsert3Quads(ctx context.Context, c *dgo.Dgraph) {
	// Set schema
	op := &api.Operation{}
	op.Schema = `name: string @index(fulltext) .`
	x.Check(c.Alter(ctx, op))

	txn := c.NewTxn()

	mu := &api.Mutation{}
	quad := &api.NQuad{
		Subject:     "200",
		Predicate:   "name",
		ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "ok 200"}},
	}
	mu.Set = []*api.NQuad{quad}
	_, err := txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}

	mu = &api.Mutation{}
	quad = &api.NQuad{
		Subject:     "300",
		Predicate:   "name",
		ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "ok 300"}},
	}
	mu.Set = []*api.NQuad{quad}
	// mu.SetNquads = []byte(`<300> <name> "ok 300" .`)
	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}

	mu = &api.Mutation{}
	quad = &api.NQuad{
		Subject:     "400",
		Predicate:   "name",
		ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "ok 400"}},
	}
	mu.Set = []*api.NQuad{quad}
	// mu.SetNquads = []byte(`<400> <name> "ok 400" .`)
	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}

	x.Check(txn.Commit(ctx))
	fmt.Println("Commit OK")
}

func TestQuery3Quads(ctx context.Context, c *dgo.Dgraph) {
	txn := c.NewTxn()
	q := fmt.Sprint(`{ me(func: uid(200, 300, 400)) { name }}`)
	resp, err := txn.Query(ctx, q)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"me\":[{\"name\":\"ok 200\"},{\"name\":\"ok 300\"},{\"name\":\"ok 400\"}]}")))
	x.AssertTrue(resp.Txn.StartTs > 0)
	x.Check(txn.Commit(ctx))
}
