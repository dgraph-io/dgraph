/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package zero

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/stretchr/testify/require"
)

func TestRemoveNode(t *testing.T) {
	server := &Server{
		state: &pb.MembershipState{
			Groups: map[uint32]*pb.Group{1: {Members: map[uint64]*pb.Member{}}},
		},
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, "debug", "true")
	err := server.removeNode(nil, 3, 1)
	require.Error(t, err)
	err = server.removeNode(nil, 1, 2)
	require.Error(t, err)
}
