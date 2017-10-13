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
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/dgraph/protos"
)

type opType int

const (
	// SET indicates a Set mutation.
	SET opType = iota
	// DEL indicates a Delete mutation.
	DEL
)

// A Req represents a single request to the backend Dgraph instance.  Each request may contain
// multiple set, delete and schema mutations, and a single GraphQL+- query.  If the query contains
// GraphQL variables, then it must be set with SetQueryWithVariables rather than SetQuery.
type Req struct {
	gr protos.Request
	//	mark   *x.WaterMark
	//	line   uint64
	//	markWg *sync.WaitGroup // non-nil only if mark is non-nil
}

// Request returns the protos.Request backing the Req.
func (req *Req) Request() *protos.Request {
	return &req.gr
}

// SetQuery sets the query in req to the given string.
// The query string is not checked until the request is
// run, when it is parsed and checked server-side.
func (req *Req) SetQuery(q string) {
	req.gr.Query = q
}

// SetQueryWithVariables sets query q (which contains graphQL variables mapped
// in vars) as the query in req and sets vars as the corresponding query variables.
// Neither the query string nor the variables are checked until the request is run,
// when it is parsed and checked server-side.
func (req *Req) SetQueryWithVariables(q string, vars map[string]string) {
	req.gr.Query = q
	req.gr.Vars = vars
}

// SetObject allows creating a new nested object (struct). If the struct has a _uid_
// field then it is updated, else a new node is created with the given properties and edges.
// If the object can't be marshalled using json.Marshal then an error is returned.
func (req *Req) SetObject(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(protos.Mutation)
	}
	req.gr.Mutation.SetJson = b
	return nil
}

// DeleteObject allows deleting a nested object (struct).
//
// 1. If properties other than the _uid_ are specified, then only those are set for deletion.
//
// 2. If no properties are specified and only the _uid_ is specified then that corresponds to a
// S * * deletion and all properties of the node are set for deletion.
//
// 3. If only predicates are specified with null value, then it is considered a * P * deletion and
// all data for the predicate is set for deletion.
//
// If the object can't be marshalled using json.Marshal then also an error is returned.
func (req *Req) DeleteObject(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(protos.Mutation)
	}
	req.gr.Mutation.DeleteJson = b
	return nil
}

// SetSchema sets schema mutation in req with the given schema
// The schema is not checked until the request is run, when it is parsed and
// checked server-side
func (req *Req) SetSchema(q string) {
	req.gr.Query = fmt.Sprintf("mutation {\nschema {\n%s\n}\n}", q)
}

func (req *Req) AddSchema(s *protos.SchemaUpdate) error {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(protos.Mutation)
	}
	req.gr.Mutation.Schema = append(req.gr.Mutation.Schema, s)
	return nil
}

func (req *Req) Reset() {
	req.gr.Query = ""
	req.gr.Mutation.SetJson = req.gr.Mutation.SetJson[:0]
	req.gr.Mutation.DeleteJson = req.gr.Mutation.DeleteJson[:0]
	req.gr.Mutation.Schema = req.gr.Mutation.Schema[:0]
}
