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

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

type moveTabletInput struct {
	Namespace uint64
	Tablet    string
	GroupId   uint32
}

func resolveMoveTablet(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	input, err := getMoveTabletInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	// gRPC call returns a nil status if the error is non-nil
	status, err := worker.MoveTabletOverNetwork(ctx, &pb.MoveTabletRequest{
		Namespace: input.Namespace,
		Tablet:    input.Tablet,
		DstGroup:  input.GroupId,
	})
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(m,
		map[string]interface{}{m.Name(): response("Success", status.GetMsg())},
		nil,
	), true
}

func getMoveTabletInput(m schema.Mutation) (*moveTabletInput, error) {
	inputArg, ok := m.ArgValue(schema.InputArgName).(map[string]interface{})
	if !ok {
		return nil, inputArgError(errors.Errorf("can't convert input to map"))
	}

	inputRef := &moveTabletInput{}
	// namespace is an optional parameter
	if _, ok = inputArg["namespace"]; !ok {
		inputRef.Namespace = x.GalaxyNamespace
	} else {
		ns, err := parseAsUint64(inputArg["namespace"])
		if err != nil {
			return nil, inputArgError(schema.GQLWrapf(err,
				"can't convert input.namespace to uint64"))
		}
		inputRef.Namespace = ns
	}

	inputRef.Tablet, ok = inputArg["tablet"].(string)
	if !ok {
		return nil, inputArgError(errors.Errorf("can't convert input.tablet to string"))
	}

	gId, err := parseAsUint32(inputArg["groupId"])
	if err != nil {
		return nil, inputArgError(schema.GQLWrapf(err, "can't convert input.groupId to uint32"))
	}
	inputRef.GroupId = gId

	return inputRef, nil
}
