/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"

	"github.com/dgraph-io/dgraph/v25/dql"
	"github.com/dgraph-io/dgraph/v25/graphql/resolve"
	"github.com/dgraph-io/dgraph/v25/graphql/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

type currentUserResolver struct {
	baseRewriter resolve.QueryRewriter
}

func extractName(ctx context.Context) (string, error) {
	accessJwt, err := x.ExtractJwt(ctx)
	if err != nil {
		return "", err
	}

	return x.ExtractUserName(accessJwt)
}

func (gsr *currentUserResolver) Rewrite(ctx context.Context,
	gqlQuery schema.Query) ([]*dql.GraphQuery, error) {

	name, err := extractName(ctx)
	if err != nil {
		return nil, err
	}

	gqlQuery.Rename("getUser")
	gqlQuery.SetArgTo("name", name)

	return gsr.baseRewriter.Rewrite(ctx, gqlQuery)
}
