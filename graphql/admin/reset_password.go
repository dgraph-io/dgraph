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
	"strconv"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/edgraph"
	"github.com/dgraph-io/dgraph/graphql/resolve"
	"github.com/dgraph-io/dgraph/graphql/schema"
)

func resolveResetPassword(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	inp, err := getPasswordInput(m)
	if err != nil {
		glog.Error("Failed to parse the reset password input")
	}
	if err = (&edgraph.Server{}).ResetPassword(ctx, inp); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(
		m,
		map[string]interface{}{
			m.Name(): map[string]interface{}{
				"userId":    inp.UserID,
				"message":   "Reset password is successful",
				"namespace": json.Number(strconv.Itoa(int(inp.Namespace))),
			},
		},
		nil,
	), true

}

func getPasswordInput(m schema.Mutation) (*edgraph.ResetPasswordInput, error) {
	var input edgraph.ResetPasswordInput

	inputArg := m.ArgValue(schema.InputArgName)
	inputByts, err := json.Marshal(inputArg)

	if err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	if err := json.Unmarshal(inputByts, &input); err != nil {
		return nil, schema.GQLWrapf(err, "couldn't get input argument")
	}

	return &input, nil
}
