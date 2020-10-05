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

	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/worker"
)

type moveTabletInput struct {
	Tablet string
	// TODO: once we have type for uint32 available in admin schema,
	// update the type of this field from Int64 with the new type name in admin schema.
	GroupId uint32
}

func resolveMoveTablet(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	var input moveTabletInput
	err := getTypeInput(m, &input)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	var status *pb.Status
	if status, err = worker.MoveTabletOverNetwork(ctx,
		&pb.MoveTabletRequest{Tablet: input.Tablet, DstGroup: input.GroupId}); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return &resolve.Resolved{
		Data:  map[string]interface{}{m.Name(): response("Success", status.GetMsg())},
		Field: m,
	}, true
}
