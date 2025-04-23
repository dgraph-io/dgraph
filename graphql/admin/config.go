/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/golang/glog"

	"github.com/hypermodeinc/dgraph/v25/graphql/resolve"
	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
	"github.com/hypermodeinc/dgraph/v25/worker"
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
