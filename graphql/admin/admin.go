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
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	badgerpb "github.com/dgraph-io/badger/v3/pb"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgraph-io/ristretto/z"
)

const (
	errMsgServerNotReady = "Unavailable: Server not ready."

	errNoGraphQLSchema = "Not resolving %s. There's no GraphQL schema in Dgraph.  " +
		"Use the /admin API to add a GraphQL schema"
	errResolverNotFound = "%s was not executed because no suitable resolver could be found - " +
		"this indicates a resolver or validation bug. Please let us know by filing an issue."

	// GraphQL schema for /admin endpoint.
	graphqlAdminSchema = `
	scalar DateTime

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
		See : https://dgraph.io/docs/deploy/#cluster-setup.
		"""
		group: String

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

		"""
		List of Enterprise Features that are enabled.
		"""
		ee_features: [String]
	}

	type MembershipState {
		counter: Int
		groups: [ClusterGroup]
		zeros: [Member]
		maxUID: Int
		maxNsID: Int
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
		"""
		Data format for the export, e.g. "rdf" or "json" (default: "rdf")
		"""
		format: String

		"""
		Namespace for the export in multi-tenant cluster. Users from guardians of galaxy can export
		all namespaces by passing a negative value or specific namespaceId to export that namespace.
		"""
		namespace: Int

		"""
		Destination for the export: e.g. Minio or S3 bucket or /absolute/path
		"""
		destination: String

		"""
		Access key credential for the destination.
		"""
		accessKey: String

		"""
		Secret key credential for the destination.
		"""
		secretKey: String

		"""
		AWS session token, if required.
		"""
		sessionToken: String

		"""
		Set to true to allow backing up to S3 or Minio bucket that requires no credentials.
		"""
		anonymous: Boolean
	}

	type Response {
		code: String
		message: String
	}

	type ExportPayload {
		response: Response
		exportedFiles: [String]
	}

	type DrainingPayload {
		response: Response
	}

	type ShutdownPayload {
		response: Response
	}

	input ConfigInput {
		"""
		Estimated memory the caches can take. Actual usage by the process would be
		more than specified here. The caches will be updated according to the
		cache_percentage flag.
		"""
		cacheMb: Float

		"""
		True value of logRequest enables logging of all the requests coming to alphas.
		False value of logRequest disables above.
		"""
		logRequest: Boolean
	}

	type ConfigPayload {
		response: Response
	}

	type Config {
		cacheMb: Float
	}

	` + adminTypes + `

	type Query {
		getGQLSchema: GQLSchema
		health: [NodeState]
		state: MembershipState
		config: Config
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
		See : https://dgraph.io/docs/deploy/#export-database
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

var (
	// guardianOfTheGalaxyQueryMWs are the middlewares which should be applied to queries served by
	// admin server for guardian of galaxy unless some exceptional behaviour is required
	guardianOfTheGalaxyQueryMWs = resolve.QueryMiddlewares{
		resolve.IpWhitelistingMW4Query,
		resolve.GuardianOfTheGalaxyAuthMW4Query,
		resolve.LoggingMWQuery,
	}
	// guardianOfTheGalaxyMutationMWs are the middlewares which should be applied to mutations
	// served by admin server for guardian of galaxy unless some exceptional behaviour is required
	guardianOfTheGalaxyMutationMWs = resolve.MutationMiddlewares{
		resolve.IpWhitelistingMW4Mutation,
		resolve.GuardianOfTheGalaxyAuthMW4Mutation,
		resolve.LoggingMWMutation,
	}
	// guardianOfTheGalaxyMutaionWithAclMWs are the middlewares which should be applied to mutations
	// served by the admin server for guardian of galaxy with ACL enabled.
	guardianOfTheGalaxyMutaionWithAclMWs = resolve.MutationMiddlewares{
		resolve.IpWhitelistingMW4Mutation,
		resolve.AclOnlyMW4Mutation,
		resolve.GuardianOfTheGalaxyAuthMW4Mutation,
		resolve.LoggingMWMutation,
	}
	// commonAdminQueryMWs are the middlewares which should be applied to queries served by admin
	// server unless some exceptional behaviour is required
	commonAdminQueryMWs = resolve.QueryMiddlewares{
		resolve.IpWhitelistingMW4Query, // good to apply ip whitelisting before Guardian auth
		resolve.GuardianAuthMW4Query,
		resolve.LoggingMWQuery,
	}
	// commonAdminMutationMWs are the middlewares which should be applied to mutations served by
	// admin server unless some exceptional behaviour is required
	commonAdminMutationMWs = resolve.MutationMiddlewares{
		resolve.IpWhitelistingMW4Mutation, // good to apply ip whitelisting before Guardian auth
		resolve.GuardianAuthMW4Mutation,
		resolve.LoggingMWMutation,
	}
	adminQueryMWConfig = map[string]resolve.QueryMiddlewares{
		"health":       {resolve.IpWhitelistingMW4Query, resolve.LoggingMWQuery}, // dgraph checks Guardian auth for health
		"state":        {resolve.IpWhitelistingMW4Query, resolve.LoggingMWQuery}, // dgraph checks Guardian auth for state
		"config":       commonAdminQueryMWs,
		"listBackups":  guardianOfTheGalaxyQueryMWs,
		"getGQLSchema": commonAdminQueryMWs,
		// for queries and mutations related to User/Group, dgraph handles Guardian auth,
		// so no need to apply GuardianAuth Middleware
		"queryUser":      {resolve.IpWhitelistingMW4Query, resolve.LoggingMWQuery},
		"queryGroup":     {resolve.IpWhitelistingMW4Query, resolve.LoggingMWQuery},
		"getUser":        {resolve.IpWhitelistingMW4Query, resolve.LoggingMWQuery},
		"getCurrentUser": {resolve.IpWhitelistingMW4Query, resolve.LoggingMWQuery},
		"getGroup":       {resolve.IpWhitelistingMW4Query, resolve.LoggingMWQuery},
	}
	adminMutationMWConfig = map[string]resolve.MutationMiddlewares{
		"backup":          guardianOfTheGalaxyMutationMWs,
		"config":          guardianOfTheGalaxyMutationMWs,
		"draining":        guardianOfTheGalaxyMutationMWs,
		"export":          commonAdminMutationMWs, // dgraph handles the export for other namespaces by guardian of galaxy
		"login":           {resolve.IpWhitelistingMW4Mutation, resolve.LoggingMWMutation},
		"restore":         guardianOfTheGalaxyMutationMWs,
		"shutdown":        guardianOfTheGalaxyMutationMWs,
		"updateGQLSchema": commonAdminMutationMWs,
		"addNamespace":    guardianOfTheGalaxyMutaionWithAclMWs,
		"deleteNamespace": guardianOfTheGalaxyMutaionWithAclMWs,
		"resetPassword":   guardianOfTheGalaxyMutaionWithAclMWs,
		// for queries and mutations related to User/Group, dgraph handles Guardian auth,
		// so no need to apply GuardianAuth Middleware
		"addUser":     {resolve.IpWhitelistingMW4Mutation, resolve.LoggingMWMutation},
		"addGroup":    {resolve.IpWhitelistingMW4Mutation, resolve.LoggingMWMutation},
		"updateUser":  {resolve.IpWhitelistingMW4Mutation, resolve.LoggingMWMutation},
		"updateGroup": {resolve.IpWhitelistingMW4Mutation, resolve.LoggingMWMutation},
		"deleteUser":  {resolve.IpWhitelistingMW4Mutation, resolve.LoggingMWMutation},
		"deleteGroup": {resolve.IpWhitelistingMW4Mutation, resolve.LoggingMWMutation},
	}
	// mainHealthStore stores the health of the main GraphQL server.
	mainHealthStore = &GraphQLHealthStore{}
	// adminServerVar stores a pointer to the adminServer. It is used for lazy loading schema.
	adminServerVar *adminServer
)

func SchemaValidate(sch string) error {
	schHandler, err := schema.NewHandler(sch, false)
	if err != nil {
		return err
	}

	_, err = schema.FromString(schHandler.GQLSchema())
	return err
}

// GraphQLHealth is used to report the health status of a GraphQL server.
// It is required for kubernetes probing.
type GraphQLHealth struct {
	Healthy   bool
	StatusMsg string
}

// GraphQLHealthStore stores GraphQLHealth in a thread-safe way.
type GraphQLHealthStore struct {
	v atomic.Value
}

func (g *GraphQLHealthStore) GetHealth() GraphQLHealth {
	v := g.v.Load()
	if v == nil {
		return GraphQLHealth{Healthy: false, StatusMsg: "init"}
	}
	return v.(GraphQLHealth)
}

func (g *GraphQLHealthStore) up() {
	g.v.Store(GraphQLHealth{Healthy: true, StatusMsg: "up"})
}

func (g *GraphQLHealthStore) updatingSchema() {
	g.v.Store(GraphQLHealth{Healthy: true, StatusMsg: "updating schema"})
}

type gqlSchema struct {
	ID              string `json:"id,omitempty"`
	Schema          string `json:"schema,omitempty"`
	Version         uint64
	GeneratedSchema string
	loaded          bool // This indicate whether the schema has been loaded into graphql server or not
}

type adminServer struct {
	rf       resolve.ResolverFactory
	resolver *resolve.RequestResolver

	// The mutex that locks schema update operations
	mux sync.RWMutex

	// The GraphQL server that's being admin'd
	gqlServer IServeGraphQL

	schema map[uint64]*gqlSchema

	// When the schema changes, we use these to create a new RequestResolver for
	// the main graphql endpoint (gqlServer) and thus refresh the API.
	fns               *resolve.ResolverFns
	withIntrospection bool
	globalEpoch       map[uint64]*uint64
}

// NewServers initializes the GraphQL servers.  It sets up an empty server for the
// main /graphql endpoint and an admin server.  The result is mainServer, adminServer.
func NewServers(withIntrospection bool, globalEpoch map[uint64]*uint64,
	closer *z.Closer) (IServeGraphQL, IServeGraphQL, *GraphQLHealthStore) {
	gqlSchema, err := schema.FromString("")
	if err != nil {
		x.Panic(err)
	}

	resolvers := resolve.New(gqlSchema, resolverFactoryWithErrorMsg(errNoGraphQLSchema))
	e := globalEpoch[x.GalaxyNamespace]
	mainServer := NewServer()
	mainServer.Set(x.GalaxyNamespace, e, resolvers)

	fns := &resolve.ResolverFns{
		Qrw: resolve.NewQueryRewriter(),
		Arw: resolve.NewAddRewriter,
		Urw: resolve.NewUpdateRewriter,
		Drw: resolve.NewDeleteRewriter(),
		Ex:  resolve.NewDgraphExecutor(),
	}
	adminResolvers := newAdminResolver(mainServer, fns, withIntrospection, globalEpoch, closer)
	e = globalEpoch[x.GalaxyNamespace]
	adminServer := NewServer()
	adminServer.Set(x.GalaxyNamespace, e, adminResolvers)

	return mainServer, adminServer, mainHealthStore
}

// newAdminResolver creates a GraphQL request resolver for the /admin endpoint.
func newAdminResolver(
	defaultGqlServer IServeGraphQL,
	fns *resolve.ResolverFns,
	withIntrospection bool,
	epoch map[uint64]*uint64,
	closer *z.Closer) *resolve.RequestResolver {

	adminSchema, err := schema.FromString(graphqlAdminSchema)
	if err != nil {
		x.Panic(err)
	}

	rf := newAdminResolverFactory()

	server := &adminServer{
		rf:                rf,
		resolver:          resolve.New(adminSchema, rf),
		fns:               fns,
		withIntrospection: withIntrospection,
		globalEpoch:       epoch,
		schema:            make(map[uint64]*gqlSchema),
		gqlServer:         defaultGqlServer,
	}
	adminServerVar = server // store the admin server in package variable

	prefix := x.DataKey(x.GalaxyAttr(worker.GqlSchemaPred), 0)
	// Remove uid from the key, to get the correct prefix
	prefix = prefix[:len(prefix)-8]
	// Listen for graphql schema changes in group 1.
	go worker.SubscribeForUpdates([][]byte{prefix}, x.IgnoreBytes, func(kvs *badgerpb.KVList) {

		kv := x.KvWithMaxVersion(kvs, [][]byte{prefix})
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
		ns, _ := x.ParseNamespaceAttr(pk.Attr)

		newSchema := &gqlSchema{
			ID:      query.UidToHex(pk.Uid),
			Version: kv.GetVersion(),
			Schema:  string(pl.Postings[0].Value),
		}

		server.mux.RLock()
		currentSchema, ok := server.schema[ns]
		if ok && (newSchema.Version <= currentSchema.Version || newSchema.Schema == currentSchema.Schema) {
			glog.Infof("Skipping GraphQL schema update, new badger key version is %d, the old version was %d.",
				newSchema.Version, currentSchema.Version)
			server.mux.RUnlock()
			return
		}
		server.mux.RUnlock()

		var gqlSchema schema.Schema
		// on drop_all, we will receive an empty string as the schema update
		if newSchema.Schema != "" {
			gqlSchema, err = generateGQLSchema(newSchema)
			if err != nil {
				glog.Errorf("Error processing GraphQL schema: %s.  ", err)
				return
			}
		}

		server.mux.Lock()
		defer server.mux.Unlock()

		server.incrementSchemaUpdateCounter(ns)
		// if the schema hasn't been loaded yet, then we don't need to load it here
		currentSchema, ok = server.schema[ns]
		if !(ok && currentSchema.loaded) {
			// this just set schema in admin server, so that next invalid badger subscription update gets rejected upfront
			server.schema[ns] = newSchema
			glog.Infof("Skipping in-memory GraphQL schema update, it will be lazy-loaded later.")
			return
		}

		// update this schema in both admin and graphql server
		newSchema.loaded = true
		server.schema[ns] = newSchema
		server.resetSchema(ns, gqlSchema)

		glog.Infof("Successfully updated GraphQL schema. Serving New GraphQL API.")
	}, 1, closer)

	go server.initServer()

	return server.resolver
}

func newAdminResolverFactory() resolve.ResolverFactory {

	adminMutationResolvers := map[string]resolve.MutationResolverFunc{
		"addNamespace":    resolveAddNamespace,
		"backup":          resolveBackup,
		"config":          resolveUpdateConfig,
		"deleteNamespace": resolveDeleteNamespace,
		"draining":        resolveDraining,
		"export":          resolveExport,
		"login":           resolveLogin,
		"resetPassword":   resolveResetPassword,
		"restore":         resolveRestore,
		"shutdown":        resolveShutdown,
	}

	rf := resolverFactoryWithErrorMsg(errResolverNotFound).
		WithQueryMiddlewareConfig(adminQueryMWConfig).
		WithMutationMiddlewareConfig(adminMutationMWConfig).
		WithQueryResolver("health", func(q schema.Query) resolve.QueryResolver {
			return resolve.QueryResolverFunc(resolveHealth)
		}).
		WithQueryResolver("state", func(q schema.Query) resolve.QueryResolver {
			return resolve.QueryResolverFunc(resolveState)
		}).
		WithQueryResolver("config", func(q schema.Query) resolve.QueryResolver {
			return resolve.QueryResolverFunc(resolveGetConfig)
		}).
		WithQueryResolver("listBackups", func(q schema.Query) resolve.QueryResolver {
			return resolve.QueryResolverFunc(resolveListBackups)
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
	for gqlMut, resolver := range adminMutationResolvers {
		// gotta force go to evaluate the right function at each loop iteration
		// otherwise you get variable capture issues
		func(f resolve.MutationResolver) {
			rf.WithMutationResolver(gqlMut, func(m schema.Mutation) resolve.MutationResolver {
				return f
			})
		}(resolver)
	}

	return rf.WithSchemaIntrospection()
}

func getCurrentGraphQLSchema(namespace uint64) (*gqlSchema, error) {
	uid, graphQLSchema, err := edgraph.GetGQLSchema(namespace)
	if err != nil {
		return nil, err
	}

	return &gqlSchema{ID: uid, Schema: graphQLSchema}, nil
}

func generateGQLSchema(sch *gqlSchema) (schema.Schema, error) {
	schHandler, err := schema.NewHandler(sch.Schema, false)
	if err != nil {
		return nil, err
	}
	sch.GeneratedSchema = schHandler.GQLSchema()
	generatedSchema, err := schema.FromString(sch.GeneratedSchema)
	if err != nil {
		return nil, err
	}
	generatedSchema.SetMeta(schHandler.MetaInfo())

	return generatedSchema, nil
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

		sch, err := getCurrentGraphQLSchema(x.GalaxyNamespace)
		if err != nil {
			glog.Infof("Error reading GraphQL schema: %s.", err)
			continue
		}
		sch.loaded = true
		as.schema[x.GalaxyNamespace] = sch
		// adding the actual resolvers for updateGQLSchema and getGQLSchema only after server has
		// current GraphQL schema, if there was any.
		as.addConnectedAdminResolvers()
		mainHealthStore.up()

		if sch.Schema == "" {
			glog.Infof("No GraphQL schema in Dgraph; serving empty GraphQL API")
			break
		}

		generatedSchema, err := generateGQLSchema(sch)
		if err != nil {
			glog.Infof("Error processing GraphQL schema: %s.", err)
			break
		}
		as.incrementSchemaUpdateCounter(x.GalaxyNamespace)
		as.resetSchema(x.GalaxyNamespace, generatedSchema)

		glog.Infof("Successfully loaded GraphQL schema.  Serving GraphQL API.")

		break
	}
}

// addConnectedAdminResolvers sets up the real resolvers
func (as *adminServer) addConnectedAdminResolvers() {

	qryRw := resolve.NewQueryRewriter()
	dgEx := resolve.NewDgraphExecutor()

	as.rf.WithMutationResolver("updateGQLSchema",
		func(m schema.Mutation) resolve.MutationResolver {
			return &updateSchemaResolver{admin: as}
		}).
		WithQueryResolver("getGQLSchema",
			func(q schema.Query) resolve.QueryResolver {
				return &getSchemaResolver{admin: as}
			}).
		WithQueryResolver("queryGroup",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(qryRw, dgEx)
			}).
		WithQueryResolver("queryUser",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(qryRw, dgEx)
			}).
		WithQueryResolver("getGroup",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(qryRw, dgEx)
			}).
		WithQueryResolver("getCurrentUser",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(&currentUserResolver{baseRewriter: qryRw}, dgEx)
			}).
		WithQueryResolver("getUser",
			func(q schema.Query) resolve.QueryResolver {
				return resolve.NewQueryResolver(qryRw, dgEx)
			}).
		WithMutationResolver("addUser",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewDgraphResolver(resolve.NewAddRewriter(), dgEx)
			}).
		WithMutationResolver("addGroup",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewDgraphResolver(NewAddGroupRewriter(), dgEx)
			}).
		WithMutationResolver("updateUser",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewDgraphResolver(resolve.NewUpdateRewriter(), dgEx)
			}).
		WithMutationResolver("updateGroup",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewDgraphResolver(NewUpdateGroupRewriter(), dgEx)
			}).
		WithMutationResolver("deleteUser",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewDgraphResolver(resolve.NewDeleteRewriter(), dgEx)
			}).
		WithMutationResolver("deleteGroup",
			func(m schema.Mutation) resolve.MutationResolver {
				return resolve.NewDgraphResolver(resolve.NewDeleteRewriter(), dgEx)
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

func (as *adminServer) incrementSchemaUpdateCounter(ns uint64) {
	// Increment the Epoch when you get a new schema. So, that subscription's local epoch
	// will match against global epoch to terminate the current subscriptions.
	e := as.globalEpoch[ns]
	if e == nil {
		e = new(uint64)
		as.globalEpoch[ns] = e
	}
	atomic.AddUint64(e, 1)
}

func (as *adminServer) resetSchema(ns uint64, gqlSchema schema.Schema) {
	// set status as updating schema
	mainHealthStore.updatingSchema()

	var resolverFactory resolve.ResolverFactory
	// If schema is nil (which becomes after drop_all) then do not attach Resolver for
	// introspection operations, and set GQL schema to empty.
	if gqlSchema == nil {
		resolverFactory = resolverFactoryWithErrorMsg(errNoGraphQLSchema)
		gqlSchema, _ = schema.FromString("")
	} else {
		resolverFactory = resolverFactoryWithErrorMsg(errResolverNotFound).
			WithConventionResolvers(gqlSchema, as.fns)
		// If the schema is a Federated Schema then attach "_service" resolver
		if gqlSchema.IsFederated() {
			resolverFactory.WithQueryResolver("_service", func(s schema.Query) resolve.QueryResolver {
				return resolve.QueryResolverFunc(func(ctx context.Context, query schema.Query) *resolve.Resolved {
					as.mux.RLock()
					defer as.mux.RUnlock()
					sch := as.schema[ns].Schema
					handler, err := schema.NewHandler(sch, true)
					if err != nil {
						return resolve.EmptyResult(query, err)
					}
					data := handler.GQLSchemaWithoutApolloExtras()
					return resolve.DataResult(query,
						map[string]interface{}{"_service": map[string]interface{}{"sdl": data}},
						nil)
				})
			})
		}

		if as.withIntrospection {
			resolverFactory.WithSchemaIntrospection()
		}
	}

	resolvers := resolve.New(gqlSchema, resolverFactory)
	as.gqlServer.Set(ns, as.globalEpoch[ns], resolvers)

	// reset status to up, as now we are serving the new schema
	mainHealthStore.up()
}

func (as *adminServer) lazyLoadSchema(namespace uint64) {
	// if the schema is already in memory, no need to fetch it from disk
	as.mux.RLock()
	if currentSchema, ok := as.schema[namespace]; ok && currentSchema.loaded {
		as.mux.RUnlock()
		return
	}
	as.mux.RUnlock()

	// otherwise, fetch the schema from disk
	sch, err := getCurrentGraphQLSchema(namespace)
	if err != nil {
		glog.Infof("Error reading GraphQL schema: %s.", err)
		return
	}

	if sch.Schema == "" {
		glog.Infof("No GraphQL schema in Dgraph; serving empty GraphQL API")
		return
	}

	generatedSchema, err := generateGQLSchema(sch)
	if err != nil {
		glog.Infof("Error processing GraphQL schema: %s.", err)
		return
	}

	as.mux.Lock()
	defer as.mux.Unlock()
	sch.loaded = true
	as.schema[namespace] = sch
	as.resetSchema(namespace, generatedSchema)

	glog.Infof("Successfully lazy-loaded GraphQL schema. Serving GraphQL API.")
}

func LazyLoadSchema(namespace uint64) {
	adminServerVar.lazyLoadSchema(namespace)
}

func response(code, msg string) map[string]interface{} {
	return map[string]interface{}{
		"response": map[string]interface{}{"code": code, "message": msg}}
}

// DestinationFields is used by both export and backup to specify destination
type DestinationFields struct {
	Destination  string
	AccessKey    string
	SecretKey    string
	SessionToken string
	Anonymous    bool
}
