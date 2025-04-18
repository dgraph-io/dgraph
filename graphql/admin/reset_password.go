/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/golang/glog"

	"github.com/hypermodeinc/dgraph/v25/edgraph"
	"github.com/hypermodeinc/dgraph/v25/graphql/resolve"
	"github.com/hypermodeinc/dgraph/v25/graphql/schema"
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
