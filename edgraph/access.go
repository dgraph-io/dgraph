// +build oss

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

package edgraph

import (
	"context"

	"github.com/dgraph-io/badger/v2/y"
	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

// Login handles login requests from clients. This version rejects all requests
// since ACL is only supported in the enterprise version.
func (s *Server) Login(ctx context.Context,
	request *api.LoginRequest) (*api.Response, error) {
	if err := x.HealthCheck(); err != nil {
		return nil, err
	}

	glog.Warningf("Login failed: %s", x.ErrNotSupported)
	return &api.Response{}, x.ErrNotSupported
}

// ResetAcl is an empty method since ACL is only supported in the enterprise version.
func ResetAcl() {
	// do nothing
}

// ResetAcls is an empty method since ACL is only supported in the enterprise version.
func RefreshAcls(closer *y.Closer) {
	// do nothing
	<-closer.HasBeenClosed()
	closer.Done()
}

func authorizeAlter(ctx context.Context, op *api.Operation) error {
	return nil
}

func authorizeMutation(ctx context.Context, gmu *gql.Mutation) error {
	return nil
}

func authorizeQuery(ctx context.Context, parsedReq *gql.Result, graphql bool) error {
	// always allow access
	return nil
}

func authorizeGuardians(ctx context.Context) error {
	// always allow access
	return nil
}
