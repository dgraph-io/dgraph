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
	"context"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/x"
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

// TODO - This can be removed. Right now removing it causes some tests to break in client_test package.
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

// Set is used to set nquads directly. Most clients would find it easier to use SetObject instead.
// TODO(pawan) - Hide this from user.
func (req *Req) Set(nquad *protos.NQuad) {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(protos.Mutation)
	}
	req.gr.Mutation.Set = append(req.gr.Mutation.Set, nquad)
}

type Txn struct {
	startTs uint64
	context *protos.TxnContext

	dg *Dgraph
}

func (d *Dgraph) NewTxn() *Txn {
	ts := d.getTimestamp()
	txn := &Txn{
		startTs: ts,
		dg:      d,
	}
	return txn
}

func (txn *Txn) Query(q string, vars map[string]string) (*protos.Response, error) {
	req := &protos.Request{
		Query:   q,
		Vars:    vars,
		StartTs: txn.startTs,
	}
	return txn.dg.run(context.Background(), req)
}

func (txn *Txn) mergeContext(src *protos.TxnContext) error {
	if txn.context == nil {
		txn.context = src
		return nil
	}
	if txn.context.Primary != src.Primary {
		return x.Errorf("Primary key mismatch")
	}
	if txn.context.StartTs != src.StartTs {
		return x.Errorf("StartTs mismatch")
	}
	txn.context.Keys = append(txn.context.Keys, src.Keys...)
	return nil
}

func (txn *Txn) Mutate(mu *protos.Mutation) (*protos.Assigned, error) {
	mu.StartTs = txn.startTs
	ag, err := txn.dg.mutate(context.Background(), mu)
	if err == nil {
		err = txn.mergeContext(ag.Context)
	}
	return ag, err
}

func (txn *Txn) Abort() error {
	txn.context.CommitTs = 0
	_, err := txn.dg.commitOrAbort(context.Background(), txn.context)
	return err
}

func (txn *Txn) Commit() error {
	if len(txn.context.Keys) == 0 {
		// If all the prewrites don't change anything.
		return nil
	}
	txn.context.CommitTs = txn.dg.getTimestamp()
	_, err := txn.dg.commitOrAbort(context.Background(), txn.context)
	return err
}
