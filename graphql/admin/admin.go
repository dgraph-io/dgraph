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
	"encoding/json"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	badgerpb "github.com/dgraph-io/badger/v2/pb"
	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/web"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

const (
	errMsgServerNotReady = "Unavailable: Server has not yet connected to Dgraph."

	errNoGraphQLSchema = "Not resolving %s. There's no GraphQL schema in Dgraph.  " +
		"Use the /admin API to add a GraphQL schema"
	errResolverNotFound = "%s was not executed because no suitable resolver could be found - " +
		"this indicates a resolver or validation bug " +
		"(Please let us know : https://github.com/dgraph-io/dgraph/issues)"

	// The schema fragment that's needed in Dgraph to operate the GraphQL layer.
	// FIXME: dgraphAdminSchema will change to this once we have @dgraph(pred: "...")
	// dgraphAdminSchema = `
	// type dgraph.graphql {
	// 	dgraph.graphql.schema
	// 	dgraph.graphql.date
	// }
	// dgraph.graphql.schema string .
	// dgraph.graphql.date @index(day) .
	// `
	dgraphAdminSchema = `
	type dgraph.graphql {
		dgraph.graphql.schema
		dgraph.graphql.date
	}`
	// GraphQL schema for /admin endpoint.
	//
	// Eventually we should generate this from just the types definition.
	// But for now, that would add too much into the schema, so this is
	// hand crafted to be one of our schemas so we can pass it into the
	// pipeline.
	graphqlAdminSchema = `
 type Schema @dgraph(type: "dgraph.graphql") {
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

 directive @dgraph(type: String, pred: String) on OBJECT | INTERFACE | FIELD_DEFINITION

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

type adminServer struct {
	rf       resolve.ResolverFactory
	resolver *resolve.RequestResolver
	status   healthStatus

	// The mutex that locks schema update operations
	mux sync.Mutex

	// The GraphQL server that's being admin'd
	gqlServer web.IServeGraphQL

	// When the schema changes, we use these to create a new RequestResolver for
	// the main graphql endpoint (gqlServer) and thus refresh the API.
	fns               *resolve.ResolverFns
	withIntrospection bool
}

// NewServers initializes the GraphQL servers.  It sets up an empty server for the
// main /graphql endpoint and an admin server.  The result is mainServer, adminServer.
func NewServers(withIntrospection bool) (web.IServeGraphQL, web.IServeGraphQL) {

	gqlSchema, err := schema.FromString("")
	if err != nil {
		panic(err)
	}

	resolvers := resolve.New(gqlSchema, resolverFactoryWithErrorMsg(errNoGraphQLSchema))
	mainServer := web.NewServer(resolvers)

	fns := &resolve.ResolverFns{
		Qrw: resolve.NewQueryRewriter(),
		Arw: resolve.NewAddRewriter,
		Urw: resolve.NewUpdateRewriter,
		Drw: resolve.NewDeleteRewriter(),
	}
	adminResolvers := newAdminResolver(mainServer, fns, withIntrospection)
	adminServer := web.NewServer(adminResolvers)

	return mainServer, adminServer
}

// newAdminResolver creates a GraphQL request resolver for the /admin endpoint.
func newAdminResolver(
	gqlServer web.IServeGraphQL,
	fns *resolve.ResolverFns,
	withIntrospection bool) *resolve.RequestResolver {

	adminSchema, err := schema.FromString(graphqlAdminSchema)
	if err != nil {
		panic(err)
	}

	rf := newAdminResolverFactory()

	server := &adminServer{
		rf:                rf,
		resolver:          resolve.New(adminSchema, rf),
		status:            errNoConnection,
		gqlServer:         gqlServer,
		fns:               fns,
		withIntrospection: withIntrospection,
	}

	prefix := x.DataKey("dgraph.graphql.schema", 0)
	// Remove uid from the key, to get the correct prefix
	prefix = prefix[:len(prefix)-8]
	// Listen for graphql schema changes in group 1.
	go worker.SubscribeForUpdates([][]byte{prefix}, func(kvs *badgerpb.KVList) {
		// Last update contains the latest value. So, taking the last update.
		lastIdx := len(kvs.GetKv()) - 1
		kv := kvs.GetKv()[lastIdx]

		// Unmarshal the incoming posting list.
		pl := &pb.PostingList{}
		err := pl.Unmarshal(kv.GetValue())
		if err != nil {
			glog.Errorf("Unable to marshal the psoting list for graphql schema update %s", err)
		}

		// There should be only one posting.
		if len(pl.Postings) != 1 {
			glog.Errorf("Only one posting is expected in the graphql schema posting list but got %d",
				len(pl.Postings))
		}
		graphqlSchema := string(pl.Postings[0].Value)
		glog.Info("Updating graphql schema")

		schHandler, err := schema.NewHandler(graphqlSchema)
		if err != nil {
			glog.Errorf("Error processing GraphQL schema: %s.  "+
				"Admin server is connected, but no GraphQL schema is being served.", err)
			return
		}

		gqlSchema, err := schema.FromString(schHandler.GQLSchema())
		if err != nil {
			glog.Errorf("Error processing GraphQL schema: %s.  "+
				"Admin server is connected, but no GraphQL schema is being served.", err)
			return
		}

		glog.Infof("Successfully loaded GraphQL schema.  Now serving GraphQL API.")

		server.mux.Lock()
		defer server.mux.Unlock()
		server.status = healthy
		server.resetSchema(gqlSchema)
	}, 1)

	go server.initServer()

	return server.resolver
}

func newAdminResolverFactory() resolve.ResolverFactory {
	rf := resolverFactoryWithErrorMsg(errResolverNotFound).
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
					return &resolve.Resolved{Err: errors.Errorf(errMsgServerNotReady)}, false
				})
		}).
		WithQueryResolver("querySchema", func(q schema.Query) resolve.QueryResolver {
			return resolve.QueryResolverFunc(
				func(ctx context.Context, query schema.Query) *resolve.Resolved {
					return &resolve.Resolved{Err: errors.Errorf(errMsgServerNotReady)}
				})
		}).
		WithSchemaIntrospection()

	return rf
}

func (as *adminServer) initServer() {
	var waitFor time.Duration
	for {
		<-time.After(waitFor)
		waitFor = 10 * time.Second

		err := checkAdminSchemaExists()
		if err != nil {
			if glog.V(3) {
				glog.Infof("Failed checking GraphQL admin schema: %s.  Trying again in %f seconds",
					err, waitFor.Seconds())
			}
			continue
		}

		// Nothing else should be able to lock before here.  The admin resolvers aren't yet
		// set up (they all just error), so we will obtain the lock here without contention.
		// We then setup the admin resolvers and they must wait until we are done before the
		// first admin calls will go through.
		as.mux.Lock()
		defer as.mux.Unlock()

		as.addConnectedAdminResolvers()

		as.status = noGraphQLSchema

		sch, err := getCurrentGraphQLSchema(as.resolver)
		if err != nil {
			glog.Infof("Error reading GraphQL schema: %s.  "+
				"Admin server is connected, but no GraphQL schema is being served.", err)
			break
		} else if sch == nil {
			glog.Infof("No GraphQL schema in Dgraph; serving empty GraphQL API")
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

func checkAdminSchemaExists() error {
	// We could query for existing schema and only alter if it's not there, but
	// this has same effect.  We might eventually have to migrate old versions of the
	// metadata here.
	_, err := (&edgraph.Server{}).Alter(context.Background(),
		&dgoapi.Operation{Schema: dgraphAdminSchema})
	return err
}

// addConnectedAdminResolvers sets up the real resolvers
func (as *adminServer) addConnectedAdminResolvers() {

	qryRw := resolve.NewQueryRewriter()
	mutRw := resolve.NewAddRewriter()
	qryExec := resolve.DgraphAsQueryExecutor()
	mutExec := resolve.DgraphAsMutationExecutor()

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

func resolverFactoryWithErrorMsg(msg string) resolve.ResolverFactory {
	errFunc := func(name string) error { return errors.Errorf(msg, name) }
	qErr :=
		resolve.QueryResolverFunc(func(ctx context.Context, query schema.Query) *resolve.Resolved {
			return &resolve.Resolved{Err: errFunc(query.ResponseName())}
		})

	mErr := resolve.MutationResolverFunc(
		func(ctx context.Context, mutation schema.Mutation) (*resolve.Resolved, bool) {
			return &resolve.Resolved{Err: errFunc(mutation.ResponseName())}, false
		})

	return resolve.NewResolverFactory(qErr, mErr)
}

func (as *adminServer) resetSchema(gqlSchema schema.Schema) {

	resolverFactory := resolverFactoryWithErrorMsg(errResolverNotFound).
		WithConventionResolvers(gqlSchema, as.fns)
	if as.withIntrospection {
		resolverFactory.WithSchemaIntrospection()
	}

	as.gqlServer.ServeGQL(resolve.New(gqlSchema, resolverFactory))

	as.status = healthy
}
