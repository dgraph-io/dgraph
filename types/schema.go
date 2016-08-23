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

import "github.com/dgraph-io/dgraph/gql"

// TODO(akhil): validator for client uploaded schema as well, to ensure it declares all types.
// GraphQLSchema declares the schema structure the GraphQL queries.
type GraphQLSchema struct {
	Query    GraphQLObject
	Mutation GraphQLObject
}

// TestSchema defines a dummy schema to test type and validaiton system.
var TestSchema = &GraphQLSchema{Query: queryType}

// queryType defines sample basic schema.
var queryType = GraphQLObject{
	Name: "My Query",
	Desc: "Investiture of a Shard",
	Fields: FieldMap{
		"Work": &Field{
			Type: String,
			Resolve: func(rp ResolveParams) interface{} {
				return "In progress"
			},
		},
	},
}

// ValidateSchema validates the parsed query tree against the present schema.
// TODO(akhil): traverse GraphQuery and compare each node with corresponding schema struct.
// TODO(akhil): implement error function (extending error interface).
func ValidateSchema(gq *gql.GraphQuery, s *GraphQLSchema) error {
	return nil
}
