/*
 * Copyright 2019 Dgraph Labs, Inc. and Contributors
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

package resolve

import (
	"context"
	"encoding/json"

	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/dgraph"
	"github.com/dgraph-io/dgraph/dgraph/cmd/graphql/schema"
	"github.com/vektah/gqlparser/gqlerror"
)

// QueryResolver can resolve a single GraphQL query field
type QueryResolver struct {
	query  schema.Query
	schema schema.Schema
	dgraph dgraph.Client
}

// Resolve a single query.
func (qr *QueryResolver) Resolve(ctx context.Context) ([]byte, error) {

	var qb *dgraph.QueryBuilder

	// currently only handles getT(id: "0x123") queries
	switch qr.query.QueryType() {
	case schema.GetQuery:
		qb = dgraph.NewQueryBuilder().
			WithAttr(qr.query.ResponseName()).
			WithIDArgRoot(qr.query).
			WithTypeFilter(qr.query.Type().Name()).
			WithSelectionSetFrom(qr.query)
		// TODO: also builder.withPagination() ... etc ...
	default:
		return nil, gqlerror.Errorf("Only get queries are implemented")
	}

	res, err := qr.dgraph.Query(ctx, qb)
	if err != nil {
		glog.Infof("Dgraph query failed : %s", err)
		return nil, schema.GQLWrapf(err, "failed to resolve query")
	}

	var data map[string]interface{}
	err = json.Unmarshal(res, &data)
	if err != nil {
		glog.Errorf("Failed to unmarshal query result : %v+", err)
		return nil, schema.GQLWrapf(err, "internal error, couldn't unmarshal dgraph result")
	}

	return completeDgraphResult(qr.query, data[qr.query.ResponseName()])
}
