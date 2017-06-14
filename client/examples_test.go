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
	"strconv"
	"strings"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/rdf"
	"github.com/dgraph-io/dgraph/x"
	"github.com/gogo/protobuf/proto"
	"google.golang.org/grpc"
)

func ExampleBatchMutation() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	// Start a new batch with batch size 1000 and 100 concurrent requests.
	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts, "/tmp")

	// Process your file, convert data to a protos.NQuad and add it to the batch.
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
	var line int
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

		nq.Subject = Node(nq.Subject, dgraphClient)
		if len(nq.ObjectId) > 0 {
			nq.ObjectId = Node(nq.ObjectId, dgraphClient)
		}
		if err = dgraphClient.BatchSet(client.NewEdge(nq)); err != nil {
			log.Fatal("While adding mutation to batch: ", err)
		}
	}
	if err != io.EOF {
		x.Checkf(err, "Error while reading file")
	}
	// Wait for all requests to complete. This is very important, else some
	// data might not be sent to server.
	dgraphClient.BatchFlush()
}

func Node(val string, c *client.Dgraph) string {
	if uid, err := strconv.ParseUint(val, 0, 64); err == nil {
		return c.NodeUid(uid).String()
	}
	if strings.HasPrefix(val, "_:") {
		n, err := c.NodeBlank(val[2:])
		if err != nil {
			log.Fatal("Error while converting to node: %v", err)
		}
		return n.String()
	}
	n, err := c.NodeXid(val, false)
	if err != nil {
		log.Fatal("Error while converting to node: %v", err)
	}
	return n.String()
}

func ExampleReq_AddMutation() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts, "/tmp")

	req := client.Req{}
	person1, err := dgraphClient.NodeBlank("person1")
	if err != nil {
		log.Fatal(err)
	}

	// Creating a person node, and adding a name attribute to it.
	e := person1.Edge("name")
	e.SetValueString("Steven Spielberg")
	req.Set(e)
	e = person1.Edge("salary")
	e.SetValueFloat(13333.6161)
	req.Set(e)

	resp, err := dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_BatchMutation() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts, "/tmp")

	person1, err := dgraphClient.NodeBlank("person1")
	if err != nil {
		log.Fatal(err)
	}

	// Creating a person node, and adding a name attribute to it.
	e := person1.Edge("name")
	e.SetValueString("Steven Spielberg")
	dgraphClient.BatchSet(e)
	e = person1.Edge("salary")
	e.SetValueFloat(13333.6161)
	dgraphClient.BatchSet(e)

	dgraphClient.BatchFlush()
}

func ExampleReq_AddMutation_facets() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts, "/tmp")

	req := client.Req{}
	person1, err := dgraphClient.NodeXid("person1", false)
	if err != nil {
		log.Fatal(err)
	}
	e := person1.Edge("name")
	e.SetValueString("Steven Spielberg")
	e.AddFacet("since", "2006-01-02T15:04:05")
	e.AddFacet("alias", `"Steve"`)

	req.Set(e)

	person2, err := dgraphClient.NodeXid("person2", false)
	if err != nil {
		log.Fatal(err)
	}
	e = person2.Edge("name")
	e.SetValueString("William Jones")
	req.Set(e)

	e = person1.ConnectTo("friend", person2)
	e.AddFacet("close", "true")
	req.Set(e)

	req.SetQuery(`{
		me(id: person1) {
			name @facets
			friend @facets {
				name
			}
		}
	}`)

	resp, err := dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_AddMutation_schema() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := protos.NewDgraphClient(conn)

	req := client.Req{}
	// Doing mutation and setting schema, then getting schema.
	req.SetQuery(`
mutation {
 schema {
  name: string @index .
  release_date: date @index .
 }
}

schema {}
`)
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

	bmOpts := client.BatchMutationOptions{
		Size:          1000,
		Pending:       100,
		PrintCounters: false,
	}
	dgraphClient := client.NewDgraphClient(conn, bmOpts, "/tmp")

	req := client.Req{}
	alice, err := dgraphClient.NodeXid("alice", false)
	if err != nil {
		log.Fatal(err)
	}
	e := alice.Edge("name")
	e.SetValueString("Alice")
	req.Set(e)

	e = alice.Edge("falls.in")
	e.SetValueString("Rabbit hole")
	req.Set(e)

	req.SetQuery(`{
		me(id: alice) {
			name
			falls.in
		}
	}`)
	resp, err := dgraphClient.Run(context.Background(), &req)
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
}

func ExampleReq_SetQueryWithVariables() {
	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure())
	x.Checkf(err, "While trying to dial gRPC")
	defer conn.Close()

	dgraphClient := protos.NewDgraphClient(conn)

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
