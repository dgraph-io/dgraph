/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
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

package zero

import (
	"context"
	"math"
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

func TestRemoveNode(t *testing.T) {
	server := &Server{
		state: &pb.MembershipState{
			Groups: map[uint32]*pb.Group{1: {Members: map[uint64]*pb.Member{}}},
		},
	}
	_, err := server.RemoveNode(context.TODO(), &pb.RemoveNodeRequest{NodeId: 3, GroupId: 1})
	require.Error(t, err)
	_, err = server.RemoveNode(context.TODO(), &pb.RemoveNodeRequest{NodeId: 1, GroupId: 2})
	require.Error(t, err)
}

func TestIdLeaseOverflow(t *testing.T) {
	require.NoError(t, testutil.AssignUids(100))
	err := testutil.AssignUids(math.MaxUint64 - 10)
	require.Error(t, err)
	require.Contains(t, err.Error(), "limit has reached")
}
