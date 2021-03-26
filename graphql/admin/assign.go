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
	"strconv"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
)

const (
	uid         = "UID"
	timestamp   = "TIMESTAMP"
	namespaceId = "NAMESPACE_ID"
)

type assignInput struct {
	What string
	Num  uint64
}

func resolveAssign(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	input, err := getAssignInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	var resp *pb.AssignedIds
	num := &pb.Num{Val: input.Num}
	switch input.What {
	case uid:
		resp, err = worker.AssignUidsOverNetwork(ctx, num)
	case timestamp:
		if num.Val == 0 {
			num.ReadOnly = true
		}
		resp, err = worker.Timestamps(ctx, num)
	case namespaceId:
		resp, err = worker.AssignNsIdsOverNetwork(ctx, num)
	}
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	var startId, endId, readOnly interface{}
	// if it was readonly TIMESTAMP request, then let other output fields be `null`,
	// otherwise, let readOnly field remain `null`.
	if input.What == timestamp && num.Val == 0 {
		readOnly = json.Number(strconv.FormatUint(resp.GetReadOnly(), 10))
	} else {
		startId = json.Number(strconv.FormatUint(resp.GetStartId(), 10))
		endId = json.Number(strconv.FormatUint(resp.GetEndId(), 10))
	}

	return resolve.DataResult(m,
		map[string]interface{}{m.Name(): map[string]interface{}{
			"response": map[string]interface{}{
				"startId":  startId,
				"endId":    endId,
				"readOnly": readOnly,
			},
		}},
		nil,
	), true
}

func getAssignInput(m schema.Mutation) (*assignInput, error) {
	inputArg, ok := m.ArgValue(schema.InputArgName).(map[string]interface{})
	if !ok {
		return nil, inputArgError(errors.Errorf("can't convert input to map"))
	}

	inputRef := &assignInput{}
	inputRef.What, ok = inputArg["what"].(string)
	if !ok {
		return nil, inputArgError(errors.Errorf("can't convert input.what to string"))
	}

	num, err := parseAsUint64(inputArg["num"])
	if err != nil {
		return nil, inputArgError(schema.GQLWrapf(err, "can't convert input.num to uint64"))
	}
	inputRef.Num = num

	return inputRef, nil
}
