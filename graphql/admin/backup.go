/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/golang/glog"

	"github.com/hypermodeinc/dgraph/v25/graphql/resolve"
	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/worker"
)

type backupInput struct {
	DestinationFields
	ForceFull bool
}

func resolveBackup(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got backup request")

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
