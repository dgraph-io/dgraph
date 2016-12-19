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

package client

import (
	"fmt"

	"github.com/dgraph-io/dgraph/query/graph"
)

type Op int

const (
	SET Op = iota
	DEL
)

// Req wraps the graph.Request so that we can define helper methods for the
// client around it.
type Req struct {
	gr graph.Request
}

// NewRequest initializes and returns a new request which can be used to query
// or perform set/delete mutations.
func NewRequest() Req {
	return Req{}
}

// Request returns the graph request object which is sent to the server to perform
// a query/mutation.
func (req *Req) Request() *graph.Request {
	return &req.gr
}

func checkNQuad(nq graph.NQuad) error {
	if len(nq.Sub) == 0 {
		return fmt.Errorf("Subject can't be empty")
	}
	if len(nq.Pred) == 0 {
		return fmt.Errorf("Predicate can't be empty")
	}
	hasVal := nq.Value != nil && nq.Value.Val != nil
	if len(nq.ObjId) == 0 && !hasVal {
		return fmt.Errorf("Both objectId and objectValue can't be nil")
	}
	if len(nq.ObjId) > 0 && hasVal {
		return fmt.Errorf("Only one out of objectId and objectValue can be set")
	}
	return nil
}

// SetQuery sets a query as part of the request.
// Example usage
// req := client.NewRequest()
// req.SetQuery("{ me(_xid_: alice) { name falls.in } }")
// resp, err := c.Query(context.Background(), req.Request())
// Check response and handle errors
func (req *Req) SetQuery(q string) {
	req.gr.Query = q
}

// AddMutation adds a SET/DELETE mutation operation.
//
// Example usage
// req := client.NewRequest()
// To set a string value
// if err := req.AddMutation(graph.NQuad{
// 	Sub:   "alice",
// 	Pred:  "name",
// 	Value: client.Str("Alice"),
// }, client.SET); err != nil {
// ....
// handle error
// ....
// }

// To set an integer value
// if err := req.AddMutation(graph.NQuad{
// 	Sub:   "alice",
// 	Pred:  "age",
// 	Value: client.Int(13),
// }, client.SET); err != nil {
// ....
// handle error
// ....
// }

// To add a mutation with a DELETE operation
// if err := req.AddMutation(graph.NQuad{
// 	Sub:   "alice",
// 	Pred:  "name",
// 	Value: client.Str("Alice"),
// }, client.DEL); err != nil {
// ....
// handle error
// ....
// }
func (req *Req) AddMutation(nq graph.NQuad, op Op) error {
	if err := checkNQuad(nq); err != nil {
		return err
	}

	if req.gr.Mutation == nil {
		req.gr.Mutation = new(graph.Mutation)
	}

	if op == SET {
		req.gr.Mutation.Set = append(req.gr.Mutation.Set, &nq)
	} else if op == DEL {
		req.gr.Mutation.Del = append(req.gr.Mutation.Del, &nq)
	}
	return nil
}
