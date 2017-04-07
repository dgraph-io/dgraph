package client_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc"
)

func ExampleBatchMutation() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := graphp.NewDgraphClient(conn)

	// Start a new batch with batch size 1000 and 100 concurrent requests.
	batch := client.NewBatchMutation(context.Background(), dgraphClient, 1000, 100)

	// Process your file, convert data to a graphp.NQuad and add it to the batch.
	// For each graph.NQuad, run batch.AddMutation (this would typically be done in a loop
	// after processing the data into nquads). Here we show example of reading a
	// file with RDF data, converting it to NQuads and adding it to the batch.

	f, err := os.Open("goldendata.rdf.gz")
	x.Check(err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	x.Check(err)

	var buf bytes.Buffer
	bufReader := bufio.NewReader(gr)
	for {
		err = x.ReadLine(bufReader, &buf)
		if err != nil {
			break
		}
		line++
		nq, err := rdf.Parse(buf.String())
		if err == rdf.ErrEmpty { // special case: comment/empty line
			buf.Reset()
			continue
		} else if err != nil {
			log.Fatalf("Error while parsing RDF: %v, on line:%v %v", err, line, buf.String())
		}
		buf.Reset()
		if err = batch.AddMutation(nq, client.SET); err != nil {
			log.Fatal("While adding mutation to batch: ", err)
		}
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
	// Wait for all requests to complete. This is very important, else some
	// data might not be sent to server.
	batch.Flush()
}

func ExampleReq_AddMutation() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := graphp.NewDgraphClient(conn)

	req := client.Req{}
	// Creating a person node, and adding a name attribute to it.
	nq := graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "name",
	}

	client.Str("Steven Spielberg", &nq)
	if err := req.AddMutation(nq, client.SET); err != nil {
		// handle error
	}
	nq = graphp.NQuad{
		Subject:   "_:person1",
		Predicate: "salary",
	}
	if err = client.Float(13333.6161, &nq); err != nil {
		log.Fatal(err)
	}

	resp, err := dgraphClient.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_AddMutation_facets() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := graphp.NewDgraphClient(conn)

	req := client.Req{}
	// Doing mutation and setting facets while using the raw query block.
	req.SetQuery(`
mutation {
 set {
  <alice> <name> "alice" .
  <alice> <mobile> "040123456" (since=2006-01-02T15:04:05) .
  <alice> <car> "MA0123" (since=2006-02-02T13:01:09, first=true) .
 }
}
{
 data(id:alice) {
  name
  mobile @facets
  car @facets
 }
}`)
	resp, err := dgraphClient.Run(context.Background(), req.Request())
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_SetQuery() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := graphp.NewDgraphClient(conn)

	req := client.Req{}
	nq := graphp.NQuad{
		Subject:   "alice",
		Predicate: "name",
	}
	client.Str("Alice", &nq)
	req.AddMutation(nq, client.SET)

	nq = graphp.NQuad{
		Subject:   "alice",
		Predicate: "falls.in",
	}
	if err = client.Str("Rabbit hole", &nq); err != nil {
		log.Fatal(err)
	}
	req.AddMutation(nq, client.SET)

	req.SetQuery(`{
		me(id: alice) {
			name
			falls.in
		}
	}`)
	resp, err := dgraphClient.Run(context.Background(), req.Request())
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_SetQueryWithVariables() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := graphp.NewDgraphClient(conn)

	req := client.Req{}
	variables := make(map[string]string)
	variables["$a"] = "3"
	req.SetQueryWithVariables(`
		query test ($a: int = 1) {
			me(id: 0x01) {
				name
				gender
				friend(first: $a) {
					name
				}
			}
		}`, variables)
	resp, err := dgraphClient.Run(context.Background(), req.Request())
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}
