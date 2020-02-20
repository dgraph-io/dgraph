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
	"bytes"
	"context"

	dgoapi "github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/gql"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/dgraph-io/dgraph/x"
	"github.com/golang/glog"
)

type loginResolver struct {
	mutation   schema.Mutation
	accessJwt  string
	refreshJwt string
}

type loginInput struct {
	UserId       string
	Password     string
	RefreshToken string
}

func (lr *loginResolver) Rewrite(
	m schema.Mutation) (*gql.GraphQuery, []*dgoapi.Mutation, error) {
	glog.Info("Got login request")
	lr.mutation = m
	return nil, nil, nil
}

func (lr *loginResolver) FromMutationResult(
	mutation schema.Mutation,
	assigned map[string]string,
	result map[string]interface{}) (*gql.GraphQuery, error) {

	return nil, nil
}

func (lr *loginResolver) Mutate(
	ctx context.Context,
	query *gql.GraphQuery,
	mutations []*dgoapi.Mutation) (map[string]string, map[string]interface{}, error) {

	input := getLoginInput(lr.mutation)
	resp, err := (&edgraph.Server{}).Login(ctx, &dgoapi.LoginRequest{
		Userid:       input.UserId,
		Password:     input.Password,
		RefreshToken: input.RefreshToken,
	})
	if err != nil {
		return nil, nil, err
	}
	jwt := &dgoapi.Jwt{}
	if err := jwt.Unmarshal(resp.GetJson()); err != nil {
		return nil, nil, err
	}
	lr.accessJwt = jwt.AccessJwt
	lr.refreshJwt = jwt.RefreshJwt
	return nil, nil, nil
}

func (lr *loginResolver) Query(ctx context.Context, query *gql.GraphQuery) ([]byte, error) {
	var buf bytes.Buffer

	x.Check2(buf.WriteString(`{ "`))
	x.Check2(buf.WriteString(lr.mutation.SelectionSet()[0].ResponseName() + `": [{`))

	for i, sel := range lr.mutation.SelectionSet()[0].SelectionSet() {
		var val string
		switch sel.Name() {
		case "accessJWT":
			val = lr.accessJwt
		case "refreshJWT":
			val = lr.refreshJwt
		}
		if i != 0 {
			x.Check2(buf.WriteString(","))
		}
		x.Check2(buf.WriteString(`"`))
		x.Check2(buf.WriteString(sel.ResponseName()))
		x.Check2(buf.WriteString(`":`))
		x.Check2(buf.WriteString(`"` + val + `"`))
	}
	x.Check2(buf.WriteString("}]}"))

	return buf.Bytes(), nil
}

func getLoginInput(m schema.Mutation) *loginInput {
	// We should be able to convert these to string as GraphQL schema validation should ensure this.
	// If the input wasn't specified, then the arg value would be nil and the string value empty.
	userID, _ := m.ArgValue("userId").(string)
	password, _ := m.ArgValue("password").(string)
	refreshToken, _ := m.ArgValue("refreshToken").(string)

	return &loginInput{
		userID,
		password,
		refreshToken,
	}
}
