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

package zero

import (
	"context"
	"io/ioutil"
	"math"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/raftwal"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/dgraph-io/ristretto/z"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
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

func TestIdBump(t *testing.T) {
	dialOpts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithInsecure(),
	}
	ctx := context.Background()
	con, err := grpc.DialContext(ctx, testutil.SockAddrZero, dialOpts...)
	require.NoError(t, err)

	zc := pb.NewZeroClient(con)

	res, err := zc.AssignIds(ctx, &pb.Num{Val: 10, Type: pb.Num_UID})
	require.NoError(t, err)
	require.Equal(t, uint64(10), res.GetEndId()-res.GetStartId()+1)

	// Next assignemnt's startId should be greater than 10.
	res, err = zc.AssignIds(ctx, &pb.Num{Val: 50, Type: pb.Num_UID})
	require.NoError(t, err)
	require.Greater(t, res.GetStartId(), uint64(10))
	require.Equal(t, uint64(50), res.GetEndId()-res.GetStartId()+1)

	bumpTo := res.GetEndId() + 100000

	// Bump the lease to (last result + 100000).
	_, err = zc.AssignIds(ctx, &pb.Num{Val: bumpTo, Type: pb.Num_UID, Bump: true})
	require.NoError(t, err)

	// Next assignemnt's startId should be greater than bumpTo.
	res, err = zc.AssignIds(ctx, &pb.Num{Val: 10, Type: pb.Num_UID})
	require.NoError(t, err)
	require.Greater(t, res.GetStartId(), bumpTo)
	require.Equal(t, uint64(10), res.GetEndId()-res.GetStartId()+1)

	// If bump request is less than maxLease, then it should result in no-op.
	_, err = zc.AssignIds(ctx, &pb.Num{Val: 10, Type: pb.Num_UID, Bump: true})
	require.Contains(t, err.Error(), "Nothing to be leased")
}

func TestProposalKey(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_pk")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	store := raftwal.Init(dir)

	id := uint64(2)
	rc := &pb.RaftContext{Id: id}
	n := conn.NewNode(rc, store, nil)
	node := &node{Node: n, ctx: context.Background(), closer: z.NewCloser(1)}
	node.initProposalKey(node.Id)

	pkey := proposalKey
	nodeIdFromKey := proposalKey >> 48
	require.Equal(t, id, nodeIdFromKey, "id extracted from proposal key is not equal to initial value")

	node.uniqueKey()
	require.Equal(t, pkey+1, proposalKey, "proposal key should increment by 1 at each call of unique key")

	uniqueKeys := make(map[uint64]struct{})
	for i := 0; i < 10; i++ {
		node.uniqueKey()
		uniqueKeys[proposalKey] = struct{}{}
	}
	require.Equal(t, len(uniqueKeys), 10, "each iteration should create unique key")
}
