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
	"time"

	"github.com/dgraph-io/dgo/v2"
	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/web"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/golang/glog"
	"github.com/pkg/errors"
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

type healthResolver struct{}

func (hr *healthResolver) Resolve(ctx context.Context, query schema.Query) *resolve.Resolved {
	// TODO: The actual algorithm for determining health goes here...
	return &resolve.Resolved{Data: []byte(`"message" : "I'm alive"`)}
}

// A addSchemaResolver serves as the mutation rewriter and executor in handling
// the addSchema mutation.
type addSchemaResolver struct {
	// the Dgraph that gets its schema changed
	dgraph *dgo.Dgraph

	// schema that is generated from the mutation input
	newGQLSchema    schema.Schema
	newDgraphSchema string

	// The underlying executor and rewriter that persist the schema into Dgraph as
	// GraphQL metadata
	baseMutationRewriter resolve.MutationRewriter
	baseMutationExecutor resolve.MutationExecutor

	// The GraphQL server that's being admin'd
	gqlServer web.IServeGraphQL

	// When the schema changes, we use these to create a new RequestResolver for
	// the main graphql endpoint (gqlServer) and thus refresh the API.
	fns               *resolve.ResolverFns
	withIntrospection bool
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
	// The args ^^ are the rewriters and executors used by the endpoint being admin'd.
	// These are the ones used by the admin endpoint itself.
	mutRw := resolve.NewMutationRewriter()
	qryRw := resolve.NewQueryRewriter()
	qryExec := resolve.DgoAsQueryExecutor(dg)
	mutExec := resolve.DgoAsMutationExecutor(dg)

	return resolve.NewResolverFactory().
		WithQueryResolver("health",
			func(q schema.Query) resolve.QueryResolver { return &healthResolver{} }).
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

func (asr *addSchemaResolver) Rewrite(m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {
	sch, err := getSchemaInput(m)
	if err != nil {
		return nil, nil, err
	}

	schHandler, err := schema.NewHandler(sch)
	if err != nil {
		return nil, nil, err
	}

	asr.newGQLSchema, err = schema.FromString(schHandler.GQLSchema())
	if err != nil {
		return nil, nil, err
	}

	asr.newDgraphSchema = schHandler.DGSchema()

	m.SetArgTo(schema.InputArgName, map[string]interface{}{"schema": sch, "date": time.Now()})
	return asr.baseMutationRewriter.Rewrite(m)
}

func (asr *addSchemaResolver) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	mutated map[string][]string) (*gql.GraphQuery, error) {
	return asr.baseMutationRewriter.FromMutationResult(mutation, assigned, mutated)
}

func (asr *addSchemaResolver) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string][]string, error) {

	assigned, mutated, err := asr.baseMutationExecutor.Mutate(ctx, query, mutations)
	if err != nil {
		return nil, nil, err
	}

	if glog.V(3) {
		glog.Infof("Altering Dgraph schema:\n\n%s\n", asr.newDgraphSchema)
	}

	err = asr.dgraph.Alter(ctx, &dgoapi.Operation{Schema: asr.newDgraphSchema})
	if err != nil {
		return nil, nil, schema.GQLWrapf(err,
			"succeeded in saving GraphQL schema but failed to alter Dgraph schema "+
				"(you should retry)")
	}

	resolverFactory := resolve.NewResolverFactory().
		WithConventionResolvers(asr.newGQLSchema, asr.fns)

	if asr.withIntrospection {
		resolverFactory.WithSchemaIntrospection()
	}

	resolvers := resolve.New(asr.newGQLSchema, resolverFactory)
	asr.gqlServer.ServeGQL(resolvers)

	return assigned, mutated, nil
}

func getSchemaInput(m schema.Mutation) (string, error) {
	input, ok := m.ArgValue(schema.InputArgName).(map[string]interface{})
	if !ok {
		return "", errors.New("couldn't get input argument")
	}

	return input["schema"].(string), nil
}
