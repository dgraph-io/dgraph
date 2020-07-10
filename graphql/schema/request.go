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
	"net/http"

	"github.com/pkg/errors"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
)

// A Request represents a GraphQL request.  It makes no guarantees that the
// request is valid.
type Request struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`

	Header http.Header
}

// Operation finds the operation in req, if it is a valid request for GraphQL
// schema s. If the request is GraphQL valid, it must contain a single valid
// Operation.  If either the request is malformed or doesn't contain a valid
// operation, all GraphQL errors encountered are returned.
func (s *schema) Operation(req *Request) (Operation, error) {
	if req == nil || req.Query == "" {
		return nil, errors.New("no query string supplied in request")
	}

	doc, gqlErr := parser.ParseQuery(&ast.Source{Input: req.Query})
	if gqlErr != nil {
		return nil, gqlErr
	}

	listErr := validator.Validate(s.schema, doc)
	if len(listErr) != 0 {
		return nil, listErr
	}

	if len(doc.Operations) == 1 && doc.Operations[0].Operation == ast.Subscription &&
		s.schema.Subscription == nil {
		return nil, errors.Errorf("Not resolving subscription because schema doesn't have any " +
			"fields defined for subscription operation.")
	}

	if len(doc.Operations) > 1 && req.OperationName == "" {
		return nil, errors.Errorf("Operation name must by supplied when query has more " +
			"than 1 operation.")
	}

	op := doc.Operations.ForName(req.OperationName)
	if op == nil {
		return nil, errors.Errorf("Supplied operation name %s isn't present in the request.",
			req.OperationName)
	}

	vars, gqlErr := validator.VariableValues(s.schema, op, req.Variables)
	if gqlErr != nil {
		return nil, gqlErr
	}

	operation := &operation{op: op,
		vars:     vars,
		query:    req.Query,
		header:   req.Header,
		doc:      doc,
		inSchema: s,
	}

	// recursively expand fragments in operation as selection set fields
	for _, s := range op.SelectionSet {
		recursivelyExpandFragmentSelections(s.(*ast.Field), operation)
	}

	return operation, nil
}

// recursivelyExpandFragmentSelections puts a fragment's selection set directly inside this
// field's selection set, and does it recursively for all the fields in this field's selection
// set. This eventually expands all the fragment references anywhere in the hierarchy.
// To understand how expansion works, let's consider following graphql schema (Reference: Starwars):
// 			interface Employee { ... }
// 			interface Character { ... }
// 			type Human implements Character & Employee { ... }
// 			type Droid implements Character { ... }
// 1. field returns an Interface: Consider executing following query:
//			query {
//				queryCharacter {
//					...commonCharacterFrag
//					...humanFrag
//					...droidFrag
//				}
//			}
//			fragment commonCharacterFrag on Character { ... }
//			fragment humanFrag on Human { ... }
//			fragment droidFrag on Droid { ... }
//    As queryCharacter returns Characters, so any fragment reference used inside queryCharacter and
//    defined on Character interface should be expanded. Also, any fragments defined on the types
//    which implement Character interface should also be expanded. That means, any fragments on
//    Character, Human and Droid will be expanded in the result of queryCharacter.
// 2. field returns an Object: Consider executing following query:
// 			query {
//				queryHuman {
//					...employeeFrag
//					...characterFrag
//					...humanFrag
//				}
//			}
//			fragment employeeFrag on Employee { ... }
//			fragment characterFrag on Character { ... }
//			fragment humanFrag on Human { ... }
//    As queryHuman returns Humans, so any fragment reference used inside queryHuman and
//    defined on Human type should be expanded. Also, any fragments defined on the interfaces
//    which are implemented by Human type should also be expanded. That means, any fragments on
//    Human, Character and Employee will be expanded in the result of queryHuman.
// 3. field returns a Union: process is similar to the case when field returns an interface.
func recursivelyExpandFragmentSelections(field *ast.Field, op *operation) {
	// This happens in case of introspection queries, as they don't have any types in graphql schema
	// but explicit resolvers defined. So, when the parser parses the raw request, it is not able to
	// find a definition for such fields in the schema. Introspection queries are already handling
	// fragments, so it is fine to not do it for them. But, in future, if anything doesn't have
	// associated types for them in graphql schema, then it needs to handle fragment expansion by
	// itself.
	if field.Definition == nil {
		return
	}

	// Find all valid type names that this field satisfies

	typeName := field.Definition.Type.Name()
	typeKind := op.inSchema.schema.Types[typeName].Kind
	// this field always has to expand any fragment on its own type
	// "" tackles the case for an inline fragment which doesn't specify type condition
	satisfies := []string{typeName, ""}
	var additionalTypes []*ast.Definition
	switch typeKind {
	case ast.Interface:
		// expand fragments on types which implement this interface
		additionalTypes = op.inSchema.schema.PossibleTypes[typeName]
	case ast.Union:
		// expand fragments on types of which it is a union
		additionalTypes = op.inSchema.schema.PossibleTypes[typeName]
	case ast.Object:
		// expand fragments on interfaces which are implemented by this object
		additionalTypes = op.inSchema.schema.Implements[typeName]
	default:
		// return, as fragment can't be present on a field which is not Interface, Union or Object
		return
	}
	for _, typ := range additionalTypes {
		satisfies = append(satisfies, typ.Name)
	}

	// collect all fields from any satisfying fragments into selectionSet
	collectedFields := collectFields(&requestContext{
		RawQuery:  op.query,
		Variables: op.vars,
		Doc:       op.doc,
	}, field.SelectionSet, satisfies)
	field.SelectionSet = make([]ast.Selection, 0, len(collectedFields))
	for _, collectedField := range collectedFields {
		field.SelectionSet = append(field.SelectionSet, collectedField.Field)
	}

	// It helps when __typename is requested for an Object in a fragment on Interface, so we don't
	// have to fetch dgraph.type from dgraph. Otherwise, each field in the selection set will have
	// its ObjectDefinition point to an Interface instead of an Object, resulting in wrong output
	// for __typename. For example:
	// 		query {
	//			queryHuman {
	//				...characterFrag
	//				...
	//			}
	//		}
	//		fragment characterFrag on Character {
	//			__typename
	//			...
	//		}
	// Here, queryHuman is guaranteed to return an Object and not an Interface, so dgraph.type is
	// never fetched for it, thinking that its fields will have their ObjectDefinition point to a
	// Human. But, when __typename is put into the selection set of queryHuman expanding the
	// fragment on Character (an Interface), it still has its ObjectDefinition point to Character.
	// This, if not set to point to Human, will result in __typename being reported as Character.
	if typeKind == ast.Object {
		typeDefinition := op.inSchema.schema.Types[typeName]
		for _, f := range field.SelectionSet {
			f.(*ast.Field).ObjectDefinition = typeDefinition
		}
	}

	// recursively run for this field's selectionSet
	for _, f := range field.SelectionSet {
		recursivelyExpandFragmentSelections(f.(*ast.Field), op)
	}
}
