package zero

import (
	"context"
	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRemoveNode(t *testing.T) {
	server := &Server{
		state: &intern.MembershipState{
			Groups: map[uint32]*intern.Group{1: {Members: map[uint64]*intern.Member{}}},
		},
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, "debug", "true")
	err := server.removeNode(nil, 3, 1)
	require.Error(t, err)
	err = server.removeNode(nil, 1, 2)
	require.Error(t, err)
}
