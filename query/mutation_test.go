/*
 * Copyright 2016-2019 Dgraph Labs, Inc. and Contributors
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

package query

import (
	"context"
	"testing"
	"time"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

func TestReserverPredicateForMutation(t *testing.T) {
	err := addTriplesToCluster(`_:x <dgraph.graphql.schema> "df"`)
	require.Error(t, err, "Cannot mutate graphql reserved predicate dgraph.graphql.schema")
}

func TestAlterInternalTypes(t *testing.T) {
	ctx, _ := context.WithTimeout(context.Background(), 100*time.Second)

	dg, err := testutil.DgraphClientWithGroot(testutil.SockAddr)
	require.NoError(t, err)

	testutil.DropAll(t, dg)
	op := &api.Operation{Schema: `
		type dgraph.type.User {
			name: string
			age: int
		}
	`}
	err = dg.Alter(ctx, op)
	require.Error(t, err, "altering internal type shouldn't have succeeded")

	op = &api.Operation{Schema: `
		type dgraph.type.Group {
			name: string
			age: int
		}
	`}
	err = dg.Alter(ctx, op)
	require.Error(t, err, "altering internal type shouldn't have succeeded")

	op = &api.Operation{Schema: `
		type dgraph.type.Rule {
			name: string
			age: int
		}
	`}
	err = dg.Alter(ctx, op)
	require.Error(t, err, "altering internal type shouldn't have succeeded")
}
