package main

import (
	"bytes"
	"fmt"
	"log"
	"sync"

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

	TestTxnRead1(dg)
	TestTxnRead2(dg)
	TestTxnRead3(dg)
	TestTxnRead4(dg)
	TestConflict(dg)
}

// readTs == startTs
func TestTxnRead1(dg *client.Dgraph) {
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(mu)
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
	resp, err := txn.Query(q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish\"}]}}")))
	txn.Commit()
}

// readTs < commitTs
func TestTxnRead2(dg *client.Dgraph) {
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(mu)
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
	txn.Commit()

	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn2.Query(q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTruef(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[]}}")), "%s", resp.Json)
}

// readTs > commitTs
func TestTxnRead3(dg *client.Dgraph) {
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(mu)
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

	txn.Commit()
	txn = dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn.Query(q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish\"}]}}")))
}

// readTs > commitTs
func TestTxnRead4(dg *client.Dgraph) {
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(mu)
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

	txn.Commit()
	txn2 := dg.NewTxn()

	txn3 := dg.NewTxn()
	mu = &protos.Mutation{}
	mu.SetJson = []byte(fmt.Sprintf("{\"_uid_\": %d, \"name\": \"Manish2\"}", uid))
	assigned, err = txn3.Mutate(mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	txn3.Commit()

	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn2.Query(q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish\"}]}}")))

	txn4 := dg.NewTxn()
	q = fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err = txn4.Query(q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish2\"}]}}")))
}

func TestConflict(dg *client.Dgraph) {
	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(mu)
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
	assigned, err = txn2.Mutate(mu)
	x.AssertTrue(err != nil)

	txn.Commit()
	txn = dg.NewTxn()
	q := fmt.Sprintf(`{ me(func: uid(%d)) { name }}`, uid)
	resp, err := txn.Query(q, nil)
	if err != nil {
		log.Fatalf("Error while running query: %v\n", err)
	}
	x.AssertTrue(bytes.Equal(resp.Json, []byte("{\"data\": {\"me\":[{\"name\":\"Manish\"}]}}")))
}
