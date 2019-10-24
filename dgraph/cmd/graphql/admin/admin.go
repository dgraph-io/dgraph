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
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgo/v2"
	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/resolve"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/web"
	"github.com/dgraph-io/dgraph/x"
)

const (
	// The schema fragment that's needed in Dgraph to operate the GraphQL layer.
	// FIXME: will change to this once we have the edge names done
	// dgraphAdminSchema = `
	// type dgraph.graphql {
	// 	dgraph.graphql.schema
	// 	dgraph.graphql.date
	// }
	// dgraph.graphql.schema string .
	// dgraph.graphql.date @index(day) .
	// `
	dgraphAdminSchema = `
	type Schema {
		Schema.schema
		Schema.date
	}
	Schema.schema: string .
	Schema.date: dateTime @index(day) .
	`

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

// ConnectionSettings is the settings used in setting up the dgo connection from
// GraphQL Layer -> Dgraph.
type ConnectionSettings struct {
	Alphas         string
	TlScfg         *tls.Config
	UseCompression bool
}

type schemaDef struct {
	Schema string    `json:"schema,omitempty"`
	Date   time.Time `json:"date,omitempty"`
}

type adminServer struct {
	settings *ConnectionSettings
	rf       resolve.ResolverFactory
	resolver *resolve.RequestResolver
	status   healthStatus

	// The mutex that locks schema update operations
	mux sync.Mutex

	// the Dgraph that gets its schema changed
	dgraph *dgo.Dgraph

	// The GraphQL server that's being admin'd
	gqlServer web.IServeGraphQL

	// When the schema changes, we use these to create a new RequestResolver for
	// the main graphql endpoint (gqlServer) and thus refresh the API.
	fns               *resolve.ResolverFns
	withIntrospection bool
}

// NewAdminResolver creates a GraphQL request resolver for the /admin endpoint.
func NewAdminResolver(
	settings *ConnectionSettings,
	gqlServer web.IServeGraphQL,
	fns *resolve.ResolverFns,
	introspection bool) *resolve.RequestResolver {

	adminSchema, err := schema.FromString(adminSchema)
	if err != nil {
		panic(err)
	}

	rf := newAdminResolverFactory()

	admin := &adminServer{
		settings:          settings,
		rf:                rf,
		resolver:          resolve.New(adminSchema, rf),
		status:            errNoConnection,
		gqlServer:         gqlServer,
		fns:               fns,
		withIntrospection: introspection,
	}

	go admin.pollForConnection()

	return admin.resolver
}

func newAdminResolverFactory() resolve.ResolverFactory {

	errResult := errors.Errorf("Unavailable: Server has not yet connected to Dgraph.")

	rf := resolve.NewResolverFactory().
		WithQueryResolver("health",
			func(q schema.Query) resolve.QueryResolver {
				health := &healthResolver{
					status: errNoConnection,
				}

				return resolve.NewQueryResolver(
					health,
					health,
					resolve.StdQueryCompletion())
			}).
		WithMutationResolver("addSchema", func(m schema.Mutation) resolve.MutationResolver {
			return resolve.MutationResolverFunc(
				func(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
					return &resolve.Resolved{Err: errResult}, false
				})
		}).
		WithQueryResolver("querySchema", func(q schema.Query) resolve.QueryResolver {
			return resolve.QueryResolverFunc(
				func(ctx context.Context, query schema.Query) *resolve.Resolved {
					return &resolve.Resolved{Err: errResult}
				})
		}).
		WithSchemaIntrospection()

	return rf
}

func (as *adminServer) pollForConnection() {

	var waitFor time.Duration
	for {
		<-time.After(waitFor)
		waitFor = 10 * time.Second

		glog.Infof("Trying to connect to Dgraph at %s", as.settings.Alphas)
		dgraphClient, disconnect, err := connect(as.settings)
		if err != nil {
			glog.Infof("Failed to connect to Dgraph: %s.  Trying again in %f seconds",
				err, waitFor.Seconds())
			continue
		}
		glog.Infof("Established Dgraph connection")

		err = ensureAdminSchemaExists(dgraphClient)
		if err != nil {
			glog.Infof("Failed checking GraphQL admin schema: %s.  Trying again in %f seconds",
				err, waitFor.Seconds())
			disconnect()
			continue
		}

		as.dgraph = dgraphClient

		as.addConnectedAdminResolvers(dgraphClient)

		as.status = noGraphQLSchema

		sch, err := getCurrentGraphQLSchema(as.resolver)
		if err != nil {
			glog.Infof("Error reading GraphQL schema: %s.  "+
				"Admin server is connected, but no GraphQL schema is being served.", err)
		} else if sch == nil {
			glog.Infof("No GraphQL schema in Dgraph; serving empty GraphQL API")
		}

		if sch == nil {
			break
		}

		schHandler, err := schema.NewHandler(sch.Schema)
		if err != nil {
			glog.Infof("Error processing GraphQL schema: %s.  "+
				"Admin server is connected, but no GraphQL schema is being served.", err)
			break
		}

		gqlSchema, err := schema.FromString(schHandler.GQLSchema())
		if err != nil {
			glog.Infof("Error processing GraphQL schema: %s.  "+
				"Admin server is connected, but no GraphQL schema is being served.", err)
			break
		}

		glog.Infof("Successfully loaded GraphQL schema.  Now serving GraphQL API.")

		as.status = healthy
		as.resetSchema(gqlSchema)

		break
	}
}

func ensureAdminSchemaExists(dg *dgo.Dgraph) error {
	// We could query for existing schema and only alter if it's not there, but
	// this has same effect.  We might eventually have to migrate old versions of the
	// metadata here.
	return dg.Alter(context.Background(), &dgoapi.Operation{Schema: dgraphAdminSchema})
}

// addConnectedAdminResolvers sets up the real resolvers now that there's a connection
// to Dgraph (before there's a connection, addSchema etc just return errors).
func (as *adminServer) addConnectedAdminResolvers(dg *dgo.Dgraph) {

	qryRw := resolve.NewQueryRewriter()
	mutRw := resolve.NewMutationRewriter()
	qryExec := resolve.DgoAsQueryExecutor(dg)
	mutExec := resolve.DgoAsMutationExecutor(dg)

	as.fns.Qe = qryExec
	as.fns.Me = mutExec

	as.rf.WithQueryResolver("health",
		func(q schema.Query) resolve.QueryResolver {
			health := &healthResolver{
				status: as.status,
			}

			return resolve.NewQueryResolver(
				health,
				health,
				resolve.StdQueryCompletion())
		}).
		WithMutationResolver("addSchema",
			func(m schema.Mutation) resolve.MutationResolver {
				addResolver := &addSchemaResolver{
					admin:                as,
					baseMutationRewriter: mutRw,
					baseMutationExecutor: mutExec}

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
			})
}

func getCurrentGraphQLSchema(r *resolve.RequestResolver) (*schemaDef, error) {
	req := &schema.Request{
		Query: `query { querySchema(order: { desc: date }, first: 1) { schema date } }`}
	resp := r.Resolve(context.Background(), req)
	if len(resp.Errors) > 0 {
		return nil, resp.Errors
	}

	var result struct {
		QuerySchema []*schemaDef
	}

	if resp.Data.Len() > 0 {
		err := json.Unmarshal(resp.Data.Bytes(), &result)
		if err != nil {
			return nil, err
		}
	}

	if len(result.QuerySchema) == 0 {
		return nil, nil
	}
	return result.QuerySchema[0], nil
}

func (as *adminServer) resetSchema(gqlSchema schema.Schema) {

	resolverFactory := resolve.NewResolverFactory().WithConventionResolvers(gqlSchema, as.fns)
	if as.withIntrospection {
		resolverFactory.WithSchemaIntrospection()
	}

	as.gqlServer.ServeGQL(resolve.New(gqlSchema, resolverFactory))

	as.status = healthy
}

func connect(cs *ConnectionSettings) (*dgo.Dgraph, func(), error) {
	var clients []dgoapi.DgraphClient
	disconnect := func() {}

	ds := strings.Split(cs.Alphas, ",")
	for _, d := range ds {
		conn, err := x.SetupConnection(d, cs.TlScfg, cs.UseCompression)
		if err != nil {
			disconnect()
			return nil, nil, fmt.Errorf("couldn't connect to %s, %s", d, err)
		}
		disconnect = func(dis func()) func() {
			return func() { dis(); conn.Close() }
		}(disconnect)

		clients = append(clients, dgoapi.NewDgraphClient(conn))
	}

	return dgo.NewDgraphClient(clients...), disconnect, nil
}
