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

package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/testutil"
	"github.com/stretchr/testify/require"
)

func TestOverwriteUidValues(t *testing.T) {
	dc, err := testutil.DgraphClient(testutil.SockAddr)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, dc.Alter(ctx, &api.Operation{DropAll: true}))
	require.NoError(t, dc.Alter(ctx, &api.Operation{
		Schema: `best_friend: uid .`,
	}))

	txn := dc.NewTxn()
	assigned, err := txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(`
			_:alicia <name> "Alicia" .
			_:ben <name> "Ben" .
			_:carl <name> "Carl" .
			_:alicia <best_friend> _:ben .
		`),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))

	txn = dc.NewTxn()
	_, err = txn.Mutate(ctx, &api.Mutation{
		SetNquads: []byte(fmt.Sprintf(`<%s> <best_friend> <%s> .`, assigned.Uids["alicia"],
			assigned.Uids["carl"])),
	})
	require.NoError(t, err)
	require.NoError(t, txn.Commit(ctx))
}
