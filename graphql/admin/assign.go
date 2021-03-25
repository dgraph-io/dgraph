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

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
)

const (
	UID       = "UID"
	TIMESTAMP = "TIMESTAMP"
)

type assignInput struct {
	What string
	// TODO: once we have type for uint64 available in admin schema,
	// update the type of this field from Int64 with the new type name in admin schema.
	Num uint64
}

func resolveAssign(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	input, err := getAssignInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	var resp *pb.AssignedIds
	num := &pb.Num{Val: input.Num}
	switch input.What {
	case UID:
		resp, err = worker.AssignUidsOverNetwork(ctx, num)
	case TIMESTAMP:
		if num.Val == 0 {
			num.ReadOnly = true
		}
		resp, err = worker.Timestamps(ctx, num)
	}
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	var startId, endId, readOnly interface{}
	// if it was readonly TIMESTAMP request, then let other output fields be `null`,
	// otherwise, let readOnly field remain `null`.
	if input.What == TIMESTAMP && num.Val == 0 {
		// TODO: these conversions should be removed after we have a type which can contain
		// uint64 values in admin schema.
		readOnly = int64(resp.GetReadOnly())
	} else {
		startId = int64(resp.GetStartId())
		endId = int64(resp.GetEndId())
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

	num, err := getInt64FieldAsUint64(inputArg["num"])
	if err != nil {
		return nil, inputArgError(schema.GQLWrapf(err, "can't convert input.num to uint64"))
	}
	inputRef.Num = num

	return inputRef, nil
}
