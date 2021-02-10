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
	"encoding/json"
	"strconv"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
	"github.com/golang/glog"
)

type passwordInput struct {
	UserID    string
	Password  string
	Namespace uint64
}

func resolveResetPassword(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got reset password request")

	in := getPasswordInput(m)
	err := (&edgraph.Server{}).ResetPassword(ctx, in.Namespace, in.UserID, in.Password)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(
		m,
		map[string]interface{}{
			m.Name(): map[string]interface{}{
				"userId":    in.UserID,
				"message":   "Reset password is successful",
				"namespace": json.Number(strconv.Itoa(int(in.Namespace))),
			},
		},
		nil,
	), true

}

func getPasswordInput(m schema.Mutation) *passwordInput {
	var input passwordInput

	input.UserID, _ = m.ArgValue("userId").(string)
	input.Password, _ = m.ArgValue("password").(string)

	b, err := json.Marshal(m.ArgValue("namespace"))
	if err != nil {
		return nil
	}

	if err = json.Unmarshal(b, &input.Namespace); err != nil {
		return nil
	}
	return &input
}
