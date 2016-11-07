/*
* Copyright 2016 DGraph Labs, Inc.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
* 		http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package worker

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/x"
)

func TestAddToMutationArray(t *testing.T) {
	group.ParseGroupConfig("")
	dir, err := ioutil.TempDir("", "storetest_")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	mutationsMap := make(map[uint32]*x.Mutations)
	edges := []x.DirectedEdge{}

	edges = append(edges, x.DirectedEdge{
		Value:     []byte("set edge"),
		Source:    "test-mutation",
		Timestamp: time.Now(),
	})

	addToMutationMap(mutationsMap, edges, set)
	mu := mutationsMap[0]
	require.NotNil(t, mu)
	require.NotNil(t, mu.Set)

	addToMutationMap(mutationsMap, edges, del)
	mu = mutationsMap[0]
	require.NotNil(t, mu)
	require.NotNil(t, mu.Del)

}
