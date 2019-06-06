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
	"fmt"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"

	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

// Request represents a GraphQL request.  It makes no guarantees that the request is valid.
type Request struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

// Validate validates r as a valid GraphQL request for GraphQL schema s.
// If the request is GraphQL valid, it must contain a valid Operation, which is returned,
// otherwise an error is returned.
func (r *Request) Validate(s schema.Schema) (*schema.Operation, error) {
	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: r.Query})
	if gqlErr != nil {
		return nil, gqlErr
	}

	listErr := validator.Validate(rh.schema.schema, doc)
	if len(listErr) != 0 {
		rh.err = fmt.Errorf("Invalid request")
		rh.resp = &graphQLResponse{Errors: listErr}
		return
	}

	op := doc.Operations.ForName(rh.gqlReq.OperationName)
	if op == nil {
		rh.err = fmt.Errorf("Unable to find operation to resolve")
		return
	}

	vars, err := validator.VariableValues(rh.schema.schema, op, rh.gqlReq.Variables)
	if err != nil {
		rh.err = err
		return
	}

	rh.op = &operation{op: op, vars: vars}
}
