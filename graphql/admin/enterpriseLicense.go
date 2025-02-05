/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"encoding/json"

	"github.com/hypermodeinc/dgraph/v24/graphql/resolve"
	"github.com/hypermodeinc/dgraph/v24/graphql/schema"
	"github.com/hypermodeinc/dgraph/v24/protos/pb"
	"github.com/hypermodeinc/dgraph/v24/worker"
)

type enterpriseLicenseInput struct {
	License string
}

func resolveEnterpriseLicense(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	input, err := getEnterpriseLicenseInput(m)
	if err != nil {
		return resolve.EmptyResult(m, err), false
	}

	if _, err = worker.ApplyLicenseOverNetwork(
		ctx,
		&pb.ApplyLicenseRequest{License: []byte(input.License)},
	); err != nil {
		return resolve.EmptyResult(m, err), false
	}

	return resolve.DataResult(m,
		map[string]interface{}{m.Name(): response("Success", "License applied.")},
		nil,
	), true
}

func getEnterpriseLicenseInput(m schema.Mutation) (*enterpriseLicenseInput, error) {
	inputArg := m.ArgValue(schema.InputArgName)
	inputBytes, err := json.Marshal(inputArg)
	if err != nil {
		return nil, inputArgError(err)
	}

	var input enterpriseLicenseInput
	err = schema.Unmarshal(inputBytes, &input)
	return &input, inputArgError(err)
}
