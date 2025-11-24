/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"

	"github.com/golang/glog"
	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/v25/edgraph"
	"github.com/dgraph-io/dgraph/v25/graphql/resolve"
	"github.com/dgraph-io/dgraph/v25/graphql/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

func resolveHealth(ctx context.Context, q schema.Query) *resolve.Resolved {
	glog.Info("Got health request")

	resp, err := (&edgraph.Server{}).Health(ctx, true)
	if err != nil {
		return resolve.EmptyResult(q, errors.Errorf("%s: %s", x.Error, err.Error()))
	}

	var health []map[string]interface{}
	err = schema.Unmarshal(resp.GetJson(), &health)

	return resolve.DataResult(
		q,
		map[string]interface{}{q.Name(): health},
		err,
	)
}
