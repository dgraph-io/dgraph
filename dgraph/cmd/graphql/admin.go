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
	"encoding/json"
	"time"

	"github.com/dgraph-io/dgo"
	dgoapi "github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/web"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/golang/glog"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/ast"
	"github.com/vektah/gqlparser/parser"
	"github.com/vektah/gqlparser/validator"
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
    date: DateTime! #@search(by: day)
}

type Health {
    message: String!
    status: HealthStatus!
}

enum HealthStatus {
    OK
    DgraphUnreachable
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

type schemaInput struct {
	Schema    string
	DateAdded time.Time
}

type healthResolver struct{}

func (hr *healthResolver) Resolve(ctx context.Context, query schema.Query) (*resolve.Resolved, bool) {
	// FIXME: The actual algorithm for determining health goes here...
	return &resolve.Resolved{Data: []byte(`"message" : "I'm alive"`)}, true
}

type schemaAdder struct {
	dgraph               *dgo.Dgraph
	baseMutationRewriter resolve.MutationRewriter

	gqlServer web.IServeGraphQL // The GraphQL endpoint that's being admin'd.

	// When the schema changes, we use these to create a new RequestResolver for
	// the main graphql endpoint (gqlServer) and thus refresh the API.
	queryExecutor     resolve.QueryExecutor
	mutationExecutor  resolve.MutationExecutor
	queryRewriter     resolve.QueryRewriter
	mutationRewriter  resolve.MutationRewriter
	withIntrospection bool
}

func (sa *schemaAdder) Rewrite(m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {

	// gqlgen, just trusts the validation and casts here
	// there must be a better way to do this?
	val, err := json.Marshal(m.ArgValue(schema.InputArgName))
	// deal with err
	input := &schemaInput{}
	err = json.Unmarshal(val, input)
	// deal with err

	schHandler, err := schema.NewHandler(input.Schema)
	if err != nil {
		return nil, nil, err
	}

	dgSchema := schHandler.DGSchema()

	if glog.V(3) {
		glog.Infof("Built Dgraph schema:\n\n%s\n", dgSchema)
	}

	op := &dgoapi.Operation{}
	op.Schema = dgSchema
	ctx := context.Background() // FIXME: should come in from existing context
	// plus auth token like live?
	err = sa.dgraph.Alter(ctx, op)
	if err != nil {
		return nil, nil, schema.GQLWrapf(err, "failed to write Dgraph schema")
	}

	// TODO: + work out the diff from the current schema

	resolverFactory :=
		resolve.NewResolverFactory(sa.queryRewriter, sa.mutationRewriter,
			sa.queryExecutor, sa.mutationExecutor)
	if sa.withIntrospection {
		resolverFactory.WithSchemaIntrospection()
	}

	doc, gqlErr := parser.ParseSchemas(validator.Prelude,
		&ast.Source{Input: schHandler.GQLSchema()})
	if gqlErr != nil {
		return nil, nil, errors.Wrap(gqlErr, "while parsing GraphQL schema")
	}

	gqlSchema, gqlErr := validator.ValidateSchemaDocument(doc)
	if gqlErr != nil {
		return nil, nil, errors.Wrap(gqlErr, "while validating GraphQL schema")
	}

	resolvers := resolve.New(schema.AsSchema(gqlSchema), resolverFactory)
	sa.gqlServer.ServeGQL(resolvers)

	return sa.baseMutationRewriter.Rewrite(m)
}

func (sa *schemaAdder) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	mutated map[string][]string) (*gql.GraphQuery, error) {
	return sa.baseMutationRewriter.FromMutationResult(mutation, assigned, mutated)
}

func NewAdminResolver(
	dg *dgo.Dgraph,
	gqlServer web.IServeGraphQL,
	// rewriters etc we should use when refreshing the main endpoint
	queryRewriter resolve.QueryRewriter,
	mutationRewriter resolve.MutationRewriter,
	queryExecutor resolve.QueryExecutor,
	mutationExecutor resolve.MutationExecutor) *resolve.RequestResolver {

	adminDoc, gqlErr := parser.ParseSchemas(validator.Prelude, &ast.Source{Input: adminSchema})
	if gqlErr != nil {
		panic(gqlErr)
	}

	adminSchema, gqlErr := validator.ValidateSchemaDocument(adminDoc)
	if gqlErr != nil {
		panic(gqlErr)
	}

	return resolve.New(
		schema.AsSchema(adminSchema),
		newAdminResolverFactory(
			dg,
			gqlServer,
			queryRewriter,
			mutationRewriter,
			queryExecutor,
			mutationExecutor))
}

func newAdminResolverFactory(
	dg *dgo.Dgraph,
	gqlServer web.IServeGraphQL,
	queryRewriter resolve.QueryRewriter,
	mutationRewriter resolve.MutationRewriter,
	queryExecutor resolve.QueryExecutor,
	mutationExecutor resolve.MutationExecutor) resolve.ResolverFactory {

	mutRewriter := resolve.NewMutationRewriter()

	return resolve.NewResolverFactory(
		resolve.NewQueryRewriter(),
		mutRewriter,
		resolve.DgoAsQueryExecutor(dg),
		resolve.DgoAsMutationExecutor(dg)).
		WithQueryResolver("health", &healthResolver{}).
		WithMutationResolver("addSchema",
			resolve.NewMutationResolver(
				&schemaAdder{
					gqlServer:            gqlServer,
					dgraph:               dg,
					baseMutationRewriter: mutRewriter,
					queryRewriter:        queryRewriter,
					mutationRewriter:     mutationRewriter,
					queryExecutor:        queryExecutor,
					mutationExecutor:     mutationExecutor,
					withIntrospection:    true},
				queryExecutor,
				mutationExecutor,
				resolve.StdMutationCompletion())).
		WithSchemaIntrospection()
}
