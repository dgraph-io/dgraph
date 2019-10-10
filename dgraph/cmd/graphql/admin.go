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
	"context"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/gql"
)

// GraphQL schema for /admin endpoint.

// Eventually we should generate this from just the types definition.
// But for now, that would add too much into the schema, so this is
// hand crafted to be one of our schemas so we can pass it into the
// pipeline.

const (
	adminSchema = `
type Schema {
    schema: String!  # the input schema, not the expanded schema
    date: DateTime! @search(by: day)
}

type Health {
    message: String!
    status: HealthStatus!
}

enum HealthStatus {
    OK
    DgraphUnreachable
}

type SchemaDiff {
    types: [TypeDiff!]
}

type TypeDiff {
    name: String!
    new: Boolean
    newFields: [String!]
    missingFields: [String!]
}

type AddSchemaPayload {
    schema: Schema
    diff: SchemaDiff
}

input SchemaInput {
    schema: String!
    dateAdded: DateTime
}

input SchemaOrder {
	asc: SchemaOrderable
	desc: SchemaOrderable
	then: SchemaOrder
}

enum SchemaOrderable {
	date
}

type Query {
    querySchema(filter: SchemaFilter, order: SchemaOrder, first: Int, offset: Int): [Schema]
    health(): Health
}

type Mutation {
    addSchema(input: SchemaInput!) : AddSchemaPayload
}
`
)

type healthResolver struct{}

func (hr *healthResolver) Resolve(ctx context.Context) (*resolve.Resolved, bool) {
	// The actual algorithm for determining health goes here
	return &resolve.Resolved{Data: []byte("I'm alive")}, true
}

type schemaAdder struct {
	baseRewriter         resolve.QueryRewriter
	baseQueryExecutor    resolve.QueryExecutor
	baseMutationExecutor resolve.MutationExecutor
}

func (sa *schemaAdder) Rewrite(q schema.Query) (*gql.GraphQuery, error) {

	// 1) grab the schema argument
	//
	// 2) apply the validation and dgraph schema gen from schema package
	//    return any errors if there are any
	//
	// 3) alter the dgraph schema
	//    return any errors if there are any
	//
	// 4) persist the schema by just continuing to pass through the resolver pipeline
	// which is just...

	return sa.baseRewriter.Rewrite(q)
}

func (sa *schemaAdder) FromMutationResult(
	m schema.Mutation,
	assigned map[string]string,
	mutated map[string][]string) (*gql.GraphQuery, error) {

	return sa.baseRewriter.FromMutationResult(m, assigned, mutated)
}

func NewAdminResolverFactory(
	dg dgraph.Client,
	queryRewriter resolve.QueryRewriter,
	mutRewriter resolve.MutationRewriter) resolve.ResolverFactory {

	return resolve.NewResolverFactory(dg, queryRewriter, mutRewriter).
		WithResolver(
			func(gqlSchema schema.Schema, op schema.Operation, field schema.Field) resolve.Resolver {
				if qry, ok := field.(schema.Query); ok {
					if qry.Name() == "health" {
						return &healthResolver{}
					}
				}
				return nil
			})
}
