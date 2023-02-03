/*
 * Copyright 2022 Dgraph Labs, Inc. and Contributors
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
	"encoding/json"

	"github.com/golang/glog"

	dgoapi "github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

type loginInput struct {
	UserId       string
	Password     string
	Namespace    uint64
	RefreshToken string
}

func resolveLogin(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got login request")

	input := getLoginInput(m)
	resp, err := (&edgraph.Server{}).Login(ctx, &dgoapi.LoginRequest{
		Userid:       input.UserId,
		Password:     input.Password,
		Namespace:    input.Namespace,
		RefreshToken: input.RefreshToken,
	})
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	jwt := &dgoapi.Jwt{}
	if err := jwt.Unmarshal(resp.GetJson()); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(
		m,
		map[string]interface{}{
			m.Name(): map[string]interface{}{
				"response": map[string]interface{}{
					"accessJWT":  jwt.AccessJwt,
					"refreshJWT": jwt.RefreshJwt}}},
		nil,
	), true

}

func getLoginInput(m schema.Mutation) *loginInput {
	// We should be able to convert these to string as GraphQL schema validation should ensure this.
	// If the input wasn't specified, then the arg value would be nil and the string value empty.

	var input loginInput

	input.UserId, _ = m.ArgValue("userId").(string)
	input.Password, _ = m.ArgValue("password").(string)
	input.RefreshToken, _ = m.ArgValue("refreshToken").(string)

	b, err := json.Marshal(m.ArgValue("namespace"))
	if err != nil {
		return nil
	}

	err = json.Unmarshal(b, &input.Namespace)
	if err != nil {
		return nil
	}
	return &input
}
