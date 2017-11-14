package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

const targetAddr = "localhost:9081"

var insert = flag.Bool("add", false, "Insert")

func main() {
	flag.Parse()

	// Setup dgraph client
	ctx := context.Background()
	conn, err := grpc.Dial(targetAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	pc := protos.NewDgraphClient(conn)
	c := client.NewDgraphClient(pc)

	// Ingest
	if *insert {
		TestInsert3Quads(ctx, c)
	} else {
		TestQuery3Quads(ctx, c)
	}
}

func TestInsert3Quads(ctx context.Context, c *client.Dgraph) {
	// Set schema
	op := &protos.Operation{}
	op.Schema = `name: string @index(fulltext) .`
	x.Check(c.Alter(ctx, op))

	txn := c.NewTxn()

	mu := &protos.Mutation{}
	quad := &protos.NQuad{
		Subject:     "200",
		Predicate:   "name",
		ObjectValue: &protos.Value{&protos.Value_StrVal{"ok 200"}},
	}
	mu.Set = []*protos.NQuad{quad}
	_, err := txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}

	mu = &protos.Mutation{}
	quad = &protos.NQuad{
		Subject:     "300",
		Predicate:   "name",
		ObjectValue: &protos.Value{&protos.Value_StrVal{"ok 300"}},
	}
	mu.Set = []*protos.NQuad{quad}
	// mu.SetNquads = []byte(`<300> <name> "ok 300" .`)
	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}

	mu = &protos.Mutation{}
	quad = &protos.NQuad{
		Subject:     "400",
		Predicate:   "name",
		ObjectValue: &protos.Value{&protos.Value_StrVal{"ok 400"}},
	}
	mu.Set = []*protos.NQuad{quad}
	// mu.SetNquads = []byte(`<400> <name> "ok 400" .`)
	_, err = txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}

	x.Check(txn.Commit(ctx))
	fmt.Println("Commit OK")
}

func TestQuery3Quads(ctx context.Context, c *client.Dgraph) {
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
