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
	"fmt"
	"sync"
	"time"

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/gql"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	badgerpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/graphql/web"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

const (
	errMsgServerNotReady = "Unavailable: Server not ready."

	errNoGraphQLSchema = "Not resolving %s. There's no GraphQL schema in Dgraph.  " +
		"Use the /admin API to add a GraphQL schema"
	errResolverNotFound = "%s was not executed because no suitable resolver could be found - " +
		"this indicates a resolver or validation bug " +
		"(Please let us know : https://github.com/dgraph-io/dgraph/issues)"

	gqlSchemaXidKey = "dgraph.graphql.xid"
	gqlSchemaXidVal = "dgraph.graphql.schema"
	gqlSchemaPred   = "dgraph.graphql.schema"

	// GraphQL schema for /admin endpoint.
	graphqlAdminSchema = `
	"""
	Data about the GraphQL schema being served by Dgraph.
	"""
	type GQLSchema @dgraph(type: "dgraph.graphql") {
		id: ID!

		"""
		Input schema (GraphQL types) that was used in the latest schema update.
		"""
		schema: String!  @dgraph(pred: "dgraph.graphql.schema")

		"""
		The GraphQL schema that was generated from the 'schema' field.  
		This is the schema that is being served by Dgraph at /graphql.
		"""
		generatedSchema: String!
	}

	"""
	A NodeState is the state of an individual node in the Dgraph cluster.
	"""
	type NodeState {

		"""
		Node type : either 'alpha' or 'zero'.
		"""
		instance: String

		"""
		Address of the node.
		"""
		address: String

		"""
		Node health status : either 'healthy' or 'unhealthy'.
		"""
		status: String

		"""
		The group this node belongs to in the Dgraph cluster.
		See : https://docs.dgraph.io/deploy/#cluster-setup.
		"""
		group: Int

		"""
		Version of the Dgraph binary.
		"""
		version: String

		"""
		Time in nanoseconds since the node started.
		"""
		uptime: Int

		"""
		Time in Unix epoch time that the node was last contacted by another Zero or Alpha node.
		"""
		lastEcho: Int

		"""
		List of ongoing operations in the background.
		"""
		ongoing: [String]

		"""
		List of predicates for which indexes are built in the background.
		"""
		indexing: [String]
	}

	type MembershipState {
		counter: Int
		groups: [ClusterGroup]
		zeros: [Member]
		maxLeaseId: Int
		maxTxnTs: Int
		maxRaftId: Int
		removed: [Member]
		cid: String
		license: License
	}

	type ClusterGroup {
		id: Int
		members: [Member]
		tablets: [Tablet]
		snapshotTs: Int
		checksum: Int
	}

	type Member {
		id: Int
		groupId: Int
		addr: String
		leader: Boolean
		amDead: Boolean
		lastUpdate: Int
		clusterInfoOnly: Boolean
		forceGroupId: Boolean
	}

	type Tablet {
		groupId: Int
		predicate: String
		force: Boolean
		space: Int
		remove: Boolean
		readOnly: Boolean
		moveTs: Int
	}

	type License {
		user: String
		maxNodes: Int
		expiryTs: Int
		enabled: Boolean
	}

	directive @dgraph(type: String, pred: String) on OBJECT | INTERFACE | FIELD_DEFINITION
	directive @id on FIELD_DEFINITION
	directive @secret(field: String!, pred: String) on OBJECT | INTERFACE


	type UpdateGQLSchemaPayload {
		gqlSchema: GQLSchema
	}

	input UpdateGQLSchemaInput {
		set: GQLSchemaPatch!
	}

	input GQLSchemaPatch {
		schema: String!
	}

	input ExportInput {
		format: String
	}

	type Response {
		code: String
		message: String
	}

	type ExportPayload {
		response: Response
	}

	type DrainingPayload {
		response: Response
	}

	type ShutdownPayload {
		response: Response
	}

	input ConfigInput {

		"""
		Estimated memory the LRU cache can take. Actual usage by the process would be 
		more than specified here. (default -1 means no set limit)
		"""
		lruMb: Float
	}

	type ConfigPayload {
		response: Response
	}

	` + adminTypes + `

	type Query {
		getGQLSchema: GQLSchema
		health: [NodeState]
		state: MembershipState

		` + adminQueries + `
	}

	type Mutation {

		"""
		Update the Dgraph cluster to serve the input schema.  This may change the GraphQL
		schema, the types and predicates in the Dgraph schema, and cause indexes to be recomputed.
		"""
		updateGQLSchema(input: UpdateGQLSchemaInput!) : UpdateGQLSchemaPayload

		"""
		Starts an export of all data in the cluster.  Export format should be 'rdf' (the default
		if no format is given), or 'json'.
		See : https://docs.dgraph.io/deploy/#export-database
		"""
		export(input: ExportInput!): ExportPayload

		"""
		Set (or unset) the cluster draining mode.  In draining mode no further requests are served.
		"""
		draining(enable: Boolean): DrainingPayload

		"""
		Shutdown this node.
		"""
		shutdown: ShutdownPayload

		"""
		Alter the node's config.
		"""
		config(input: ConfigInput!): ConfigPayload

		` + adminMutations + `
	}
 `
)

type gqlSchema struct {
	ID              string `json:"id,omitempty"`
	Schema          string `json:"schema,omitempty"`
	GeneratedSchema string
}

type adminServer struct {
	rf       resolve.ResolverFactory
	resolver *resolve.RequestResolver

	// The mutex that locks schema update operations
	mux sync.Mutex

	// The GraphQL server that's being admin'd
	gqlServer web.IServeGraphQL

	schema *gqlSchema

	// When the schema changes, we use these to create a new RequestResolver for
	// the main graphql endpoint (gqlServer) and thus refresh the API.
	fns               *resolve.ResolverFns
	withIntrospection bool
}

// NewServers initializes the GraphQL servers.  It sets up an empty server for the
// main /graphql endpoint and an admin server.  The result is mainServer, adminServer.
func NewServers(withIntrospection bool, closer *y.Closer) (web.IServeGraphQL, web.IServeGraphQL) {
	gqlSchema, err := schema.FromString("")
	if err != nil {
		x.Panic(err)
	}

	resolvers := resolve.New(gqlSchema, resolverFactoryWithErrorMsg(errNoGraphQLSchema))
	mainServer := web.NewServer(resolvers)

	fns := &resolve.ResolverFns{
		Qrw: resolve.NewQueryRewriter(),
		Arw: resolve.NewAddRewriter,
		Urw: resolve.NewUpdateRewriter,
		Drw: resolve.NewDeleteRewriter(),
	}
	adminResolvers := newAdminResolver(mainServer, fns, withIntrospection, closer)
	adminServer := web.NewServer(adminResolvers)

	return mainServer, adminServer
}

// newAdminResolver creates a GraphQL request resolver for the /admin endpoint.
func newAdminResolver(
	gqlServer web.IServeGraphQL,
	fns *resolve.ResolverFns,
	withIntrospection bool,
	closer *y.Closer) *resolve.RequestResolver {

	adminSchema, err := schema.FromString(graphqlAdminSchema)
	if err != nil {
		x.Panic(err)
	}

	rf := newAdminResolverFactory()

	server := &adminServer{
		rf:                rf,
		resolver:          resolve.New(adminSchema, rf),
		gqlServer:         gqlServer,
		fns:               fns,
		withIntrospection: withIntrospection,
	}

	prefix := x.DataKey(gqlSchemaPred, 0)
	// Remove uid from the key, to get the correct prefix
	prefix = prefix[:len(prefix)-8]
	// Listen for graphql schema changes in group 1.
	go worker.SubscribeForUpdates([][]byte{prefix}, func(kvs *badgerpb.KVList) {
		// Last update contains the latest value. So, taking the last update.
		lastIdx := len(kvs.GetKv()) - 1
		kv := kvs.GetKv()[lastIdx]

		glog.Infof("Updating GraphQL schema from subscription.")

		// Unmarshal the incoming posting list.
		pl := &pb.PostingList{}
		err := pl.Unmarshal(kv.GetValue())
		if err != nil {
			glog.Errorf("Unable to unmarshal the posting list for graphql schema update %s", err)
			return
		}

		// There should be only one posting.
		if len(pl.Postings) != 1 {
			glog.Errorf("Only one posting is expected in the graphql schema posting list but got %d",
				len(pl.Postings))
			return
		}

		pk, err := x.Parse(kv.GetKey())
		if err != nil {
			glog.Errorf("Unable to find uid of updated schema %s", err)
			return
		}

		newSchema := &gqlSchema{
			ID:     fmt.Sprintf("%#x", pk.Uid),
			Schema: string(pl.Postings[0].Value),
		}

		gqlSchema, err := generateGQLSchema(newSchema)
		if err != nil {
			glog.Errorf("Error processing GraphQL schema: %s.  ", err)
			return
		}

		glog.Infof("Successfully updated GraphQL schema. Serving New GraphQL API.")

		server.mux.Lock()
		defer server.mux.Unlock()

		server.schema = newSchema
		server.resetSchema(*gqlSchema)
	}, 1, closer)

	go server.initServer()

	return server.resolver
}

func newAdminResolverFactory() resolve.ResolverFactory {

	adminMutationResolvers :=
		map[string]func(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool){
			"backup":   resolveBackup,
			"config":   resolveConfig,
			"draining": resolveExport,
			"export":   resolveExport,
			"login":    resolveLogin,
			"restore":  resolveRestore,
			"shutdown": resolveShutdown,
		}

	rf := resolverFactoryWithErrorMsg(errResolverNotFound).
		WithQueryResolver("health", func(q schema.Query) resolve.QueryResolver {
			health := &healthResolver{}

			return resolve.NewQueryResolver(
				health,
				health,
				resolve.StdQueryCompletion())
		}).
		WithQueryResolver("state", func(q schema.Query) resolve.QueryResolver {
			state := &stateResolver{}

			return resolve.NewQueryResolver(
				state,
				state,
				resolve.StdQueryCompletion())
		}).
		WithMutationResolver("updateGQLSchema", func(m schema.Mutation) resolve.MutationResolver {
			return resolve.MutationResolverFunc(
				func(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
					return &resolve.Resolved{Err: errors.Errorf(errMsgServerNotReady), Field: m},
						false
				})
		}).
		WithQueryResolver("getGQLSchema", func(q schema.Query) resolve.QueryResolver {
			return resolve.QueryResolverFunc(
				func(ctx context.Context, query schema.Query) *resolve.Resolved {
					return &resolve.Resolved{Err: errors.Errorf(errMsgServerNotReady), Field: q}
				})
		})

	for op, fn := range adminMutationResolvers {
		rf.WithMutationResolver(op, func(m schema.Mutation) resolve.MutationResolver {
			return resolve.MutationResolverFunc(fn)
		})
	}

	rf.WithSchemaIntrospection()

	return rf
}

func upsertEmptyGQLSchema() (*gqlSchema, error) {
	varName := "GQLSchema"
	gqlType := "dgraph.graphql"

	qry := &gql.GraphQuery{
		Attr: varName,
		Var:  varName,
		Func: &gql.Function{
			Name: "eq",
			Args: []gql.Arg{
				{Value: gqlSchemaXidKey},
				{Value: fmt.Sprintf("%q", gqlSchemaXidVal)},
			},
		},
		Filter: &gql.FilterTree{
			Func: &gql.Function{
				Name: "type",
				Args: []gql.Arg{{Value: gqlType}},
			},
		},
		Children: []*gql.GraphQuery{{Attr: "uid"}, {Attr: gqlSchemaPred}},
	}

	mutations := []*dgoapi.Mutation{
		{
			SetJson: []byte(fmt.Sprintf(`
			{
				"uid": "_:%s",
				"dgraph.type": ["%s"],
				"%s": "%s",
				"%s": ""
			}`, varName, gqlType, gqlSchemaXidKey, gqlSchemaXidVal, gqlSchemaPred)),
			Cond: fmt.Sprintf(`@if(eq(len(%s),0))`, varName),
		},
	}

	assigned, result, err := resolve.AdminMutationExecutor().Mutate(context.Background(), qry,
		mutations)
	if err != nil {
		return nil, err
	}

	// the Alpha which created the gql schema node will get the uid here
	uid, ok := assigned[varName]
	if ok {
		return &gqlSchema{ID: uid}, nil
	}

	// the Alphas which didn't create the gql schema node, will get the uid here.
	gqlSchemaNode := result[varName].([]interface{})[0].(map[string]interface{})
	return &gqlSchema{
		ID:     gqlSchemaNode["uid"].(string),
		Schema: gqlSchemaNode[gqlSchemaPred].(string),
	}, nil
}

func generateGQLSchema(sch *gqlSchema) (*schema.Schema, error) {
	schHandler, err := schema.NewHandler(sch.Schema)
	if err != nil {
		return nil, err
	}

	sch.GeneratedSchema = schHandler.GQLSchema()
	generatedSchema, err := schema.FromString(sch.GeneratedSchema)
	if err != nil {
		return nil, err
	}

	return &generatedSchema, nil
}

func (as *adminServer) initServer() {
	// Nothing else should be able to lock before here.  The admin resolvers aren't yet
	// set up (they all just error), so we will obtain the lock here without contention.
	// We then setup the admin resolvers and they must wait until we are done before the
	// first admin calls will go through.
	as.mux.Lock()
	defer as.mux.Unlock()

	// It takes a few seconds for the Dgraph cluster to be up and running.
	// Before that, trying to read the GraphQL schema will result in error:
	// "Please retry again, server is not ready to accept requests."
	// 5 seconds is a pretty reliable wait for a fresh instance to read the
	// schema on a first try.
	waitFor := 5 * time.Second

	for {
		<-time.After(waitFor)

		sch, err := upsertEmptyGQLSchema()
		if err != nil {
			glog.Infof("Error reading GraphQL schema: %s.", err)
			continue
		}

		as.schema = sch
		// adding the actual resolvers for updateGQLSchema and getGQLSchema only after server has ID
		as.addConnectedAdminResolvers()

		if sch.Schema == "" {
			glog.Infof("No GraphQL schema in Dgraph; serving empty GraphQL API")
			break
		}

		generatedSchema, err := generateGQLSchema(sch)
		if err != nil {
			glog.Infof("Error processing GraphQL schema: %s.", err)
			break
		}

		glog.Infof("Successfully loaded GraphQL schema.  Serving GraphQL API.")

		as.resetSchema(*generatedSchema)

		break
	}
}

// addConnectedAdminResolvers sets up the real resolvers
func (as *adminServer) addConnectedAdminResolvers() {

	qryRw := resolve.NewQueryRewriter()
	updRw := resolve.NewUpdateRewriter()
	qryExec := resolve.DgraphAsQueryExecutor()
	mutExec := resolve.DgraphAsMutationExecutor()

	as.fns.Qe = qryExec
	as.fns.Me = mutExec

	as.rf.WithMutationResolver("updateGQLSchema",
		func(m schema.Mutation) resolve.MutationResolver {
			updResolver := &updateSchemaResolver{
				admin:                as,
				baseMutationRewriter: updRw,
				baseMutationExecutor: mutExec,
			}

			return resolve.NewMutationResolver(
				updResolver,
				updResolver,
				updResolver,
				resolve.StdMutationCompletion(m.Name()))
		}).
		WithQueryResolver("getGQLSchema",
			func(q schema.Query) resolve.QueryResolver {
				getResolver := &getSchemaResolver{
					admin: as,
				}

				return resolve.NewQueryResolver(
					getResolver,
					getResolver,
					resolve.StdQueryCompletion())
			}).
		WithQueryResolver("queryGroup",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(
					qryRw,
					qryExec,
					resolve.StdQueryCompletion())
			}).
		WithQueryResolver("queryUser",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(
					qryRw,
					qryExec,
					resolve.StdQueryCompletion())
			}).
		WithQueryResolver("getGroup",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(
					qryRw,
					qryExec,
					resolve.StdQueryCompletion())
			}).
		WithQueryResolver("getCurrentUser",
			func(q schema.Query) resolve.QueryResolver {
				cuResolver := &currentUserResolver{
					baseRewriter: qryRw,
				}

				return resolve.NewQueryResolver(
					cuResolver,
					qryExec,
					resolve.StdQueryCompletion())
			}).
		WithQueryResolver("getUser",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(
					qryRw,
					qryExec,
					resolve.StdQueryCompletion())
			}).
		WithMutationResolver("addUser",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewMutationResolver(
					resolve.NewAddRewriter(),
					resolve.DgraphAsQueryExecutor(),
					resolve.DgraphAsMutationExecutor(),
					resolve.StdMutationCompletion(m.Name()))
			}).
		WithMutationResolver("addGroup",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewMutationResolver(
					NewAddGroupRewriter(),
					resolve.DgraphAsQueryExecutor(),
					resolve.DgraphAsMutationExecutor(),
					resolve.StdMutationCompletion(m.Name()))
			}).
		WithMutationResolver("updateUser",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewMutationResolver(
					resolve.NewUpdateRewriter(),
					resolve.DgraphAsQueryExecutor(),
					resolve.DgraphAsMutationExecutor(),
					resolve.StdMutationCompletion(m.Name()))
			}).
		WithMutationResolver("updateGroup",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewMutationResolver(
					NewUpdateGroupRewriter(),
					resolve.DgraphAsQueryExecutor(),
					resolve.DgraphAsMutationExecutor(),
					resolve.StdMutationCompletion(m.Name()))
			}).
		WithMutationResolver("deleteUser",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewMutationResolver(
					resolve.NewDeleteRewriter(),
					resolve.NoOpQueryExecution(),
					resolve.DgraphAsMutationExecutor(),
					resolve.StdDeleteCompletion(m.Name()))
			}).
		WithMutationResolver("deleteGroup",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewMutationResolver(
					resolve.NewDeleteRewriter(),
					resolve.NoOpQueryExecution(),
					resolve.DgraphAsMutationExecutor(),
					resolve.StdDeleteCompletion(m.Name()))
			})
}

func resolverFactoryWithErrorMsg(msg string) resolve.ResolverFactory {
	errFunc := func(name string) error { return errors.Errorf(msg, name) }
	qErr :=
		resolve.QueryResolverFunc(func(ctx context.Context, query schema.Query) *resolve.Resolved {
			return &resolve.Resolved{Err: errFunc(query.ResponseName()), Field: query}
		})

	mErr := resolve.MutationResolverFunc(
		func(ctx context.Context, mutation schema.Mutation) (*resolve.Resolved, bool) {
			return &resolve.Resolved{Err: errFunc(mutation.ResponseName()), Field: mutation}, false
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
}

func response(code, msg string) map[string]interface{} {
	return map[string]interface{}{
		"response": map[string]interface{}{"code": code, "message": msg}}
}

func emptyResult(f schema.Field, err error) *resolve.Resolved {
	return &resolve.Resolved{
		Data:  map[string]interface{}{f.Name(): nil},
		Field: f,
		Err:   err,
	}
}
