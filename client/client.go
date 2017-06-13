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
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

type Op int

const (
	// SET indicates a Set mutation.
	SET Op = iota
	// DEL indicates a Delete mutation.
	DEL
)

// Req wraps the protos.Request so that helper methods can be defined on it.
type Req struct {
	gr protos.Request
}

// Request returns the graph request object which is sent to the server to perform
// a query/mutation.
func (req *Req) Request() *protos.Request {
	return &req.gr
}

func checkNQuad(nq protos.NQuad) error {
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

func checkSchema(schema protos.SchemaUpdate) error {
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

// SetQuery sets a query with graphQL variables as part of the request.
func (req *Req) SetQuery(q string) {
	req.gr.Query = q
}

// SetQueryWithVariables sets a query with graphQL variables as part of the request.
func (req *Req) SetQueryWithVariables(q string, vars map[string]string) {
	req.gr.Query = q
	req.gr.Vars = vars
}

func (req *Req) addMutation(e Edge, op Op) {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(protos.Mutation)
	}

	if op == SET {
		req.gr.Mutation.Set = append(req.gr.Mutation.Set, &e.nq)
	} else if op == DEL {
		req.gr.Mutation.Del = append(req.gr.Mutation.Del, &e.nq)
	}
}

func (req *Req) Set(e Edge) error {
	if err := checkNQuad(e.nq); err != nil {
		return err
	}
	req.addMutation(e, SET)
	return nil
}

func (req *Req) Delete(e Edge) error {
	if err := checkNQuad(e.nq); err != nil {
		return err
	}
	req.addMutation(e, DEL)
	return nil
}

// AddSchema sets the schema mutations
func (req *Req) addSchema(s protos.SchemaUpdate) error {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(protos.Mutation)
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
	e  Edge
	op Op
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

type Node uint64

func (n Node) String() string {
	return fmt.Sprintf("%#x", uint64(n))
}

func (n *Node) ScalarEdge(pred string, val interface{},
	typ types.TypeID) (Edge, error) {
	e := n.Edge(pred)
	switch typ {
	case types.StringID:
		e.SetValueString(val.(string))
	case types.DefaultID:
		e.SetValueDefault(val.(string))
	case types.DateTimeID:
		e.SetValueDatetime(val.(time.Time))
	case types.PasswordID:
		e.SetValuePassword(val.(string))
	case types.IntID:
		e.SetValueInt(val.(int64))
	case types.FloatID:
		e.SetValueFloat(val.(float64))
	case types.BoolID:
		e.SetValueBool(val.(bool))
	case types.GeoID:
		e.SetValueGeoJson(val.(string))
	default:
		return emptyEdge, ErrInvalidType
	}
	return e, nil
}

func (n *Node) ConnectTo(pred string, n1 Node) Edge {
	e := Edge{}
	e.nq.Subject = n.String()
	e.nq.Predicate = pred
	e.ConnectTo(n1)
	return e
}

func (n *Node) Edge(pred string) Edge {
	e := Edge{}
	e.nq.Subject = n.String()
	e.nq.Predicate = pred
	return e
}

type Edge struct {
	nq protos.NQuad
}

func NewEdge(nq protos.NQuad) Edge {
	return Edge{nq}
}

func (e *Edge) ConnectTo(n Node) error {
	if e.nq.ObjectType > 0 {
		return ErrValue
	}
	e.nq.ObjectId = n.String()
	return nil
}

func validateStr(val string) error {
	for idx, c := range val {
		if c == '"' && (idx == 0 || val[idx-1] != '\\') {
			return fmt.Errorf(`" must be preceded by a \ in object value`)
		}
	}
	return nil
}

func (e *Edge) SetValueString(val string) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	if err := validateStr(val); err != nil {
		return err
	}

	v, err := types.ObjectValue(types.StringID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.StringID)
	return nil
}

func (e *Edge) SetValueInt(val int64) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	v, err := types.ObjectValue(types.IntID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.IntID)
	return nil
}

func (e *Edge) SetValueFloat(val float64) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	v, err := types.ObjectValue(types.FloatID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.FloatID)
	return nil
}

func (e *Edge) SetValueBool(val bool) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	v, err := types.ObjectValue(types.BoolID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.BoolID)
	return nil
}

func (e *Edge) SetValuePassword(val string) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	v, err := types.ObjectValue(types.PasswordID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.PasswordID)
	return nil
}

func (e *Edge) SetValueDatetime(dateTime time.Time) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	d, err := types.ObjectValue(types.DateTimeID, dateTime)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = d
	e.nq.ObjectType = int32(types.DateTimeID)
	return nil
}

func (e *Edge) SetValueGeoJson(json string) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	var g geom.T
	// Parse the json
	err := geojson.Unmarshal([]byte(json), &g)
	if err != nil {
		return err
	}

	geo, err := types.ObjectValue(types.GeoID, g)
	if err != nil {
		return err
	}

	e.nq.ObjectValue = geo
	e.nq.ObjectType = int32(types.GeoID)
	return nil
}

func (e *Edge) SetValueDefault(val string) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	if err := validateStr(val); err != nil {
		return err
	}

	v, err := types.ObjectValue(types.DefaultID, val)
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.StringID)
	return nil
}

func (e *Edge) AddFacet(key, val string) {
	e.nq.Facets = append(e.nq.Facets, &protos.Facet{
		Key: key,
		Val: val,
	})
}
