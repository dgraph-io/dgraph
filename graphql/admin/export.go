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

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type exportResolver struct {
	mutation schema.Mutation
}

type exportInput struct {
	Format string
}

func (er *exportResolver) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {
	glog.Info("Got export request through GraphQL admin API")

	er.mutation = m
	input, err := getExportInput(m)
	if err != nil {
		return nil, nil, err
	}

	format := worker.DefaultExportFormat
	if input.Format != "" {
		format = worker.NormalizeExportFormat(input.Format)
		if format == "" {
			return nil, nil, errors.Errorf("invalid export format: %v", input.Format)
		}
	}

	err = worker.ExportOverNetwork(context.Background(), format)
	return nil, nil, errors.Wrapf(err, "export failed")
}

func (er *exportResolver) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	return nil, nil
}

func (er *exportResolver) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string]interface{}, error) {

	return nil, nil, nil
}

func (er *exportResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	buf := writeResponse(er.mutation, "Success", "Export completed.")
	return buf, nil
}

func getExportInput(m schema.Mutation) (*exportInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input exportInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
