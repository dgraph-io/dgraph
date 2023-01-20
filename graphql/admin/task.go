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
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
)

type taskInput struct {
	Id string
}

func resolveTask(ctx context.Context, q schema.Query) *resolve.Resolved {
	// Get Task ID.
	input, err := getTaskInput(q)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}
	if input.Id == "" {
		return resolve.EmptyResult(q, fmt.Errorf("task ID is missing"))
	}
	taskId, err := strconv.ParseUint(input.Id, 0, 64)
	if err != nil {
		err = errors.Wrapf(err, "invalid task ID: %s", input.Id)
		return resolve.EmptyResult(q, err)
	}

	// Get TaskMeta from network.
	req := &pb.TaskStatusRequest{TaskId: taskId}
	resp, err := worker.TaskStatusOverNetwork(context.Background(), req)
	if err != nil {
		return resolve.EmptyResult(q, err)
	}
	meta := worker.TaskMeta(resp.GetTaskMeta())
	return resolve.DataResult(
		q,
		map[string]interface{}{q.Name(): map[string]interface{}{
			"kind":        meta.Kind().String(),
			"status":      meta.Status().String(),
			"lastUpdated": meta.Timestamp().Format(time.RFC3339),
		}},
		nil,
	)
}

func getTaskInput(q schema.Query) (*taskInput, error) {
	inputArg := q.ArgValue(schema.InputArgName)
	inputBytes, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	var input taskInput
	if err := json.Unmarshal(inputBytes, &input); err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}
	return &input, nil
}
