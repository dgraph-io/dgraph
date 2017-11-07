package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

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
	Raw      []byte   `json:"raw_bytes",omitempty`
	Friends  []Person `json:"friend,omitempty"`
	Location loc      `json:"loc,omitempty"`
	School   []School `json:"school,omitempty"`
}

func Example_setObject() {
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
		CommitImmediately: true,
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
