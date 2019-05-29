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
	"github.com/dgraph-io/dgo"
	"github.com/vektah/gqlparser/ast"
	"net/http"
)

// Spec https://facebook.github.io/graphql/
//
// Serves Get and Post
// https://graphql.org/learn/serving-over-http/
//
// Get:
// http://myapi/graphql?query={me{name}}
//
// Post:
// Should have a json content body like
// {
//   "query": "...",
//   "operationName": "...",
//   "variables": { "myVariable": "someValue", ... }
// }
//
// result should be json like
// {
//   "data": { ... },
//   "errors": [ ... ]
// }
//
//
// Note the case for multiples in a single operation
//
// {
//   q1(...) {...}
//   q2(...) {...}
// }
//
// should result in
// {
//   "data": {
//     "q1": {...},
//     "q2": {...}
//   }
//   "errors": [ ... ]
// }

type graphqlHandler struct {
	dgraphClient *dgo.Dgraph
	schema       *ast.Schema
}

func (gh *graphqlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(ErrorResponse("Not yet implemented"))
	if err != nil {
		panic(err)
	}
	w.Write(b)
}
