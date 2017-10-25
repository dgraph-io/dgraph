package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.Dial("localhost:8888", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	zero := protos.NewZeroClient(conn)

	conn, err = grpc.Dial("localhost:9080", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	dc := protos.NewDgraphClient(conn)

	dg := client.NewDgraphClient(zero, dc)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			dg.NewTxn()
			wg.Done()
		}()
	}
	wg.Wait()

	ctx := context.Background()
	TestTxnRead1(ctx, dg)
	TestTxnRead2(ctx, dg)
	TestTxnRead3(ctx, dg)
	TestTxnRead4(ctx, dg)
	TestConflict(ctx, dg)
	TestConflictTimeout(ctx, dg)
	TestConflictTimeout2(ctx, dg)
}

// readTs == startTs
func TestTxnRead1(ctx context.Context, dg *client.Dgraph) {
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid uint64
	for _, u := range assigned.Uids {
		uid = u
	}

	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn.Query(ctx, q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish\"}]}}")))
	x.Check(txn.Commit(ctx))
}

// readTs < commitTs
func TestTxnRead2(ctx context.Context, dg *client.Dgraph) {
	fmt.Println("TestTxnRead2")
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid uint64
	for _, u := range assigned.Uids {
		uid = u
	}

	txn2 := dg.NewTxn()
	x.Check(txn.Commit(ctx))

	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn2.Query(ctx, q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTruef(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[]}}")), "%s", resp.Json)
}

// readTs > commitTs
func TestTxnRead3(ctx context.Context, dg *client.Dgraph) {
	fmt.Println("TestTxnRead3")
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(context.Background(), mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid uint64
	for _, u := range assigned.Uids {
		uid = u
	}

	x.Check(txn.Commit(ctx))
	txn = dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn.Query(ctx, q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish\"}]}}")))
}

// readTs > commitTs
func TestTxnRead4(ctx context.Context, dg *client.Dgraph) {
	fmt.Println("TestTxnRead4")
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid uint64
	for _, u := range assigned.Uids {
		uid = u
	}

	x.Check(txn.Commit(ctx))
	txn2 := dg.NewTxn()

	txn3 := dg.NewTxn()
	mu = &protos.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf("{\"_uid_\": %d, \"name\": \"Manish2\"}", uid))
	assigned, err = txn3.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	fmt.Println("Committing txn3")
	x.Check(txn3.Commit(ctx))

	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn2.Query(ctx, q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish\"}]}}")))

	txn4 := dg.NewTxn()
	q = fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err = txn4.Query(ctx, q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish2\"}]}}")))
}

func TestConflict(ctx context.Context, dg *client.Dgraph) {
	fmt.Println("TestConflict")
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(ctx, mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	if len(assigned.Uids) != 1 {
		log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
	}
	var uid uint64
	for _, u := range assigned.Uids {
		uid = u
	}

	txn2 := dg.NewTxn()
	mu = &protos.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf("{\"_uid_\": %d, \"name\": \"Manish\"}", uid))
	assigned, err = txn2.Mutate(ctx, mu)
	x.AssertTrue(err != nil)

	x.Check(txn.Commit(ctx))
	txn = dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn.Query(ctx, q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	fmt.Printf("Response JSON: %q\n", resp.Json)
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish\"}]}}")))
}

func TestConflictTimeout(ctx context.Context, dg *client.Dgraph) {
	fmt.Println("TestConflictTimeout")
	var uid uint64
	{
		txn := dg.NewTxn()

		mu := &protos.Mutation{}
		mu.SetJson = []byte(`{"name": "Manish"}`)
		assigned, err := txn.Mutate(ctx, mu)
		if err != nil {
			log.Fatalf("Error while running mutation: %v\n", err)
		}
		if len(assigned.Uids) != 1 {
			log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
		}
		for _, u := range assigned.Uids {
			uid = u
		}
	}

	txn2 := dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn2.Query(ctx, q, nil)
	x.Check(err)

	resp, err = txn2.Query(ctx, q, nil)
	x.Check(err)
	fmt.Printf("Response should be empty. JSON: %q\n", resp.Json)

	mu := &protos.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf("{\"_uid_\": %d, \"name\": \"Jan the man\"}", uid))
	_, err = txn2.Mutate(ctx, mu)
	fmt.Printf("txn2.mutate error: %v\n", err)
	if err == nil {
		x.Check(txn2.Commit(ctx))
	}

	// err = txn.Commit()
	// fmt.Printf("This txn should fail with error. Err got: %v\n", err)

	txn3 := dg.NewTxn()
	q = fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err = txn3.Query(ctx, q, nil)
	x.Check(err)
	fmt.Printf("Final Response JSON: %q\n", resp.Json)
}

func TestConflictTimeout2(ctx context.Context, dg *client.Dgraph) {
	fmt.Println("TestConflictTimeout2")
	var uid uint64
	{
		txn := dg.NewTxn()

		mu := &protos.Mutation{}
		mu.SetJson = []byte(`{"name": "Manish"}`)
		assigned, err := txn.Mutate(ctx, mu)
		if err != nil {
			log.Fatalf("Error while running mutation: %v\n", err)
		}
		if len(assigned.Uids) != 1 {
			log.Fatalf("Error. Nothing assigned. %+v\n", assigned)
		}
		for _, u := range assigned.Uids {
			uid = u
		}
	}

	txn2 := dg.NewTxn()
	mu := &protos.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf("{\"_uid_\": %d, \"name\": \"Jan the man\"}", uid))
	_, err := txn2.Mutate(ctx, mu)
	fmt.Printf("This txn should fail with error. Err got: %v\n", err)
	x.AssertTrue(err != nil)

	time.Sleep(time.Second * 15)

	txn3 := dg.NewTxn()
	mu = &protos.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf("{\"_uid_\": %d, \"name\": \"Jan the man\"}", uid))
	assigned, err := txn3.Mutate(ctx, mu)
	fmt.Printf("txn2.mutate error: %v\n", err)
	if err == nil {
		x.Check(txn3.Commit(ctx))
	}
	for _, u := range assigned.Uids {
		uid = u
	}

	txn4 := dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn4.Query(ctx, q, nil)
	x.Check(err)
	fmt.Printf("Final Response JSON: %q\n", resp.Json)
}
