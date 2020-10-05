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

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
)

type enterpriseLicenseInput struct {
	License string
}

func resolveEnterpriseLicense(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	var input enterpriseLicenseInput
	if err := getTypeInput(m, &input); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	if _, err := worker.ApplyLicenseOverNetwork(ctx, &pb.ApplyLicenseRequest{License: []byte(input.
		License)}); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return &resolve.Resolved{
		Data:  map[string]interface{}{m.Name(): response("Success", "License applied.")},
		Field: m,
	}, true
}
