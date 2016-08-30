/*
 * Copyright 2015 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package types

import (
	"fmt"
	"github.com/dgraph-io/dgraph/gql"
)

// TODO(akhil): validator for client uploaded schema as well, to ensure it declares all types.

// GraphQLSchema declares the schema structure the GraphQL queries.
type GraphQLSchema struct {
	Query    GraphQLObject
	Mutation GraphQLObject
}

var (
	// Schema defines a dummy schema to test type and validaiton system.
	Schema *GraphQLSchema
	// QueryType defines sample basic schema.
	QueryType GraphQLObject
	// personType for validating coercion system implementation.
	personType GraphQLObject
)

// LoadSchema loads the schema
func LoadSchema() error {

	var combinedError error
	var err error

	// load scalar types
	if err = LoadScalarTypes(); err != nil {
		combinedError = err
	}

	// TODO(akhil): implement mechanism for client to define and upload a schema.
	// Client schema will be parsed and loaded here.

	personType = GraphQLObject{
		Name: "Person",
		Desc: "object to represent a person type",
		Fields: FieldMap{
			"name": &Field{
				Type: String,
			},
			"gender": &Field{
				Type: String,
			},
			"age": &Field{
				Type: Int,
			},
			"sword_present": &Field{
				Type: Boolean,
			},
			"survival_rate": &Field{
				Type: Float,
			},
		},
	}

	QueryType = GraphQLObject{
		Name: "Query",
		Desc: "Sample query structure",
		Fields: FieldMap{
			"actor": &Field{
				Type: personType,
			},
		},
	}
	Schema = &GraphQLSchema{Query: QueryType}
	return combinedError
}

// ValidateMutation validates the parsed mutation string against the present schema.
// TODO(akhil): traverse Mutation and compare each node with corresponding schema struct.
// TODO(akhil): implement error function (extending error interface).
func ValidateMutation(mu *gql.Mutation, s *GraphQLSchema) error {
	return nil
}

func joinError(e1 error, e2 error) error {
	return fmt.Errorf("%v\n%v", e1.Error(), e2.Error())
}
