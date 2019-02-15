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

package edgelim

import (
	"context"
	"os"
	"testing"

	"github.com/dgraph-io/dgraph/query"
	"github.com/stretchr/testify/require"
)

// Tests in this file require a cluster running with --query_edge_limit=5.

func TestRecurseEdgeLimitError(t *testing.T) {
	q := `
		{
			me(func: uid(0x01)) @recurse(loop: true, depth: 2) {
				friend
				name
			}
		}`
	_, err := query.ProcessQuery(t, context.Background(), q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Exceeded query edge limit")
}

func TestShortestPath_EdgeLimitError(t *testing.T) {
	q := `
		{
			shortest(from:0x01, to:101) {
              friend
			}
		}`

	_, err := query.ProcessQuery(t, context.Background(), q)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Exceeded query edge limit")
}

func TestMain(m *testing.M) {
	query.PopulateCluster()
	os.Exit(m.Run())
}
