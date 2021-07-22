/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type updateLambdaInput struct {
	Set worker.LambdaScript `json:"set,omitempty"`
}

func resolveUpdateLambda(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got updateLambdaScript request")

	input, err := getLambdaInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	resp, err := edgraph.UpdateLambdaScript(ctx, input.Set.Script)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(
		m,
		map[string]interface{}{
			m.Name(): map[string]interface{}{
				"lambdaScript": map[string]interface{}{
					"id":     query.UidToHex(resp.Uid),
					"script": input.Set.Script,
				}}},
		nil), true
}

func resolveGetLambda(ctx context.Context, q schema.Query) *resolve.Resolved {
	var data map[string]interface{}

	ns, err := x.ExtractNamespace(ctx)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}

	cs, _ := worker.Lambda().GetCurrent(ns)
	if cs == nil || cs.ID == "" {
		data = map[string]interface{}{q.Name(): nil}
	} else {
		data = map[string]interface{}{
			q.Name(): map[string]interface{}{
				"id":     cs.ID,
				"script": cs.Script,
			}}
	}

	return resolve.DataResult(q, data, nil)
}

func getLambdaInput(m schema.Mutation) (*updateLambdaInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input updateLambdaInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
