/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
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

package client

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
)

var (
	ErrMaxTries = errors.New("Max retries exceeded for request while doing batch mutations.")
	emptyEdge   Edge
)

type Dgraph struct {
	opts BatchMutationOptions

	schema chan protos.SchemaUpdate
	dc     []protos.DgraphClient
}

// NewDgraphClient creates a new Dgraph for interacting with the Dgraph store connected to in
// conns.  The Dgraph client stores blanknode to uid, and XIDnode to uid mappings on disk
// in clientDir.
//
// The client can be backed by multiple connections (to the same server, or multiple servers in a
// cluster).
//
// A single client is thread safe for sharing with multiple go routines (though a single Req
// should not be shared unless the go routines negotiate exclusive assess to the Req functions).
func NewDgraphClient(conns []*grpc.ClientConn) *Dgraph {
	var clients []protos.DgraphClient
	for _, conn := range conns {
		client := protos.NewDgraphClient(conn)
		clients = append(clients, client)
	}
	return NewClient(clients)
}

// TODO(tzdybal) - hide this function from users
func NewClient(clients []protos.DgraphClient) *Dgraph {
	d := &Dgraph{
		dc: clients,
	}

	return d
}

func (d *Dgraph) printCounters() {
	d.ticker = time.NewTicker(2 * time.Second)
	start := time.Now()

	for range d.ticker.C {
		counter := d.Counter()
		rate := float64(counter.Rdfs) / counter.Elapsed.Seconds()
		elapsed := ((time.Since(start) / time.Second) * time.Second).String()
		fmt.Printf("[Request: %6d] Total RDFs done: %8d RDFs per second: %7.0f Time Elapsed: %v \r",
			counter.Mutations, counter.Rdfs, rate, elapsed)

	}
}

func (d *Dgraph) makeSchemaRequests() {
	req := new(Req)
LOOP:
	for {
		select {
		case s, ok := <-d.schema:
			if !ok {
				break LOOP
			}
			req.AddSchema(s)
		default:
			if atomic.LoadInt32(&d.retriesExceeded) == 1 {
				d.che <- ErrMaxTries
				return
			}
			start := time.Now()
			if req.Size() > 0 {
				d.request(req)
				req = new(Req)
			}
			elapsedMillis := time.Since(start).Seconds() * 1e3
			if elapsedMillis < 10 {
				time.Sleep(time.Duration(int64(10-elapsedMillis)) * time.Millisecond)
			}
		}
	}

	if req.Size() > 0 {
		d.request(req)
	}
	d.che <- nil
}

// DropAll deletes all edges and schema from Dgraph.
func (d *Dgraph) DropAll() error {
	req := &Req{
		gr: protos.Request{
			Mutation: &protos.Mutation{DropAll: true},
		},
	}
	select {
	case d.reqs <- req:
		return nil
	case <-d.opts.Ctx.Done():
		return d.opts.Ctx.Err()
	}
}

// AddSchema adds the given schema mutation to the batch of schema mutations.  If the schema
// mutation applies an index to a UID edge, or if it adds reverse to a scalar edge, then the
// mutation is not added to the batch and an error is returned. Once added, the client will
// apply the schema mutation when it is ready to flush its buffers.
func (d *Dgraph) AddSchema(s protos.SchemaUpdate) error {
	if err := checkSchema(s); err != nil {
		return err
	}
	d.schema <- s
	return nil
}

func (d *Dgraph) SetSchemaBlocking(ctx context.Context, q string) error {
	req := new(Req)
	che := make(chan error, 1)
	req.SetSchema(q)
	go func() {
		if _, err := d.dc[rand.Intn(len(d.dc))].Run(ctx, &req.gr); err != nil {
			che <- err
			return
		}
		che <- nil
	}()

	// blocking wait until schema is applied
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-che:
		return err
	}
}

func (d *Dgraph) stopTickers() {
	if d.ticker != nil {
		d.ticker.Stop()
	}
	if d.checkpointTicker != nil {
		d.checkpointTicker.Stop()
	}
}

// BatchFlush waits for all pending requests to complete. It should always be called after all
// BatchSet and BatchDeletes have been called.  Calling BatchFlush ends the client session and
// will cause a panic if further AddSchema, BatchSet or BatchDelete functions are called.
func (d *Dgraph) BatchFlush() error {
	close(d.nquads)
	close(d.schema)
	for i := 0; i < d.opts.Pending+2; i++ {
		select {
		case err := <-d.che:
			if err != nil {
				// To signal other go-routines to stop.
				d.stopTickers()
				return err
			}
		}
	}

	// After we have received response from server and sent the marks for completion,
	// we need to wait for all of them to be processed.
	for _, wm := range d.marks {
		wm.wg.Wait()
	}
	// Write final checkpoint before stopping.
	d.writeCheckpoint()
	d.stopTickers()
	return nil
}

// Run runs the request in req and returns with the completed response from the server.  Calling
// Run has no effect on batched mutations.
//
// Mutations in the request are run before a query --- except when query variables link the
// mutation and query (see for example NodeUidVar) when the query is run first.
//
// Run returns a protos.Response which has the following fields
//
// - L : Latency information
//
// - Schema : Result of a schema query
//
// - AssignedUids : a map[string]uint64 of blank node name to assigned UID (if the query string
// contained a mutation with blank nodes)
//
// - N : Slice of *protos.Node returned by the query (Note: protos.Node not client.Node).
//
// There is an N[i], with Attribute "_root_", for each named query block in the query added to req.
// The N[i] also have a slice of nodes, N[i].Children each with Attribute equal to the query name,
// for every answer to that query block.  From there, the Children represent nested blocks in the
// query, the Attribute is the edge followed and the Properties are the scalar edges.
//
// Print a response with
// 	"github.com/gogo/protobuf/proto"
// 	...
// 	req.SetQuery(`{
// 		friends(func: eq(name, "Alex")) {
//			name
//			friend {
// 				name
//			}
//		}
//	}`)
// 	...
// 	resp, err := dgraphClient.Run(context.Background(), &req)
// 	fmt.Printf("%+v\n", proto.MarshalTextString(resp))
// Outputs
//	n: <
//	  attribute: "_root_"
//	  children: <
//	    attribute: "friends"
//	    properties: <
//	      prop: "name"
//	      value: <
//	        str_val: "Alex"
//	      >
//	    >
//	    children: <
//	      attribute: "friend"
//	      properties: <
//	        prop: "name"
//	        value: <
//	          str_val: "Chris"
//	        >
//	      >
//	    >
//	...
//
// It's often easier to unpack directly into a struct with Unmarshal, than to
// step through the response.
func (d *Dgraph) Run(ctx context.Context, req *Req) (*protos.Response, error) {
	res, err := d.dc[rand.Intn(len(d.dc))].Run(ctx, &req.gr)
	if err == nil {
		req = &Req{}
	}
	return res, err
}

// Counter returns the current state of the BatchMutation.
func (d *Dgraph) Counter() Counter {
	return Counter{
		Rdfs:      atomic.LoadUint64(&d.rdfs),
		Mutations: atomic.LoadUint64(&d.mutations),
		Elapsed:   time.Since(d.start),
	}
}

// CheckVersion checks if the version of dgraph and dgraph-live-loader are the same.  If either the
// versions don't match or the version information could not be obtained an error message is
// printed.
func (d *Dgraph) CheckVersion(ctx context.Context) {
	v, err := d.dc[rand.Intn(len(d.dc))].CheckVersion(ctx, &protos.Check{})
	if err != nil {
		fmt.Printf(`Could not fetch version information from Dgraph. Got err: %v.`, err)
	} else {
		version := x.Version()
		if version != "" && v.Tag != "" && version != v.Tag {
			fmt.Printf(`
Dgraph server: %v, loader: %v dont match.
You can get the latest version from https://docs.dgraph.io
`, v.Tag, version)
		}
	}
}

// NodeUid creates a Node from the given uint64.
func (d *Dgraph) NodeUid(uid uint64) Node {
	return Node{uid: uid}
}

func xidKey(xid string) string {
	// Prefix to avoid key clashes with other data stored in badger.
	return "\x01" + xid
}

// NodeBlank creates or returns a Node given a string name for the blank node. Blank nodes do not
// exist as labelled nodes in Dgraph. Blank nodes are used as labels client side for loading and
// linking nodes correctly.  If the label is new in this session a new UID is allocated and
// assigned to the label.  If the label has already been assigned, the corresponding Node
// is returned.  If the empty string is given as the argument, a new node is allocated and returned
// but no map is stored, so every call to NodeBlank("") returns a new node.
func (d *Dgraph) NodeBlank(varname string) (Node, error) {
	if len(varname) == 0 {
		uid, err := d.alloc.AllocateUid()
		if err != nil {
			return Node{}, err
		}
		return Node{uid: uid}, nil
	}
	uid, _, err := d.alloc.AssignUid(xidKey("_:" + varname))
	if err != nil {
		return Node{}, err
	}
	return Node{uid: uid}, nil
}

// NodeXid creates or returns a Node given a string name for an XID node. An XID node identifies a
// node with an edge xid, as in
// 	node --- xid ---> XID string
// See https://docs.dgraph.io/query-language/#external-ids If the XID has already been allocated
// in this client session the allocated UID is returned, otherwise a new UID is allocated
// for xid and returned.
func (d *Dgraph) NodeXid(xid string, storeXid bool) (Node, error) {
	if len(xid) == 0 {
		return Node{}, ErrEmptyXid
	}
	uid, isNew, err := d.alloc.AssignUid(xidKey(xid))
	if err != nil {
		return Node{}, err
	}
	n := Node{uid: uid}
	if storeXid && isNew {
		e := n.Edge("xid")
		x.Check(e.SetValueString(xid))
		d.BatchSet(e)
	}
	return n, nil
}

// NodeUidVar creates a Node from a variable name.  When building a request, set and delete
// mutations may depend on the request's query, as in:
// https://docs.dgraph.io/query-language/#variables-in-mutations Such query variables in mutations
// could be built into the raw query string, but it is often more convenient to use client
// functions than manipulate strings.
//
// A request with a query and mutations (including variables in mutations) will run in the same
// manner as if the query and mutations were set into the query string.
func (d *Dgraph) NodeUidVar(name string) (Node, error) {
	if len(name) == 0 {
		return Node{}, ErrEmptyVar
	}

	return Node{varName: name}, nil
}
