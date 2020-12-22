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
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

type exportInput struct {
	Format string
	DestinationFields
}

func resolveExport(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {

	glog.Info("Got export request through GraphQL admin API")

	input, err := getExportInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	format := worker.DefaultExportFormat
	if input.Format != "" {
		format = worker.NormalizeExportFormat(input.Format)
		if format == "" {
			return resolve.EmptyResult(m, errors.Errorf("invalid export format: %v", input.Format)), false
		}
	}

	files, err := worker.ExportOverNetwork(context.Background(), &pb.ExportRequest{
		Format:       format,
		Destination:  input.Destination,
		AccessKey:    input.AccessKey,
		SecretKey:    input.SecretKey,
		SessionToken: input.SessionToken,
		Anonymous:    input.Anonymous,
	})
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	responseData := response("Success", "Export completed.")
	responseData["exportedFiles"] = toGraphQLArray(files)

	return &resolve.Resolved{
		Data:  map[string]interface{}{m.Name(): responseData},
		Field: m,
	}, true
}

func toGraphQLArray(s []string) []interface{} {
	outputFiles := make([]interface{}, 0, len(s))
	for _, f := range s {
		outputFiles = append(outputFiles, f)
	}
	return outputFiles
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
