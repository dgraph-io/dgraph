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
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

type currentUserResolver struct {
	gqlQuery     schema.Query
	baseRewriter resolve.QueryRewriter
}

func extractName(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", x.ErrNoJwt
	}
	accessJwt := md.Get("accessJwt")
	if len(accessJwt) == 0 {
		return "", x.ErrNoJwt
	}

	token, err := jwt.Parse(accessJwt[0], func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Errorf("unexpected signing method: %v",
				token.Header["alg"])
		}
		return worker.Config.HmacSecret, nil
	})

	if err != nil {
		return "", errors.Errorf("unable to parse jwt token:%v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", errors.Errorf("claims in jwt token is not map claims")
	}

	userId, ok := claims["userid"].(string)
	if !ok {
		return "", errors.Errorf("userid in claims is not a string:%v", userId)
	}

	return userId, nil
}

func (gsr *currentUserResolver) Rewrite(ctx context.Context,
	gqlQuery schema.Query) (*gql.GraphQuery, error) {

	name, err := extractName(ctx)
	if err != nil {
		return nil, err
	}

	gsr.gqlQuery = gqlQuery
	gqlQuery.Rename("getUser")
	gqlQuery.SetArgTo("name", name)

	return gsr.baseRewriter.Rewrite(ctx, gqlQuery)
}
