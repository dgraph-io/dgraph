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

package graphql

import (
	"encoding/json"
	"mime"
	"net/http"

	"github.com/dgraph-io/dgo"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
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

type graphqlHandler struct {
	dgraphClient *dgo.Dgraph
	schema       *ast.Schema
}

type graphqlRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

// ServeHTTP handles GraphQL queries and mutations that get translated
// like GraphQL->Dgraph->GraphQL.  It writes a valid GraphQL json response
// to w.
func (gh *graphqlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	var gqlReq graphqlRequest

	switch r.Method {
	case http.MethodGet:
		// fill gqlReq in
	case http.MethodPost:
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			// error response - no data
			return
		}

		switch mediaType {
		case "application/json":
			if err = json.NewDecoder(r.Body).Decode(&gqlReq); err != nil {
				// error response - no data
				return
			}
		default:
			// nothing else is valid?
			//
			// error response - no data
			return
		}
	default:
		// error response - no data
		return
	}

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: gqlReq.Query})
	if gqlErr != nil {
		// error response - no data
		return
	}

	listErr := validator.Validate(gh.schema, doc)
	if len(listErr) != 0 {
		// error response - no data
		return
	}

	op := doc.Operations.ForName(gqlReq.OperationName)
	if op == nil {
		// error response - no data
		return
	}

	// actually need the output vars here because there's been some type magic
	// done on them in the validator
	_, err := validator.VariableValues(gh.schema, op, gqlReq.Variables)
	if err != nil {
		// error response - no data
		return
	}

	switch op.Operation {
	case ast.Query:
		resp := &graphQLResponse{
			Data: []byte(`{ "lifeQuery": [ { "meaning" : 42 } ] }`),
		}

		b, err := json.Marshal(resp)
		if err != nil {
			// error response - "data": null
			return
		}
		_, err = w.Write(b)
		if err != nil {
			// nothing to do, just log?
		}
	case ast.Mutation:
		b, err := json.Marshal(errorResponse("Not yet implemented"))
		if err != nil {
			// error response - "data": null
			return
		}
		_, err = w.Write(b)
		if err != nil {
			// nothing to do, just log?
		}
	default:
		// error response - no data
		return
	}
}
