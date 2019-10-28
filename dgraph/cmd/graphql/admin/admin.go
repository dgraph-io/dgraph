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

package admin

import (
	"time"

	"github.com/dgraph-io/dgo/v2"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/web"
)

const (
	// GraphQL schema for /admin endpoint.
	//
	// Eventually we should generate this from just the types definition.
	// But for now, that would add too much into the schema, so this is
	// hand crafted to be one of our schemas so we can pass it into the
	// pipeline.
	adminSchema = `
 type Schema {
	schema: String!  # the input schema, not the expanded schema
	date: DateTime!
 }
 
 type Health {
	message: String!
	status: HealthStatus!
 }
 
 enum HealthStatus {
	ErrNoConnection
	NoGraphQLSchema
	Healthy
 }
 
 scalar DateTime
 
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
 
 type UpdateSchemaPayload {
	schema: Schema
 }
 
 input DateTimeFilter {
	eq: DateTime
	le: DateTime
	lt: DateTime
	ge: DateTime
	gt: DateTime
 }
 
 input SchemaFilter {
	date: DateTimeFilter
	and: SchemaFilter
	or: SchemaFilter
	not: SchemaFilter
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
	health: Health
 }
 
 type Mutation {
	addSchema(input: SchemaInput!) : AddSchemaPayload
 }
 `
)

type schemaDef struct {
	Schema string    `json:"schema,omitempty"`
	Date   time.Time `json:"date,omitempty"`
}

// NewAdminResolver creates a GraphQL request resolver for the /admin endpoint.
func NewAdminResolver(
	dg *dgo.Dgraph,
	gqlServer web.IServeGraphQL,
	fns *resolve.ResolverFns) *resolve.RequestResolver {

	adminSchema, err := schema.FromString(adminSchema)
	if err != nil {
		panic(err)
	}

	return resolve.New(adminSchema, newAdminResolverFactory(dg, gqlServer, fns))
}

func newAdminResolverFactory(
	dg *dgo.Dgraph,
	gqlServer web.IServeGraphQL,
	fns *resolve.ResolverFns) resolve.ResolverFactory {

	// The args are the rewriters and executors used by the endpoint being admin'd.
	// These are the ones used by the admin endpoint itself.
	mutRw := resolve.NewMutationRewriter()
	qryRw := resolve.NewQueryRewriter()
	qryExec := resolve.DgoAsQueryExecutor(dg)
	mutExec := resolve.DgoAsMutationExecutor(dg)

	return resolve.NewResolverFactory().
		WithQueryResolver("health",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(
					resolve.NoOpQueryRewrite(),
					&healthResolver{status: healthy},
					resolve.StdQueryCompletion())
			}).
		WithMutationResolver("addSchema",
			func(m schema.Mutation) resolve.MutationResolver {
				addResolver := &addSchemaResolver{
					gqlServer:            gqlServer,
					dgraph:               dg,
					baseMutationRewriter: mutRw,
					baseMutationExecutor: mutExec,
					fns:                  fns,
					withIntrospection:    true}

				return resolve.NewMutationResolver(
					addResolver,
					qryExec,
					addResolver,
					resolve.StdMutationCompletion(m.Name()))
			}).
		WithQueryResolver("querySchema",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(
					qryRw,
					qryExec,
					resolve.StdQueryCompletion())
			}).
		WithSchemaIntrospection()
}
