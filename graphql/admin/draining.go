/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package admin

import (
	"context"
	"fmt"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/v25/graphql/resolve"
	"github.com/dgraph-io/dgraph/v25/graphql/schema"
	"github.com/dgraph-io/dgraph/v25/x"
)

func resolveDraining(ctx context.Context, m schema.Mutation) (*resolve.Resolved, bool) {
	glog.Info("Got draining request through GraphQL admin API")

	enable := getDrainingInput(m)
	x.UpdateDrainingMode(enable)

	return resolve.DataResult(
		m,
		map[string]interface{}{
			m.Name(): response("Success", fmt.Sprintf("draining mode has been set to %v", enable)),
		},
		nil,
	), true
}

func getDrainingInput(m schema.Mutation) bool {
	enable, _ := m.ArgValue("enable").(bool)
	return enable
}
