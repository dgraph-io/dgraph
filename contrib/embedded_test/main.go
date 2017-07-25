package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/gogo/protobuf/proto"

	"github.com/dgraph-io/dgraph/client"
	"github.com/dgraph-io/dgraph/dgraph"
	"github.com/dgraph-io/dgraph/x"
)

func main() {
	x.Logger = log.New(ioutil.Discard, "", 0)
	clientDir, err := ioutil.TempDir("", "client_")
	x.Check(err)

	config := dgraph.GetDefaultEmbeddedConfig()
	dgraphClient := dgraph.NewEmbeddedDgraphClient(config, client.DefaultOptions, clientDir)
	defer dgraph.DisposeEmbeddedDgraph()

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

	req.SetQuery(`
mutation {
 schema {
  name: string @index(exact) .
 }
}
`)
	resp, err := dgraphClient.Run(context.Background(), &req)
	if err != nil {
		log.Fatalf("Error in getting response from server, %s", err)
	}
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))

	req.SetQuery(`{
		me(func:eq(name, "Alice")) {
			name
			falls.in
		}
	}`)

	resp, err = dgraphClient.Run(context.Background(), &req)
	resp.Descriptor()
	fmt.Printf("%+v\n", proto.MarshalTextString(resp))

}
