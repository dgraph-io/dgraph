/*
 * Copyright 2021 Dgraph Labs, Inc. and Contributors
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

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/golang/glog"
)

type taskInput struct {
	Id string
}

func resolveTask(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got task request through GraphQL admin API")

	input, err := getTaskInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}
	if input.Id == "" {
		return resolve.EmptyResult(m, fmt.Errorf("missing task ID")), false
	}
	id, err := strconv.ParseUint(input.Id, 0, 64)
	if err != nil {
		return resolve.EmptyResult(m, fmt.Errorf("invalid task ID")), false
	}

	status := worker.Tasks.GetStatus(id).String()
	msg := fmt.Sprintf("status: %s", status)
	responseData := response("Success", msg)
	responseData["status"] = status
	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): responseData},
		nil,
	), true
}

func getTaskInput(m schema.Mutation) (*taskInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	glog.Infof("inputArg: %+v", inputArg)
	inputBytes, err := json.Marshal(inputArg)
	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}
	glog.Infof("taskInput: %s", string(inputBytes))

	var input taskInput
	if err := json.Unmarshal(inputBytes, &input); err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}
	return &input, nil
}
