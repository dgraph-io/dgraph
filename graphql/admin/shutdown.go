/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/v25/graphql/resolve"
	"github.com/dgraph-io/dgraph/v25/graphql/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

func resolveShutdown(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got shutdown request through GraphQL admin API")

	x.ServerCloser.Signal()

	return resolve.DataResult(
		m,
		map[string]interface{}{m.Name(): response("Success", "Server is shutting down")},
		nil,
	), true
}
