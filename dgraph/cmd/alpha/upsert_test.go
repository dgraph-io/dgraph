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

package alpha

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpsertMutation(t *testing.T) {
	require.NoError(t, dropAll())

	require.NoError(t, alterSchema(`name: string @index(term) .`))

	q1 := `
	{
	  balances(func: anyofterms(name, "Alice Bob")) {
	    name
	    balance
	  }
	}
	`
	_, ts, err := queryWithTs(q1, "application/graphqlpm", 0)
	require.NoError(t, err)
	m1 := `
    upsert {
        mutation {
          	set {
				uid(v) <name> "Some One" .
				uid(v) <email> "someone@dgraph.io" .
          	}
        }

        query {
			me(func: eq(email, "someone@dgraph.io")) {
				v as uid
			}
        }
    }
	`

	keys, preds, mts, err := mutationWithTs(m1, "application/rdf", false, false, true, ts)
	require.NoError(t, err)
	require.Equal(t, mts, ts)
	require.Equal(t, 3, len(keys))
	require.Equal(t, 2, len(preds))
}
