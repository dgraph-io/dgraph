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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

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

	// GraphQL schema for /admin endpoint.
	//
	// Eventually we should generate this from just the types definition.
	// But for now, that would add too much into the schema, so this is
	// hand crafted to be one of our schemas so we can pass it into the
	// pipeline.
	graphqlAdminSchema = `
	type GQLSchema @dgraph(type: "dgraph.graphql") {
		id: ID!
		schema: String!  @dgraph(type: "dgraph.graphql.schema")
		generatedSchema: String!
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

	input BackupInput {
		destination: String!
		accessKey: String
		secretKey: String
		sessionToken: String
		anonymous: Boolean
		forceFull: Boolean
	}

	type Response {
		code: String
		message: String
	}

	type ExportPayload {
		response: Response
	}

	input DrainingInput {
		enable: Boolean
	}

	type DrainingPayload {
		response: Response
	}

	type ShutdownPayload {
		response: Response
	}

	input ConfigInput {
		lruMb: Float
	}

	type ConfigPayload {
		response: Response
	}

	type BackupPayload {
		response: Response
	}

	type Query {
		getGQLSchema: GQLSchema
		health: Health
	}

	type Mutation {
		updateGQLSchema(input: UpdateGQLSchemaInput!) : UpdateGQLSchemaPayload
		export(input: ExportInput!): ExportPayload
		draining(input: DrainingInput!): DrainingPayload
		shutdown: ShutdownPayload
		config(input: ConfigInput!): ConfigPayload
		backup(input: BackupInput!) : BackupPayload
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
	status   healthStatus

	// The mutex that locks schema update operations
	mux sync.Mutex

	// The GraphQL server that's being admin'd
	gqlServer web.IServeGraphQL

	schema gqlSchema

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

		newSchema := gqlSchema{
			ID:     fmt.Sprintf("%#x", pk.Uid),
			Schema: string(pl.Postings[0].Value),
		}

		schHandler, err := schema.NewHandler(newSchema.Schema)
		if err != nil {
			glog.Errorf("Error processing GraphQL schema: %s.  ", err)
			return
		}

		newSchema.GeneratedSchema = schHandler.GQLSchema()
		gqlSchema, err := schema.FromString(newSchema.GeneratedSchema)
		if err != nil {
			glog.Errorf("Error processing GraphQL schema: %s.  ", err)
			return
		}

		glog.Infof("Successfully updated GraphQL schema.")

		server.mux.Lock()
		defer server.mux.Unlock()

		server.schema = newSchema
		server.status = healthy
		server.resetSchema(gqlSchema)
	}, 1, closer)

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
		WithMutationResolver("updateGQLSchema", func(m schema.Mutation) resolve.MutationResolver {
			return resolve.MutationResolverFunc(
				func(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
					return &resolve.Resolved{Err: errors.Errorf(errMsgServerNotReady)}, false
				})
		}).
		WithQueryResolver("getGQLSchema", func(q schema.Query) resolve.QueryResolver {
			return resolve.QueryResolverFunc(
				func(ctx context.Context, query schema.Query) *resolve.Resolved {
					return &resolve.Resolved{Err: errors.Errorf(errMsgServerNotReady)}
				})
		}).
		WithMutationResolver("export", func(m schema.Mutation) resolve.MutationResolver {
			export := &exportResolver{}

			// export implements the mutation rewriter, executor and query executor hence its passed
			// thrice here.
			return resolve.NewMutationResolver(
				export,
				export,
				export,
				resolve.StdMutationCompletion(m.ResponseName()))
		}).
		WithMutationResolver("draining", func(m schema.Mutation) resolve.MutationResolver {
			draining := &drainingResolver{}

			// draining implements the mutation rewriter, executor and query executor hence its
			// passed thrice here.
			return resolve.NewMutationResolver(
				draining,
				draining,
				draining,
				resolve.StdMutationCompletion(m.ResponseName()))
		}).
		WithMutationResolver("shutdown", func(m schema.Mutation) resolve.MutationResolver {
			shutdown := &shutdownResolver{}

			// shutdown implements the mutation rewriter, executor and query executor hence its
			// passed thrice here.
			return resolve.NewMutationResolver(
				shutdown,
				shutdown,
				shutdown,
				resolve.StdMutationCompletion(m.ResponseName()))
		}).
		WithMutationResolver("config", func(m schema.Mutation) resolve.MutationResolver {
			config := &configResolver{}

			// config implements the mutation rewriter, executor and query executor hence its
			// passed thrice here.
			return resolve.NewMutationResolver(
				config,
				config,
				config,
				resolve.StdMutationCompletion(m.ResponseName()))
		}).
		WithMutationResolver("backup", func(m schema.Mutation) resolve.MutationResolver {
			backup := &backupResolver{}

			// backup implements the mutation rewriter, executor and query executor hence its passed
			// thrice here.
			return resolve.NewMutationResolver(
				backup,
				backup,
				backup,
				resolve.StdMutationCompletion(m.ResponseName()))
		}).
		WithSchemaIntrospection()

	return rf
}

func (as *adminServer) initServer() {
	var waitFor time.Duration
	for {
		<-time.After(waitFor)
		waitFor = 10 * time.Second

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
			glog.Infof("Error reading GraphQL schema: %s.", err)
			break
		} else if sch == nil {
			glog.Infof("No GraphQL schema in Dgraph; serving empty GraphQL API")
			break
		}

		schHandler, err := schema.NewHandler(sch.Schema)
		if err != nil {
			glog.Infof("Error processing GraphQL schema: %s.", err)
			break
		}

		sch.GeneratedSchema = schHandler.GQLSchema()
		generatedSchema, err := schema.FromString(sch.GeneratedSchema)
		if err != nil {
			glog.Infof("Error processing GraphQL schema: %s.", err)
			break
		}

		glog.Infof("Successfully loaded GraphQL schema.  Serving GraphQL API.")

		as.schema = *sch
		as.status = healthy
		as.resetSchema(generatedSchema)

		break
	}
}

// addConnectedAdminResolvers sets up the real resolvers
func (as *adminServer) addConnectedAdminResolvers() {

	qryRw := resolve.NewQueryRewriter()
	addRw := resolve.NewAddRewriter()
	updRw := resolve.NewUpdateRewriter()
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
		WithMutationResolver("updateGQLSchema",
			func(m schema.Mutation) resolve.MutationResolver {
				updResolver := &updateSchemaResolver{
					admin:                as,
					baseAddRewriter:      addRw,
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
					admin:        as,
					baseRewriter: qryRw,
					baseExecutor: resolve.AdminQueryExecutor(),
				}

				return resolve.NewQueryResolver(
					getResolver,
					getResolver,
					resolve.StdQueryCompletion())
			})
}

func getCurrentGraphQLSchema(r *resolve.RequestResolver) (*gqlSchema, error) {
	req := &schema.Request{
		Query: `query { getGQLSchema { id schema } }`}
	resp := r.Resolve(context.Background(), req)
	if len(resp.Errors) > 0 || resp.Data.Len() == 0 {
		return nil, resp.Errors
	}

	var result struct {
		GetGQLSchema *gqlSchema
	}

	err := json.Unmarshal(resp.Data.Bytes(), &result)

	return result.GetGQLSchema, err
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

func writeResponse(m schema.Mutation, code, message string) []byte {
	var buf bytes.Buffer

	x.Check2(buf.WriteString(`{ "`))
	x.Check2(buf.WriteString(m.SelectionSet()[0].ResponseName() + `": [{`))

	for i, sel := range m.SelectionSet()[0].SelectionSet() {
		var val string
		switch sel.Name() {
		case "code":
			val = code
		case "message":
			val = message
		}
		if i != 0 {
			x.Check2(buf.WriteString(","))
		}
		x.Check2(buf.WriteString(`"`))
		x.Check2(buf.WriteString(sel.ResponseName()))
		x.Check2(buf.WriteString(`":`))
		x.Check2(buf.WriteString(`"` + val + `"`))
	}
	x.Check2(buf.WriteString("}]}"))

	return buf.Bytes()
}
