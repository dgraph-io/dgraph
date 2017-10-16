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
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type Dgraph struct {
	zero protos.ZeroClient
	dc   []protos.DgraphClient

	mu     sync.Mutex
	needTs []chan uint64
	notify chan struct{}

	state *protos.MembershipState
}

// TODO(tzdybal) - hide this function from users
func NewClient(clients []protos.DgraphClient) *Dgraph {
	d := &Dgraph{
		dc: clients,
	}

	return d
}

// NewDgraphClient creates a new Dgraph for interacting with the Dgraph store connected to in
// conns.
// The client can be backed by multiple connections (to the same server, or multiple servers in a
// cluster).
//
// A single client is thread safe for sharing with multiple go routines (though a single Req
// should not be shared unless the go routines negotiate exclusive assess to the Req functions).
func NewDgraphClient(zero protos.ZeroClient, dc protos.DgraphClient) *Dgraph {
	dg := &Dgraph{
		zero:   zero,
		dc:     []protos.DgraphClient{dc},
		notify: make(chan struct{}, 1),
	}

	go dg.fillTimestampRequests()
	return dg
}

func (d *Dgraph) getTimestamp() uint64 {
	ch := make(chan uint64)
	d.mu.Lock()
	d.needTs = append(d.needTs, ch)
	d.mu.Unlock()

	select {
	case d.notify <- struct{}{}:
	default:
	}
	return <-ch
}

func (d *Dgraph) fillTimestampRequests() {
	var chs []chan uint64
	for range d.notify {
	RETRY:
		d.mu.Lock()
		chs = append(chs, d.needTs...)
		d.needTs = d.needTs[:0]
		d.mu.Unlock()

		if len(chs) == 0 {
			continue
		}
		num := &protos.Num{Val: uint64(len(chs))}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ts, err := d.zero.Timestamps(ctx, num)
		cancel()
		if err != nil {
			log.Printf("Error while retrieving timestamps: %v. Will retry...\n", err)
			goto RETRY
		}
		x.Printf("Got ts lease: %+v\n", ts)
		x.AssertTrue(ts.EndId-ts.StartId+1 == uint64(len(chs)))
		for i, ch := range chs {
			ch <- ts.StartId + uint64(i)
		}
		chs = chs[:0]
	}
}

// DropAll deletes all edges and schema from Dgraph.
// func (d *Dgraph) DropAll(ctx context.Context) error {
// 	req := &Req{
// 		gr: protos.Request{
// 			Mutation: &protos.Mutation{DropAll: true},
// 		},
// 	}

// 	_, err := d.dc[rand.Intn(len(d.dc))].Run(ctx, &req.gr)
// 	return err
// }

func (d *Dgraph) CheckSchema(schema *protos.SchemaUpdate) error {
	if len(schema.Predicate) == 0 {
		return x.Errorf("No predicate specified for schemaUpdate")
	}
	typ := types.TypeID(schema.ValueType)
	if typ == types.UidID && schema.Directive == protos.SchemaUpdate_INDEX {
		// index on uid type
		return x.Errorf("Index not allowed on predicate of type uid on predicate %s",
			schema.Predicate)
	} else if typ != types.UidID && schema.Directive == protos.SchemaUpdate_REVERSE {
		// reverse on non-uid type
		return x.Errorf("Cannot reverse for non-uid type on predicate %s", schema.Predicate)
	}
	return nil
}

// func (d *Dgraph) SetSchemaBlocking(ctx context.Context, updates []*protos.SchemaUpdate) error {
// 	for _, s := range updates {
// 		if err := d.CheckSchema(s); err != nil {
// 			return err
// 		}
// 		req := new(Req)
// 		che := make(chan error, 1)
// 		req.AddSchema(s)
// 		go func() {
// 			if _, err := d.dc[rand.Intn(len(d.dc))].Run(ctx, &req.gr); err != nil {
// 				che <- err
// 				return
// 			}
// 			che <- nil
// 		}()

// 		// blocking wait until schema is applied
// 		select {
// 		case <-ctx.Done():
// 			return ctx.Err()
// 		case err := <-che:
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	return nil
// }

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
func (d *Dgraph) run(ctx context.Context, req *protos.Request) (*protos.Response, error) {
	dc := d.anyClient()
	return dc.Run(ctx, req)
}

func (d *Dgraph) mutate(ctx context.Context, mu *protos.Mutation) (*protos.Assigned, error) {
	dc := d.anyClient()
	return dc.Mutate(ctx, mu)
}

func (d *Dgraph) commitOrAbort(ctx context.Context, txn *protos.TxnContext) (*protos.Payload, error) {
	dc := d.anyClient()
	return dc.CommitOrAbort(ctx, txn)
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

func (d *Dgraph) anyClient() protos.DgraphClient {
	return d.dc[rand.Intn(len(d.dc))]
}
