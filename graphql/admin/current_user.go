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
)

type currentUserResolver struct {
	baseRewriter resolve.QueryRewriter
}

func extractName(ctx context.Context) (string, error) {
	accessJwt, err := x.ExtractJwt(ctx)
	if err != nil {
		return "", err
	}

	// Code copied from access_ee.go. Couldn't put the code in x, because of dependency on
	// worker. (worker.Config.HmacSecret)
	token, err := jwt.Parse(accessJwt[0], func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Errorf("unexpected signing method: %v",
				token.Header["alg"])
		}
		return []byte(worker.Config.HmacSecret), nil
	})

	if err != nil {
		return "", errors.Wrapf(err, "unable to parse jwt token")
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
	gqlQuery schema.Query) ([]*gql.GraphQuery, error) {

	name, err := extractName(ctx)
	if err != nil {
		return nil, err
	}

	gqlQuery.Rename("getUser")
	gqlQuery.SetArgTo("name", name)

	return gsr.baseRewriter.Rewrite(ctx, gqlQuery)
}
