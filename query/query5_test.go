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

package query

import (
	"context"
	"testing"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/stretchr/testify/require"
)

func doCondMutation(query, mutation string) (*api.Assigned, error) {
	txn := client.NewTxn()
	ctx := context.Background()
	defer txn.Discard(ctx)
	return txn.Mutate(ctx, &api.Mutation{
		CondQuery: query,
		SetNquads: []byte(mutation),
		CommitNow: true,
	})
}

func TestTxnMutation(t *testing.T) {
	mutation := `
		uid(v) <name> "Gus" .
		uid(v) <profession> "Zombie Hunter" .
	`

	tests := []struct {
		query, failure string
	}{
		{query: `
			query {
				me(func: eq(alias, "Zambo Alice")) {
					v as uid
				}
			}`},
		{query: `
			query todo($oldname: string = "Rick Grimes") {
				me(func: eq(alias, "Zambo Alice")) {
					v as uid
				}
			}`},
		{query: `
			query {
				me(func: eq(alias, "Zambo Alice")) {
					x as uid
				}
			}`,
			failure: `Subject must not be empty for nquad: predicate:"name"`,
		},
		{query: `
			query {
				me(func: eq(alias, "Zambo Alice")) {
					uid
				}
			}`,
			failure: `No variables defined in conditional query`,
		},
		{query: `
			query {
				me(func: eq(alias, "srfrog")) {
					uid
				}
			}`,
			failure: `No variables defined in conditional query`,
		},
		{query: `query {}`,
			failure: `Expected some name.`,
		},
		{query: "",
			failure: `Subject must not be empty for nquad: predicate:"name"`,
		},
	}
	for _, tc := range tests {
		assigned, err := doCondMutation(tc.query, mutation)
		if tc.failure == "" {
			require.NoError(t, err)
			require.Nil(t, assigned.Uids)
		} else {
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.failure)
		}
	}
}
