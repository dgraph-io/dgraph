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

package schema

import (
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

// A Request represents a GraphQL request.  It makes no guarantees that the
// request is valid.
type Request struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

// Operation finds the operation in req, if it is a valid request for GraphQL
// schema s. If the request is GraphQL valid, it must contain a single valid
// Operation.  If either the request is malformed or doesn't contain a valid
// operation, all GraphQL errors encountered are returned.
func (s *schema) Operation(req *Request) (Operation, error) {
	if req == nil || req.Query == "" {
		return nil, gqlerror.Errorf("no query string supplied in request")
	}

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: req.Query})
	if gqlErr != nil {
		return nil, gqlErr
	}

	listErr := validator.Validate(s.schema, doc)
	if len(listErr) != 0 {
		return nil, listErr
	}

	op := doc.Operations.ForName(req.OperationName)
	if op == nil {
		return nil, gqlerror.Errorf("unable to find operation to resolve")
	}

	vars, gqlErr := validator.VariableValues(s.schema, op, req.Variables)
	if gqlErr != nil {
		return nil, gqlErr
	}

	return &operation{op: op, vars: vars, inSchema: s.schema}, nil
}
