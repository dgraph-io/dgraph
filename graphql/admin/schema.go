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
	"time"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/api"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

// A addSchemaResolver serves as the mutation rewriter and executor in handling
// the addSchema mutation.
type addSchemaResolver struct {
	admin *adminServer

	// schema that is generated from the mutation input
	newGQLSchema    schema.Schema
	newDgraphSchema string

	// The underlying executor and rewriter that persist the schema into Dgraph as
	// GraphQL metadata
	baseMutationRewriter resolve.MutationRewriter
	baseMutationExecutor resolve.MutationExecutor
}

func (asr *addSchemaResolver) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {

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

	asr.admin.mux.Lock()
	defer asr.admin.mux.Unlock()

	assigned, mutated, err := asr.baseMutationExecutor.Mutate(ctx, query, mutations)
	if err != nil {
		return nil, nil, err
	}

	glog.Infof("[%s] Altering Dgraph schema.", api.RequestID(ctx))
	if glog.V(3) {
		glog.Infof("[%s] New schema Dgraph:\n\n%s\n", api.RequestID(ctx), asr.newDgraphSchema)
	}

	_, err = (&edgraph.Server{}).Alter(ctx, &dgoapi.Operation{Schema: asr.newDgraphSchema})
	if err != nil {
		return nil, nil, schema.GQLWrapf(err,
			"succeeded in saving GraphQL schema but failed to alter Dgraph schema "+
				"(you should retry)")
	}

	asr.admin.resetSchema(asr.newGQLSchema)

	glog.Infof("[%s] Successfully loaded new GraphQL schema.  Serving New GraphQL API.",
		api.RequestID(ctx))

	return assigned, mutated, nil
}

func getSchemaInput(m schema.Mutation) (string, error) {
	input, ok := m.ArgValue(schema.InputArgName).(map[string]interface{})
	if !ok {
		return "", errors.Errorf("couldn't get argument %s", schema.InputArgName)
	}

	return input["schema"].(string), nil
}
