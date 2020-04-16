/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/golang/glog"
)

type configInput struct {
	LruMB float64
}

func resolveConfig(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got config request through GraphQL admin API")

	input, err := getConfigInput(m)
	if err != nil {
		return emptyResult(m, err), false
	}

	err = worker.UpdateLruMb(input.LruMB)
	if err != nil {
		return emptyResult(m, err), false
	}

	return &resolve.Resolved{
		Data:  map[string]interface{}{m.Name(): response("Success", "Config updated successfully")},
		Field: m,
	}, true
}

func getConfigInput(m schema.Mutation) (*configInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input configInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
