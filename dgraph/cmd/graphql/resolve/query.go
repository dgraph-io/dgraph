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
			WithTypeFilter(qr.query.TypeName()).
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

	// TODO:
	// queries come back from Dgraph like :
	// {"mult":[{ ... }, { ... }]} - multiple results
	// {"single":[{ ... }]}        - single result
	//
	// QueryResolver.Resolve() should return a fully resolved GraphQL answer.
	// That means a bunch of things in GraphQL:
	//
	// - need to dig through that response and the expected types from the
	//   schema and propagate missing ! fields and errors according to spec.
	//
	// - if schema result is a single node not a list, then need to transform
	//   {"single":[{ ... }]} ---> "single":{ ... }

	// atm we are just chopping off the {}
	if len(res) > 2 {
		// chop leading '{' and trailing '}'
		res = res[1 : len(res)-1]
	} else {
		res = []byte{}
	}
	return res, nil
}
