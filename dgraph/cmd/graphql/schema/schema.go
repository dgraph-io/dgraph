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

import "github.com/vektah/gqlparser/ast"

// wrap the github.com/vektah/gqlparser/ast defintions so that the bulk of the GraphQL
// algorithm and interface is dependent on behaviours not the exact structure in there.

type Schema interface {
}

type Operation interface {
	Queries() []Query
	Mutations() []Mutation
}

type Mutation interface {
}

type Query interface {
}

type schema struct {
	schema *ast.Schema
}

type operation struct {
	op   *ast.OperationDefinition
	vars map[string]interface{}
}

type mutation struct {
	schema *ast.Field
}

type query struct {
	schema *ast.Field
}

func (*operation) Queries() []Query {
	// TODO:
	return nil
}

func (*operation) Mutations() []Mutation {
	// TODO:
	return nil
}
