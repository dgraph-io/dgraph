/*
 * Copyright 2016 Dgraph Labs, Inc.
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

package query

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/worker"
	"github.com/dgraph-io/dgraph/x"
)

func createTestStore(t *testing.T) *store.Store {
	dir, err := ioutil.TempDir("", "storetest_")
	if err != nil {
		t.Error(err)
		return nil
	}

	defer os.RemoveAll(dir)
	ps, err := store.NewStore(dir)
	if err != nil {
		t.Error(err)
		return nil
	}
	defer ps.Close()

	worker.SetState(ps)

	posting.Init()
	posting.InitIndex(ps)
	return ps
}

func addEdgeTo(t *testing.T, edge x.DirectedEdge, ps *store.Store) {
	addEdge(t, edge, getOrCreate(posting.Key(edge.Entity, edge.Attribute), ps))
}

func createTestData(t *testing.T, ps *store.Store) {
	edge := x.DirectedEdge{
		Timestamp: time.Now(),
		Attribute: "location",
		ValueType: byte(types.GeoID),
	}
	var g types.Geo

	err := g.UnmarshalText([]byte(""))
	require.NoError(t, err)

	edge.Entity = 10
	edge.Value, err = g.MarshalBinary()
	require.NoError(t, err)

	addEdgeTo(t, edge, ps)
}
