/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package admin

import (
	"context"

	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
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
	gqlQuery schema.Query) ([]*gql.GraphQuery, error) {

	name, err := extractName(ctx)
	if err != nil {
		return nil, err
	}

	gqlQuery.Rename("getUser")
	gqlQuery.SetArgTo("name", name)

	return gsr.baseRewriter.Rewrite(ctx, gqlQuery)
}
