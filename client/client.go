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
	"encoding/base64"
	"fmt"
	"time"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
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
}

// Request returns the protos.Request backing the Req.
func (req *Req) Request() *protos.Request {
	return &req.gr
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

func (req *Req) addMutation(e Edge, op opType) {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(protos.Mutation)
	}

	if op == SET {
		req.gr.Mutation.Set = append(req.gr.Mutation.Set, &e.nq)
	} else if op == DEL {
		req.gr.Mutation.Del = append(req.gr.Mutation.Del, &e.nq)
	}
}

// Set adds edge e to the set mutation of request req, thus scheduling the edge to be added to the 
// graph when the request is run.  The edge must have a valid target (a Node or value), otherwise 
// an error is returned.  The edge is not checked agaist the schema until the request is 
// run --- so setting a UID edge to a value, for example, doesn't result in an error until 
// the request is run.
func (req *Req) Set(e Edge) error {
	if err := e.validate(); err != nil {
		return err
	}
	req.addMutation(e, SET)
	return nil
}

// Delete adds edge e to the delete mutation of request req, thus scheduling the edge to be removed
// from the graph when the request is run. The edge must have a valid target (a Node or value), 
// otherwise an error is returned.  The edge need not represent
// an edge in the graph --- applying such a mutation simply has no effect.
func (req *Req) Delete(e Edge) error {
	if err := e.validate(); err != nil {
		return err
	}
	req.addMutation(e, DEL)
	return nil
}

// AddSchema adds the single schema mutation s to the request.
func (req *Req) AddSchema(s protos.SchemaUpdate) error {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(protos.Mutation)
	}
	req.gr.Mutation.Schema = append(req.gr.Mutation.Schema, &s)
	return nil
}

// AddSchemaFromString parses s for schema mutations and adds each update in s
// to the request using AddSchema.  The given string should be of the form:
// edgename: uid @reverse .
// edge2: string @index(exact) .
// etc.
// to use the form "mutuation { schema { ... }}" issue the mutation through
// SetQuery.
func (req *Req) AddSchemaFromString(s string) error {
	schemaUpdate, err := schema.Parse(s)
	if err != nil {
		return err
	}

	if len(schemaUpdate) == 0 {
		return nil
	}

	for _, smut := range schemaUpdate {
		if err = req.AddSchema(*smut); err != nil {
			return err
		}
	}

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
	op opType
}

// Node represents a single node in the graph.
type Node struct {
	uid uint64
	// We can do variables in mutations.
	varName string
}

// String returns Node n as a string
func (n Node) String() string {
	if n.uid != 0 {
		return fmt.Sprintf("%#x", uint64(n.uid))
	}
	return n.varName
}

// ConnectTo creates an edge labelled pred from Node n to Node n1
func (n *Node) ConnectTo(pred string, n1 Node) Edge {
	e := Edge{}
	if len(n.varName) != 0 {
		e.nq.SubjectVar = n.String()
	} else {
		e.nq.Subject = n.String()
	}
	e.nq.Predicate = pred
	e.ConnectTo(n1)
	return e
}

// Edge create an edge with source Node n and predicate pred, but without a target.
// The edge needs to be completed by calling Edge.ConnectTo() if the edge is a
// UID edge, or one of the Edge.SetValue...() functions if the edge is of a scalar type.
// The edge can't be committed to the store --- calling Req.Set() to add the edge to
// a request will result in an error --- until it is completed.
func (n *Node) Edge(pred string) Edge {
	e := Edge{}
	if len(n.varName) != 0 {
		e.nq.SubjectVar = n.String()
	} else {
		e.nq.Subject = n.String()
	}
	e.nq.Predicate = pred
	return e
}

// An Edge represents an edge between a source node and a target (either a node or a value).
// Facets are stored in the edge.  See Node.Edge(), Node.ConnectTo(), Edge.ConnecTo(),
// Edge.AddFacet and the Edge.SetValue...() functions to
// make a valid edge for a set or delete mutation.
type Edge struct {
	nq protos.NQuad
}

// NewEdge creates an Edge from an NQuad.
func NewEdge(nq protos.NQuad) Edge {
	return Edge{nq}
}

// ConnectTo adds Node n as the target of the edge.  If the edge already has a known scalar type,
// for example if Edge.SetValue...() had been called on the edge, then an error is returned.
func (e *Edge) ConnectTo(n Node) error {
	if e.nq.ObjectType > 0 {
		return ErrValue
	}
	if len(n.varName) != 0 {
		e.nq.ObjectVar = n.String()
	} else {
		e.nq.ObjectId = n.String()
	}
	return nil
}

func (e *Edge) validate() error {
	// Edge should be connected to a value in which case ObjectType would be > 0.
	// Or it needs to connect to a Node (ObjectId > 0) or it should be connected to a variable.
	if e.nq.ObjectValue != nil || len(e.nq.ObjectId) > 0 || len(e.nq.ObjectVar) > 0 {
		return nil
	}
	return ErrNotConnected
}

func validateStr(val string) error {
	for idx, c := range val {
		if c == '"' && (idx == 0 || val[idx-1] != '\\') {
			return fmt.Errorf(`" must be preceded by a \ in object value`)
		}
	}
	return nil
}

// SetValueString sets the value of Edge e as string val and sets the type of the edge to 
// types.StringID.  If the edge had previous been assigned another value (even of another type), 
// the value and type are overwritten.  If the edge has previously been connected to a node, the 
// edge and type are left unchanged and ErrConnected is returned.  The string must 
// escape " with \, otherwise the edge and type are left unchanged and an error returned.
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

// SetValueInt sets the value of Edge e as int64 val and sets the type of the edge to types.IntID.
// If the edge had previous been assigned another value (even of another type), the value and type
// are overwritten.  If the edge has previously been connected to a node, the edge and type are 
// left unchanged and ErrConnected is returned.
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

// SetValueFloat sets the value of Edge e as float64 val and sets the type of the edge to 
// types.FloatID.  If the edge had previous been assigned another value (even of another type), 
// the value and type are overwritten.  If the edge has previously been connected to a node, the
// edge and type are left unchanged and ErrConnected is returned.
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

// SetValueBool sets the value of Edge e as bool val and sets the type of the edge to types.BoolID.
// If the edge had previous been assigned another value (even of another type), the value and type
// are overwritten.  If the edge has previously been connected to a node, the edge and type are 
// left unchanged and ErrConnected is returned.
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

// SetValuePassword sets the value of Edge e as password string val and sets the type of the edge 
// to types.PasswordID.  If the edge had previous been assigned another value (even of another 
// type), the value and type are overwritten.  If the edge has previously been connected to a 
// node, the edge and type are left unchanged and ErrConnected is returned.
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

// SetValueDatetime sets the value of Edge e as time.Time dateTime and sets the type of the edge 
// to types.DateTimeID.  If the edge had previous been assigned another value (even of another 
// type), the value and type are overwritten.  If the edge has previously been connected to a node,
// the edge and type are left unchanged and ErrConnected is returned.
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

// SetValueGeoJson sets the value of Edge e as the GeoJSON object parsed from json string and sets
// the type of the edge to types.GeoID.  If the edge had previous been assigned another value (even
// of another type), the value and type are overwritten.  If the edge has previously been connected
// to a node, the edge and type are left unchanged and ErrConnected is returned. If the string 
// fails to parse with geojson.Unmarshal() the edge is left unchanged and an error returned.
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

// SetValueDefault sets the value of Edge e as string val and sets the type of the edge to 
// types.DefaultID.  If the edge had previous been assigned another value (even of another 
// type), the value and type are overwritten.  If the edge has previously been connected to 
// a node, the edge and type are left unchanged and ErrConnected is returned.
// The string must escape " with \, otherwise the edge and type are left unchanged and an error returned.
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

// SetValueBytes allows setting the value of an edge to raw bytes and sets the type of the edge 
// to types.BinaryID.  If the edge had previous been assigned another value (even of another type),
// the value and type are overwritten.  If the edge has previously been connected to a node, the 
// edge and type are left unchanged and ErrConnected is returned. the bytes are encoded as base64.
func (e *Edge) SetValueBytes(val []byte) error {
	if len(e.nq.ObjectId) > 0 {
		return ErrConnected
	}
	dst := make([]byte, base64.StdEncoding.EncodedLen(len(val)))
	base64.StdEncoding.Encode(dst, val)
	v, err := types.ObjectValue(types.BinaryID, []byte(dst))
	if err != nil {
		return err
	}
	e.nq.ObjectValue = v
	e.nq.ObjectType = int32(types.BinaryID)
	return nil
}

// AddFacet adds the key, value pair as facets on Edge e.  No checking is done.
func (e *Edge) AddFacet(key, val string) {
	e.nq.Facets = append(e.nq.Facets, &protos.Facet{
		Key: key,
		Val: val,
	})
}
