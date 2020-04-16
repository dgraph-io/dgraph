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
	"sync/atomic"

	dgoapi "github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type configResolver struct {
	mutation schema.Mutation
}

type configInput struct {
	LruMB float64
	// Positive value of logRequest will make alpha print all the request it gets.
	// To stop we should set it to a negative value, zero value is being ignored because
	// that might be the case when this parameter isn't specified in query.
	// It will be used like a boolean. We are keeping int32 because we want to
	// modify it using atomics(atomics doesn't have support for bool).
	LogRequest *bool
}

func (cr *configResolver) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {
	glog.Info("Got config request through GraphQL admin API")

	cr.mutation = m
	input, err := getConfigInput(m)
	if err != nil {
		return nil, nil, err
	}

	if input.LruMB > 0 {
		if err = worker.UpdateLruMb(input.LruMB); err != nil {
			return nil, nil, err
		}
	}
	if input.LogRequest != nil {
		if *input.LogRequest {
			atomic.StoreInt32(&x.WorkerConfig.LogRequest, 1)
		} else {
			atomic.StoreInt32(&x.WorkerConfig.LogRequest, -1)
		}
	}

	return nil, nil, nil
}

func (cr *configResolver) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	return nil, nil
}

func (cr *configResolver) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string]interface{}, error) {

	return nil, nil, nil
}

func (cr *configResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	buf := writeResponse(cr.mutation, "Success", "Config updated successfully")
	return buf, nil
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
