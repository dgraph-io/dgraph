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
	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
	"github.com/golang/glog"
)

type getSchemaResolver struct {
	admin *adminServer

	gqlQuery schema.Query
}

type updateGQLSchemaInput struct {
	Set gqlSchema `json:"set,omitempty"`
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
	schHandler, err := schema.NewHandler(input.Set.Schema, true)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	if _, err = schema.FromString(schHandler.GQLSchema()); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	oldSchemaHash := farm.Fingerprint64([]byte(usr.admin.schema.Schema))
	newSchemaHash := farm.Fingerprint64([]byte(input.Set.Schema))
	updateHistory := oldSchemaHash != newSchemaHash

	resp, err := edgraph.UpdateGQLSchema(ctx, input.Set.Schema, schHandler.DGSchema())
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	if updateHistory {
		if err := edgraph.UpdateSchemaHistory(ctx, input.Set.Schema); err != nil {
			glog.Errorf("error while updating schema history %s", err.Error())
		}
	}

	return &resolve.Resolved{
		Data: map[string]interface{}{
			m.Name(): map[string]interface{}{
				"gqlSchema": map[string]interface{}{
					"id":              query.UidToHex(resp.Uid),
					"schema":          input.Set.Schema,
					"generatedSchema": schHandler.GQLSchema(),
				}}},
		Field: m,
		Err:   nil,
	}, true
}

func (gsr *getSchemaResolver) Rewrite(ctx context.Context,
	gqlQuery schema.Query) (*gql.GraphQuery, error) {
	gsr.gqlQuery = gqlQuery
	return nil, nil
}

func (gsr *getSchemaResolver) Execute(
	ctx context.Context,
	req *dgoapi.Request) (*dgoapi.Response, error) {
	b, err := doQuery(gsr.admin.schema, gsr.gqlQuery)
	return &dgoapi.Response{Json: b}, err
}

func (gsr *getSchemaResolver) CommitOrAbort(ctx context.Context, tc *dgoapi.TxnContext) error {
	return nil
}

func doQuery(gql *gqlSchema, field schema.Field) ([]byte, error) {

	var buf bytes.Buffer
	x.Check2(buf.WriteString(`{ "`))
	x.Check2(buf.WriteString(field.Name()))

	if gql.ID == "" {
		x.Check2(buf.WriteString(`": null }`))
		return buf.Bytes(), nil
	}

	x.Check2(buf.WriteString(`": [{`))

	for i, sel := range field.SelectionSet() {
		var val []byte
		var err error
		switch sel.Name() {
		case "id":
			val, err = json.Marshal(gql.ID)
		case "schema":
			val, err = json.Marshal(gql.Schema)
		case "generatedSchema":
			val, err = json.Marshal(gql.GeneratedSchema)
		}
		x.Check2(val, err)

		if i != 0 {
			x.Check2(buf.WriteString(","))
		}
		x.Check2(buf.WriteString(`"`))
		x.Check2(buf.WriteString(sel.Name()))
		x.Check2(buf.WriteString(`":`))
		x.Check2(buf.Write(val))
	}
	x.Check2(buf.WriteString("}]}"))

	return buf.Bytes(), nil
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
