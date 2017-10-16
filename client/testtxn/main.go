package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
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

	txn := dg.NewTxn()

	mu := &protos.Mutation{}
	mu.SetJson = []byte(`{"name": "Manish"}`)
	assigned, err := txn.Mutate(mu)
	if err != nil {
		log.Fatalf("Error while running mutation: %v\n", err)
	}
	fmt.Printf("Assigned: %+v\n", assigned)
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
	fmt.Printf("response: %q\n", resp.Json)
	txn.Commit()
}
