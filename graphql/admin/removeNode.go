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
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
)

type removeNodeInput struct {
	// TODO: once we have types for uint64 and uint32 available in admin schema,
	// update the type of these fields from Int64 with the new type name in admin schema.
	NodeId  uint64
	GroupId uint32
}

func resolveRemoveNode(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	input, err := getRemoveNodeInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	if _, err = worker.RemoveNodeOverNetwork(ctx, &pb.RemoveNodeRequest{NodeId: input.NodeId,
		GroupId: input.GroupId}); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(m,
		map[string]interface{}{m.Name(): response("Success",
			fmt.Sprintf("Removed node with group: %v, idx: %v", input.GroupId, input.NodeId))},
		nil,
	), true
}

func getRemoveNodeInput(m schema.Mutation) (*removeNodeInput, error) {
	inputArg, ok := m.ArgValue(schema.InputArgName).(map[string]interface{})
	if !ok {
		return nil, inputArgError(errors.Errorf("can't convert input to map"))
	}

	inputRef := &removeNodeInput{}
	nodeId, err := parseAsUint64(inputArg["nodeId"])
	if err != nil {
		return nil, inputArgError(schema.GQLWrapf(err, "can't convert input.nodeId to uint64"))
	}
	inputRef.NodeId = nodeId

	gId, err := getInt64FieldAsUint32(inputArg["groupId"])
	if err != nil {
		return nil, inputArgError(schema.GQLWrapf(err, "can't convert input.groupId to uint32"))
	}
	inputRef.GroupId = gId

	return inputRef, nil
}

func parseAsUint64(val interface{}) (uint64, error) {
	gId := uint64(0)
	var err error

	switch v := val.(type) {
	case string:
		gId, err = strconv.ParseUint(v, 10, 64)
	case json.Number:
		gId, err = strconv.ParseUint(v.String(), 10, 64)
	default:
		err = errors.Errorf("got unexpected value type")
	}

	return gId, err
}
