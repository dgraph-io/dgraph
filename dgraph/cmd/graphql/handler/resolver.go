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

package handler

import (
	"github.com/dgraph-io/dgo"
	"github.com/vektah/gqlparser/ast"
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
// Key points about the response (https://graphql.github.io/graphql-spec/June2018/#sec-Response)
//
// - If an error was encountered before execution begins,
//   the data entry should not be present in the result.
//
// - If an error was encountered during the execution that
//   prevented a valid response, the data entry in the response should be null.
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

// RequestHandler can process GraphQL requests and write JSON responses.
type RequestHandler struct {
	GqlReq       *Request
	op           *Operation
	schema       *schema
	dgraphClient *dgo.Dgraph
	err          error
}

// WithAstSchema sets the schema a rh uses.
func (rh *RequestHandler) WithAstSchema(s *ast.Schema) {
	rh.schema.schema = s
}

// WithDgoBackend sets rh to use dc as its Dgraph backend for
// resolving GraphQL queries and mutations.
func (rh *RequestHandler) WithDgoBackend(dc *dgo.Dgraph) {
	rh.dgraphClient = dc
}

// Resolve processes rh.GqlReq and returns a GraphQL response.
// rh.GqlReq should be set with a request before Resolve is called
// and a schema and backend should have been added.
// Resolve records any errors in the response's error field.
func (rh *RequestHandler) Resolve() *Response {
	// TODO: fill in with previous http response code
	return nil
}
