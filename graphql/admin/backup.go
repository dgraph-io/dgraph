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
	"fmt"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
)

type backupInput struct {
	DestinationFields
	ForceFull bool
}

func resolveBackup(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got backup request")
	if !worker.EnterpriseEnabled() {
		err := fmt.Errorf("you must enable enterprise features first. " +
			"Supply the appropriate license file to Dgraph Zero using the HTTP endpoint.")
		return resolve.EmptyResult(m, err), false
	}

	input, err := getBackupInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}
	if input.Destination == "" {
		err := fmt.Errorf("you must specify a 'destination' value")
		return resolve.EmptyResult(m, err), false
	}

	req := &pb.BackupRequest{
		Destination:  input.Destination,
		AccessKey:    input.AccessKey,
		SecretKey:    input.SecretKey,
		SessionToken: input.SessionToken,
		Anonymous:    input.Anonymous,
		ForceFull:    input.ForceFull,
	}
	taskId, err := worker.Tasks.Enqueue(req)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	msg := fmt.Sprintf("Backup queued with ID %#x", taskId)
	data := response("Success", msg)
	data["taskId"] = fmt.Sprintf("%#x", taskId)
	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): data},
		nil,
	), true
}

func getBackupInput(m schema.Mutation) (*backupInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input backupInput
	err = json.Unmarshal(inputByts, &input)
	return &input, schema.GQLWrapf(err, "couldn't get input argument")
}
