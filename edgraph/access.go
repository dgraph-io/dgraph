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

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

func (s *Server) Login(ctx context.Context,
	request *api.LoginRequest) (*api.Response, error) {

	glog.Warningf("Login failed: %s", x.ErrNotSupported)
	return &api.Response{}, x.ErrNotSupported
}

func InitAclCache() {
	// do nothing
}

func RetrieveAclsPeriodically(closeCh <-chan struct{}) {
	// do nothing
}

func (s *Server) ParseAndAuthorizeAlter(ctx context.Context, op *api.Operation) (bool,
	string, []*pb.SchemaUpdate, error) {
	updates, err := schema.Parse(op.Schema)
	if err != nil {
		return false, "", nil, err
	}
	return op.DropAll, op.DropAttr, updates, nil
}

func (s *Server) AuthorizeMutation(ctx context.Context, gmu *gql.Mutation) error {
	return nil
}

func (s *Server) AuthorizeQuery(ctx context.Context, parsedReq gql.Result) error {
	// always allow access
	return nil
}
