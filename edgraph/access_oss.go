// +build oss

package edgraph

import (
	"context"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/oss"
	"github.com/golang/glog"
)

func (s *Server) Login(ctx context.Context,
	request *api.LogInRequest) (*api.LogInResponse, error) {
	glog.Infof("Login failed: %s", oss.ErrNotSupported)
	return &api.LogInResponse{}, nil
}
