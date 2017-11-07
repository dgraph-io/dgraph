package client_test

import (
	"context"
	"fmt"
	"log"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

type School struct {
	Name string `json:"name,omitempty"`
}

// If omitempty is not set, then edges with empty values (0 for int/float, "" for string, false
// for bool) would be created for values not specified explicitly.

type Person struct {
	Uid      uint64   `json:"uid,omitempty"`
	Name     string   `json:"name,omitempty"`
	Age      int      `json:"age,omitempty"`
	Married  bool     `json:"married,omitempty"`
	Raw      []byte   `json:"raw_bytes",omitempty`
	Friends  []Person `json:"friend,omitempty"`
	Location string   `json:"loc,omitempty"`
	School   School   `json:"school,omitempty"`
}

func Example_setObject() {
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := client.NewDgraphClient([]*grpc.ClientConn{conn})

	req := client.Req{}

	// While setting an object if a struct has a Uid then its properties in the graph are updated
	// else a new node is created.
	// In the example below new nodes for Alice and Charlie and school are created (since they dont
	// have a Uid).  Alice is also connected via the friend edge to an existing node with Uid
	// 1000(Bob).  We also set Name and Age values for this node with Uid 1000.

	loc := `{"type":"Point","coordinates":[1.1,2]}`
	p := Person{
		Name:     "Alice",
		Age:      26,
		Married:  true,
		Location: loc,
		Raw:      []byte("raw_bytes"),
		Friends: []Person{{
			Uid:  1000,
			Name: "Bob",
			Age:  24,
		}, {
			Name: "Charlie",
			Age:  29,
		}},
		School: School{
			Name: "Crown Public School",
		},
	}

	req.SetSchema(`
		age: int .
		married: bool .
	`)

	err = req.SetObject(&p)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatal(err)
	}

	// Assigned uids for nodes which were created would be returned in the resp.AssignedUids map.
	puid := resp.AssignedUids["blank-0"]
	q := fmt.Sprintf(`{
		me(func: uid(%d)) {
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

	req = client.Req{}
	req.SetQuery(q)
	resp, err = dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatal(err)
	}

	type Root struct {
		Me Person `json:"me"`
	}

	var r Root
	err = client.Unmarshal(resp.N, &r)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Me: %+v\n", r.Me)
	// R.Me would be same as the person that we set above.
}
