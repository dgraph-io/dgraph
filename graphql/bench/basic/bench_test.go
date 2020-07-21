/*
 * Copyright 2020 Dgraph Labs, Inc. and Contributors
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

package basic

import (
	"testing"

	"github.com/dgraph-io/dgraph/graphql/e2e/common"
	"github.com/stretchr/testify/require"
)

func BenchmarkSingleLevel_QueryRestaurant(t *testing.T) {
	query := `
	query($offset: Int) {
	  queryRestaurant (first: 100, offset: $offset) {
		id
		name
	  }
	}
	`

	params := &common.GraphQLParams{
		Query:     query,
		Variables: map[string]interface{}{"offset": 0},
	}

	offset := 0
	for i := 0; i < b.N; i++ {
		gqlResponse := params.ExecuteAsPost(b, graphqlURL)
		require.Nilf(b, gqlResponse.Errors, "%+v", gqlResponse.Errors)
		offset += 100
		params.Variables["offset"] = offset
	}
}
