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
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/gqlerror"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
)

type schemaHandler struct {
	input string
	// Should I store ast.Schema object as well ?
	errs gqlerror.List
}

func (s *schemaHandler) genGQLSchema() (string, gqlerror.List) {
	if s.input == "" {
		return "", nil
	}

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: s.input})
	if gqlErr != nil {
		return "", []*gqlerror.Error{gqlErr}
	}

	if gqlErrList := schema.ValidateSchema(doc); gqlErrList != nil {
		return "", gqlErrList
	}

	schema.AddScalars(doc)

	sch, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return "", []*gqlerror.Error{gqlErr}
	}

	schema.GenerateCompleteSchema(sch)

	return schema.Stringify(sch), nil
}

func (s *schemaHandler) genDGSchema() (string, gqlerror.List) {
	if s.input == "" {
		return "", nil
	}

	doc, gqlErr := parser.ParseSchema(&ast.Source{Input: s.input})
	if gqlErr != nil {
		return "", []*gqlerror.Error{gqlErr}
	}

	if gqlErrList := schema.ValidateSchema(doc); gqlErrList != nil {
		return "", gqlErrList
	}

	schema.AddScalars(doc)

	sch, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return "", []*gqlerror.Error{gqlErr}
	}

	return dgraph.GenDgSchema(sch), nil
}
