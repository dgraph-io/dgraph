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
)

type enterpriseLicenseInput struct {
	License string
}

func resolveEnterpriseLicense(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	input, err := getEnterpriseLicenseInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	if _, err = worker.ApplyLicenseOverNetwork(
		ctx,
		&pb.ApplyLicenseRequest{License: []byte(input.License)},
	); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(m,
		map[string]interface{}{m.Name(): response("Success", "License applied.")},
		nil,
	), true
}

func getEnterpriseLicenseInput(m schema.Mutation) (*enterpriseLicenseInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputBytes, err := json.Marshal(inputArg)
	if err != nil {
		return nil, inputArgError(err)
	}

	var input enterpriseLicenseInput
	err = schema.Unmarshal(inputBytes, &input)
	return &input, inputArgError(err)
}
