/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// This package provides helper function for interacting with the Dgraph server.
// You can use it to run mutations and queries. You can also use BatchMutation
// to upload data concurrently.
//  conn, err := grpc.Dial(*dgraph, grpc.WithInsecure())

//  // Creating a new client.
//  c := graphp.NewDgraphClient(conn)
//  // Starting a new request.
//  req := client.Req{}
//  // _:person1 tells Dgraph to assign a new Uid and is the preferred way of creating new nodes.
//  // See https://docs.dgraph.io/master/query-language/#assigning-uid for more details.
//  nq := graphp.NQuad{
//  	Subject:   "_:person1",
//  	Predicate: "name",
//  }
//  // Str is a helper function to add a string value.
//  client.Str("Steven Spielberg", &nq)
//  // Adding a new mutation.
//  req.AddMutation(nq, client.SET)
//  nq = graphp.NQuad{
//  	Subject:   "_:person1",
//  	Predicate: "age",
//  }
//  // Int is used to add integer values.
//  if err = client.Int(25, &nq); err != nil {
//  	log.Fatal(err)
//  }
//  req.AddMutation(nq, client.SET)

//  Lets create another person and add a name for it.
//  nq = graphp.NQuad{
//  	Subject:   "_:person2",
//  	Predicate: "name",
//  }
//  // Str is a helper function to add a string value.
//  client.Str("William Jones", &nq)
//  // Adding a new mutation.
//  req.AddMutation(nq, client.SET)

//  // Lets connect the two nodes together.
//  nq = graphp.NQuad{
//  	Subject:   "_:person1",
//  	Predicate: "friend",
//  	ObjectId:  "_:person2",
//  }
//  req.AddMutation(nq, client.SET)
//  // Lets run the request with all these mutations.
//  resp, err := c.Run(context.Background(), req.Request())
//  if err != nil {
//  	log.Fatalf("Error in getting response from server, %s", err)
//  }
//  person1Uid := resp.AssignedUids["person1"]
//  person2Uid := resp.AssignedUids["person2"]

//  // Lets initiate a new request and query for the data.
//  req = client.Req{}
//  // Lets set the starting node id to person1Uid.
//  req.SetQuery(fmt.Sprintf("{ me(id: %v) { _uid_ name now birthday loc salary age married friend {_uid_ name} } }", client.Uid(person1Uid)),
//  	map[string]string{})
//  resp, err = c.Run(context.Background(), req.Request())
//  if err != nil {
//  	log.Fatalf("Error in getting response from server, %s", err)
//  }

//  person1 := resp.N[0].Children[0]
//  props := person1.Properties
//  name := props[0].Value.GetStrVal()
//  fmt.Println("Name: ", name)

//  fmt.Println("Age: ", props[1].Value.GetIntVal())

//  person2 := person1.Children[0]
//  fmt.Printf("%v name: %v\n", person2.Attribute, person2.Properties[0].Value.GetStrVal())

//  // Deleting an edge.
//  nq = graphp.NQuad{
//  	Subject:   client.Uid(person1Uid),
//  	Predicate: "friend",
//  	ObjectId:  client.Uid(person2Uid),
//  }
//  req = client.Req{}
//  req.AddMutation(nq, client.DEL)
//  // Lets run the request with all these mutations.
//  resp, err = c.Run(context.Background(), req.Request())
//  if err != nil {
//  	log.Fatalf("Error in getting response from server, %s", err)
//  }
package client

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type Op int

const (
	// Indicates a Set mutation.
	SET Op = iota
	// Indicates a Delete mutation.
	DEL
)

// Req wraps the graphp.Request so that helper methods can be defined on it.
type Req struct {
	gr graphp.Request
}

// Request returns the graph request object which is sent to the server to perform
// a query/mutation.
func (req *Req) Request() *graphp.Request {
	return &req.gr
}

func checkNQuad(nq graphp.NQuad) error {
	if len(nq.Subject) == 0 {
		return fmt.Errorf("Subject can't be empty")
	}
	if len(nq.Predicate) == 0 {
		return fmt.Errorf("Predicate can't be empty")
	}

	hasVal := nq.ObjectValue != nil
	if len(nq.ObjectId) == 0 && !hasVal {
		return fmt.Errorf("Both objectId and objectValue can't be nil")
	}
	if len(nq.ObjectId) > 0 && hasVal {
		return fmt.Errorf("Only one out of objectId and objectValue can be set")
	}
	return nil
}

// SetQuery sets a query as part of the request.
func (req *Req) SetQuery(q string, vars map[string]string) {
	req.gr.Query = q
	req.gr.Vars = vars
}

func (req *Req) addMutation(nq graphp.NQuad, op Op) {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(graphp.Mutation)
	}

	if op == SET {
		req.gr.Mutation.Set = append(req.gr.Mutation.Set, &nq)
	} else if op == DEL {
		req.gr.Mutation.Del = append(req.gr.Mutation.Del, &nq)
	}
}

// AddMutation adds (but does not send) a mutation to the Req object. Mutations
// are sent when client.Run() is called.
func (req *Req) AddMutation(nq graphp.NQuad, op Op) error {
	if err := checkNQuad(nq); err != nil {
		return err
	}
	req.addMutation(nq, op)
	return nil
}

func checkSchema(schema graphp.SchemaUpdate) error {
	typ := types.TypeID(schema.ValueType)
	if typ == types.UidID && schema.Directive == graphp.SchemaUpdate_INDEX {
		// index on uid type
		return x.Errorf("Index not allowed on predicate of type uid on predicate %s",
			schema.Predicate)
	} else if typ != types.UidID && schema.Directive == graphp.SchemaUpdate_REVERSE {
		// reverse on non-uid type
		return x.Errorf("Cannot reverse for non-uid type on predicate %s", schema.Predicate)
	}
	return nil
}

// AddSchema sets the schema mutations
func (req *Req) addSchema(s graphp.SchemaUpdate) error {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(graphp.Mutation)
	}
	req.gr.Mutation.Schema = append(req.gr.Mutation.Schema, &s)
	return nil
}

func (req *Req) size() int {
	if req.gr.Mutation == nil {
		return 0
	}
	return len(req.gr.Mutation.Set) + len(req.gr.Mutation.Del) + len(req.gr.Mutation.Schema)
}

func (req *Req) reset() {
	req.gr.Query = ""
	req.gr.Mutation.Set = req.gr.Mutation.Set[:0]
	req.gr.Mutation.Del = req.gr.Mutation.Del[:0]
	req.gr.Mutation.Schema = req.gr.Mutation.Schema[:0]
}

type nquadOp struct {
	nq graphp.NQuad
	op Op
}

// BatchMutation is used to batch mutations and send them off to the server
// concurrently. It is useful while doing migrations and bulk data loading.
// It is possible to control the batch size and the number of concurrent requests
// to make.
type BatchMutation struct {
	size    int
	pending int

	nquads chan nquadOp
	schema chan graphp.SchemaUpdate
	dc     graphp.DgraphClient
	wg     sync.WaitGroup

	// Miscellaneous information to print counters.
	// Num of RDF's sent
	rdfs uint64
	// Num of mutations sent
	mutations uint64
	// To get time elapsed.
	start time.Time
}

func (batch *BatchMutation) request(req *Req) {
	counter := atomic.AddUint64(&batch.mutations, 1)
RETRY:
	_, err := batch.dc.Run(context.Background(), &req.gr)
	if err != nil {
		errString := err.Error()
		// Irrecoverable
		if strings.Contains(errString, "x509") || grpc.Code(err) == codes.Internal {
			log.Fatal(errString)
		}
		fmt.Printf("Retrying req: %d. Error: %v\n", counter, errString)
		time.Sleep(5 * time.Millisecond)
		goto RETRY
	}
	req.reset()
}

func (batch *BatchMutation) makeRequests() {
	req := new(Req)

	for n := range batch.nquads {
		req.addMutation(n.nq, n.op)
		if req.size() == batch.size {
			batch.request(req)
		}
	}

	if req.size() > 0 {
		batch.request(req)
	}
	batch.wg.Done()
}

func (batch *BatchMutation) makeSchemaRequests() {
	req := new(Req)
LOOP:
	for {
		select {
		case s, ok := <-batch.schema:
			if !ok {
				break LOOP
			}
			req.addSchema(s)
		default:
			start := time.Now()
			if req.size() > 0 {
				batch.request(req)
			}
			elapsedMillis := time.Since(start).Seconds() * 1e3
			if elapsedMillis < 10 {
				time.Sleep(time.Duration(int64(10-elapsedMillis)) * time.Millisecond)
			}
		}
	}

	if req.size() > 0 {
		batch.request(req)
	}
	batch.wg.Done()
}

// NewBatchMutation is used to create a new batch.
// size is the number of RDF's that are sent as part of one request to Dgraph.
// pending is the number of concurrent requests to make to Dgraph server.
func NewBatchMutation(ctx context.Context, client graphp.DgraphClient,
	size int, pending int) *BatchMutation {
	bm := BatchMutation{
		size:    size,
		pending: pending,
		nquads:  make(chan nquadOp, 2*size),
		schema:  make(chan graphp.SchemaUpdate, 2*size),
		start:   time.Now(),
		dc:      client,
	}

	for i := 0; i < pending; i++ {
		bm.wg.Add(1)
		go bm.makeRequests()
	}
	bm.wg.Add(1)
	go bm.makeSchemaRequests()
	return &bm
}

// AddMutation is used to add a NQuad to a batch. It can either have SET or
// DEL as Op(operation).
func (batch *BatchMutation) AddMutation(nq graphp.NQuad, op Op) error {
	if err := checkNQuad(nq); err != nil {
		return err
	}
	batch.nquads <- nquadOp{nq: nq, op: op}
	atomic.AddUint64(&batch.rdfs, 1)
	return nil
}

// Flush waits for all pending requests to complete. It should always be called
// after adding all the NQuads using batch.AddMutation().
func (batch *BatchMutation) Flush() {
	close(batch.nquads)
	close(batch.schema)
	batch.wg.Wait()
}

// AddSchema is used to add a schema mutation.
func (batch *BatchMutation) AddSchema(s graphp.SchemaUpdate) error {
	if err := checkSchema(s); err != nil {
		return err
	}
	batch.schema <- s
	return nil
}

// Counter keeps a track of various parameters about a batch mutation.
type Counter struct {
	// Number of RDF's processed by server.
	Rdfs uint64
	// Number of mutations processed by the server.
	Mutations uint64
	// Time elapsed sinze the batch started.
	Elapsed time.Duration
}

// Counter returns the current state of the BatchMutation.
func (batch *BatchMutation) Counter() Counter {
	return Counter{
		Rdfs:      atomic.LoadUint64(&batch.rdfs),
		Mutations: atomic.LoadUint64(&batch.mutations),
		Elapsed:   time.Since(batch.start),
	}
}
