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
	"github.com/pkg/errors"
)

type exportInput struct {
	Format string
}

func resolveExport(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {

	glog.Info("Got export request through GraphQL admin API")

	input, err := getExportInput(m)
	if err != nil {
		return emptyResult(m, err), false
	}

	format := worker.DefaultExportFormat
	if input.Format != "" {
		format = worker.NormalizeExportFormat(input.Format)
		if format == "" {
			return emptyResult(m, errors.Errorf("invalid export format: %v", input.Format)), false
		}
	}

	err = worker.ExportOverNetwork(context.Background(), format)
	if err != nil {
		return emptyResult(m, err), false
	}

	return &resolve.Resolved{
		Data:  map[string]interface{}{m.Name(): response("Success", "Export completed.")},
		Field: m,
	}, true
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
