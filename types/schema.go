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

// GraphQLSchema declares the schema structure the GraphQL queries.
type GraphQLSchema struct {
	Query    GraphQLObject
	Mutation GraphQLObject
}

// Schema defines a dummy schema to test type and validation system.
var Schema GraphQLSchema

// LoadSchema loads the schema
func LoadSchema() {

	// load scalar types
	loadScalarTypes()

	// TODO(akhil): implement mechanism for client to define and upload a schema.
	// Client schema will be parsed and loaded here.

	personType := GraphQLObject{
		Name: "Person",
		Desc: "object to represent a person type",
		Fields: FieldMap{
			"name":          String,
			"gender":        String,
			"age":           Int,
			"survival_rate": Float,
			"sword_present": Boolean,
		},
	}

	// defined this field separately to let personType instantiate properly
	personType.Fields["friend"] = GraphQLList{HasType: personType}

	queryType := GraphQLObject{
		Name: "Query",
		Desc: "Sample query structure",
		Fields: FieldMap{
			"actor": personType,
		},
	}
	Schema = GraphQLSchema{Query: queryType}
}
