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
	"strconv"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/worker"
)

type configInput struct {
	CacheMb *float64
	// LogDQLRequest is used to update WorkerOptions.LogDQLRequest. true value of LogDQLRequest enables
	// logging of all requests coming to alphas. LogDQLRequest type has been kept as *bool instead of
	// bool to avoid updating WorkerOptions.LogDQLRequest when it has default value of false.
	LogDQLRequest *bool
}

func resolveUpdateConfig(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got config update through GraphQL admin API")

	input, err := getConfigInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	// update cacheMB only when it is specified by user
	if input.CacheMb != nil {
		if err = worker.UpdateCacheMb(int64(*input.CacheMb)); err != nil {
			return resolve.EmptyResult(m, err), false
		}
	}

	// input.LogDQLRequest will be nil, when it is not specified explicitly in config request.
	if input.LogDQLRequest != nil {
		worker.UpdateLogDQLRequest(*input.LogDQLRequest)
	}

	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): response("Success", "Config updated successfully")},
		nil,
	), true
}

func resolveGetConfig(ctx context.Context, q schema.Query) *resolve.Resolved {
	glog.Info("Got config query through GraphQL admin API")

	return resolve.DataResult(
		q,
		map[string]interface{}{q.Name(): map[string]interface{}{
			"cacheMb": json.Number(strconv.FormatInt(worker.Config.CacheMb, 10)),
		}},
		nil,
	)

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
