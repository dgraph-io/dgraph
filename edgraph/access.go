//go:build oss
// +build oss

/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package edgraph

import (
	"context"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgo/v240/protos/api"
	"github.com/dgraph-io/ristretto/v2/z"
	"github.com/hypermodeinc/dgraph/v24/dql"
	"github.com/hypermodeinc/dgraph/v24/query"
	"github.com/hypermodeinc/dgraph/v24/x"
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
func InitializeAcl(closer *z.Closer) {
	// do nothing
}

func upsertGuardianAndGroot(closer *z.Closer, ns uint64) {
	// do nothing
}

// SubscribeForAclUpdates is an empty method since ACL is only supported in the enterprise version.
func SubscribeForAclUpdates(closer *z.Closer) {
	// do nothing
	<-closer.HasBeenClosed()
	closer.Done()
}

// RefreshACLs is an empty method since ACL is only supported in the enterprise version.
func RefreshACLs(ctx context.Context) {
	return
}

func authorizeAlter(ctx context.Context, op *api.Operation) error {
	return nil
}

func authorizeMutation(ctx context.Context, gmu *dql.Mutation) error {
	return nil
}

func authorizeQuery(ctx context.Context, parsedReq *dql.Result, graphql bool) error {
	// always allow access
	return nil
}

func authorizeSchemaQuery(ctx context.Context, er *query.ExecutionResult) error {
	// always allow schema access
	return nil
}

func AuthorizeGuardians(ctx context.Context) error {
	// always allow access
	return nil
}

func AuthGuardianOfTheGalaxy(ctx context.Context) error {
	// always allow access
	return nil
}

func validateToken(jwtStr string) ([]string, error) {
	return nil, nil
}

func upsertGuardian(ctx context.Context) error {
	return nil
}

func upsertGroot(ctx context.Context) error {
	return nil
}
