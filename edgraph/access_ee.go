// +build !oss

/*
 * Copyright 2018 Dgraph Labs, Inc. All rights reserved.
 *
 * Licensed under the Dgraph Community License (the "License"); you
 * may not use this file except in compliance with the License. You
 * may obtain a copy of the License at
 *
 *     https://github.com/dgraph-io/dgraph/blob/master/licenses/DCL.txt
 */

package edgraph

import (
	"context"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func (s *Server) Login(ctx context.Context,
	request *api.LoginRequest) (*api.Response, error) {

	glog.Warningf("Login failed in version 1.0: %s", x.ErrNotSupported)
	return &api.Response{}, x.ErrNotSupported
}
