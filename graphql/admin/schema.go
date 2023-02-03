/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type getSchemaResolver struct {
	admin *adminServer
}

type updateGQLSchemaInput struct {
	Set worker.GqlSchema `json:"set,omitempty"`
}

type updateSchemaResolver struct {
	admin *adminServer
}

func (usr *updateSchemaResolver) Resolve(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got updateGQLSchema request")

	input, err := getSchemaInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	// We just need to validate the schema. Schema is later set in `resetSchema()` when the schema
	// is returned from badger.
	schHandler, err := schema.NewHandler(input.Set.Schema, false)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	// we don't need the correct namespace for validation, so passing the Galaxy namespace
	if _, err = schema.FromString(schHandler.GQLSchema(), x.GalaxyNamespace); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	resp, err := edgraph.UpdateGQLSchema(ctx, input.Set.Schema, schHandler.DGSchema())
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(
		m,
		map[string]interface{}{
			m.Name(): map[string]interface{}{
				"gqlSchema": map[string]interface{}{
					"id":              query.UidToHex(resp.Uid),
					"schema":          input.Set.Schema,
					"generatedSchema": schHandler.GQLSchema(),
				}}},
		nil), true
}

func (gsr *getSchemaResolver) Resolve(ctx context.Context, q schema.Query) *resolve.Resolved {
	var data map[string]interface{}

	gsr.admin.mux.RLock()
	defer gsr.admin.mux.RUnlock()

	ns, err := x.ExtractNamespace(ctx)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}

	cs, _ := gsr.admin.gqlSchemas.GetCurrent(ns)
	if cs == nil || cs.ID == "" {
		data = map[string]interface{}{q.Name(): nil}
	} else {
		data = map[string]interface{}{
			q.Name(): map[string]interface{}{
				"id":              cs.ID,
				"schema":          cs.Schema,
				"generatedSchema": cs.GeneratedSchema,
			}}
	}

	return resolve.DataResult(q, data, nil)
}

func getSchemaInput(m schema.Mutation) (*updateGQLSchemaInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input updateGQLSchemaInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
