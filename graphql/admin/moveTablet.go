/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"

	"github.com/pkg/errors"

	"github.com/hypermodeinc/dgraph/v25/graphql/resolve"
	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
	"github.com/hypermodeinc/dgraph/v25/protos/pb"
	"github.com/hypermodeinc/dgraph/v25/worker"
	"github.com/hypermodeinc/dgraph/v25/x"
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
		inputRef.Namespace = x.RootNamespace
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
