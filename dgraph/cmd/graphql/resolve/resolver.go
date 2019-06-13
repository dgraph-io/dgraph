/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package resolve

import (
	"github.com/golang/glog"

	"github.com/dgraph-io/dgo"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/vektah/gqlparser/gqlerror"
)

// GraphQL spec:
// https://graphql.github.io/graphql-spec/June2018
//
//
// GraphQL servers should serve both GET and POST
// https://graphql.org/learn/serving-over-http/
//
// GET should be like
// http://myapi/graphql?query={me{name}}
//
// POST should have a json content body like
// {
//   "query": "...",
//   "operationName": "...",
//   "variables": { "myVariable": "someValue", ... }
// }
//
// GraphQL servers should return 200 (even on errors),
// and result body should be json:
// {
//   "data": { "query_name" : { ... } },
//   "errors": [ { "message" : ..., ...} ... ]
// }
//
// Key points about the response
// (https://graphql.github.io/graphql-spec/June2018/#sec-Response)
//
// - If an error was encountered before execution begins,
//   the data entry should not be present in the result.
//
// - If an error was encountered during the execution that
//   prevented a valid response, the data entry in the response should be null.
//
// - If there's errors and data, both are returned
//
// - If no errors were encountered during the requested operation,
//   the errors entry should not be present in the result.
//
// - There's rules around how errors work when there's ! fields in the schema
//   https://graphql.github.io/graphql-spec/June2018/#sec-Errors-and-Non-Nullability
//
// - The "message" in an error is required, the rest is up to the implementation
//
// - The "data" works just like a Dgraph query
//

const (
	idArgName    = "id"
	inputArgName = "input"
)

// RequestResolver can process GraphQL requests and write GraphQL JSON responses.
type RequestResolver struct {
	GqlReq       *schema.Request
	Schema       schema.Schema
	dgraphClient *dgo.Dgraph
	resp         *schema.Response
}

// New creates a new RequestResolver
func New(s schema.Schema, dc *dgo.Dgraph) *RequestResolver {
	return &RequestResolver{
		Schema:       s,
		dgraphClient: dc,
		resp:         &schema.Response{},
	}
}

// WithErrors records all errors errs in rh to be reported when a GraphQL
// response is generated
func (r *RequestResolver) WithErrors(errs ...*gqlerror.Error) {
	r.resp.Errors = append(r.resp.Errors, errs...)
}

// Resolve processes rh.GqlReq and returns a GraphQL response.
// rh.GqlReq should be set with a request before Resolve is called
// and a schema and backend should have been added.
// Resolve records any errors in the response's error field.
func (r *RequestResolver) Resolve() *schema.Response {
	if r == nil {
		glog.Error("Call to Resolve with nil RequestResolver")
		return schema.ErrorResponsef("Internal error")
	}

	if r.Schema == nil {
		glog.Error("Call to Resolve with no schema")
		return schema.ErrorResponsef("Internal error")
	}

	if r.resp.Errors != nil {
		r.resp.Data.Reset()
		return r.resp
	}

	op, resp := r.Schema.Operation(r.GqlReq)
	if resp != nil {
		return resp
	}

	switch {
	case op.IsQuery():
		// TODO: this should handle queries in parallel
		for _, q := range op.Queries() {
			r.resolveQuery(q)
		}
	case op.IsMutation():
		// unlike queries, mutations are always handled serially
		for _, m := range op.Mutations() {
			r.resolveMutation(m)
		}
	case op.IsSubscription():
		schema.ErrorResponsef("Subscriptions not yet supported")
	}

	return r.resp
}
