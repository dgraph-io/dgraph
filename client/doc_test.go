package client

import (
	"context"
	"log"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/x"
	"google.golang.org/grpc"
)

func ExampleBatchMutation() {
	// Make a connection to Dgraph.
	conn, err := grpc.Dial("127.0.0.1:8080", grpcWithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	// Get a dgraph client.
	dgraphClient := graphp.NewDgraphClient(conn)

	// Start a new batch with batch size 1000 and 100 concurrent requests.
	batch := client.NewBatchMutation(context.Background(), dgraphClient, 1000, 100)

	// Process your file, convert data to a graphp.NQuad and add it to the batch.
	// For each graph.NQuad , run batch.AddMutation
	if err = batch.AddMutation(nquad, client.SET); err != nil {
		log.Fatal("While adding mutation to batch: ", err)
	}

	// Wait for all requests to complete.
	batch.Flush()
}

func ExampleReq_AddMutation() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpcWithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	// Get a dgraph client.
	dgraphClient := graphp.NewDgraphClient(conn)

	req := client.Req{}
	// Creating a person node, and adding a name attribute to it.
	nq := graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "name",
	}

	// Str is a helper function to add a string value.
	client.Str("Steven Spielberg", &nq)
	// Adding a new mutation.
	if err := req.AddMutation(nq, client.SET); err != nil {
		// handle error
	}
	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "salary",
	}
	// Float is used to floating values.
	if err = client.Float(13333.6161, &nq); err != nil {
		log.Fatal(err)
	}

	// Run the request and get the response.
	resp, err = dgraphClient.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
}

func ExampleReq_SetQuery() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpcWithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	// Get a dgraph client.
	dgraphClient := graphp.NewDgraphClient(conn)

	req := client.Req{}
	req.SetQuery(`{
		me(id: alice) {
			name
			falls.in
		}
	}`)
	resp, err := dgraphClient.Run(context.Background(), req.Request())
	// Check response and handle errors
}

func ExampleReq() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpcWithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	// Get a dgraph client.
	dgraphClient := graphp.NewDgraphClient(conn)

	req := client.Req{}
	// Creating a person node, and adding a name attribute to it.
	nq := graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "name",
	}

	// Str is a helper function to add a string value.
	client.Str("Steven Spielberg", &nq)
	// Adding a new mutation.
	if err := req.AddMutation(nq, client.SET); err != nil {
		// handle error
	}
	// Lets create another person and add a name for it.
	nq = graphp.NQuad{
		Subject:   "_:person2",
		Predicate: "name",
	}
	client.Str("William Jones", &nq)
	// Adding a new mutation.
	req.AddMutation(nq, client.SET)

	// Lets connect the two nodes together.
	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "friend",
		ObjectId:  "_:person2",
	}
	req.AddMutation(nq, client.SET)

	// One query and multiple mutations can be sent as part of the same request.
	req.SetQuery(`{
		me(id: alice) {
			name
			falls.in
		}
	}`)
	resp, err := dgraphClient.Run(context.Background(), req.Request())

}
