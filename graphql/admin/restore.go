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

type restoreInput struct {
	Location     string
	BackupId     string
	KeyFile      string
	AccessKey    string
	SecretKey    string
	SessionToken string
	Anonymous    bool
}

func resolveRestore(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {

	input, err := getRestoreInput(m)
	if err != nil {
		return emptyResult(m, err), false
	}

	req := pb.RestoreRequest{
		Location:     input.Location,
		BackupId:     input.BackupId,
		KeyFile:      input.KeyFile,
		AccessKey:    input.AccessKey,
		SecretKey:    input.SecretKey,
		SessionToken: input.SessionToken,
		Anonymous:    input.Anonymous,
	}
	err = worker.ProcessRestoreRequest(context.Background(), &req)
	if err != nil {
		return emptyResult(m, err), false
	}

	return &resolve.Resolved{
		Data:  map[string]interface{}{m.Name(): response("Success", "Restore completed.")},
		Field: m,
	}, true
}

func getRestoreInput(m schema.Mutation) (*restoreInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input restoreInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
