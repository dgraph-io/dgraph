/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"encoding/json"

	"github.com/golang/glog"
	"google.golang.org/protobuf/proto"

	dgoapi "github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/hypermodeinc/dgraph/v25/edgraph"
	"github.com/hypermodeinc/dgraph/v25/graphql/resolve"
	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
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
	if err := proto.Unmarshal(resp.GetJson(), jwt); err != nil {
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
