/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package schema

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type nameType struct {
	name string
	typ  *types.Schema
}

func checkSchema(t *testing.T, h map[string]*types.Schema, expected []nameType) {
	require.Len(t, h, len(expected))
	for _, nt := range expected {
		typ, found := h[nt.name]
		require.True(t, found, nt)
		require.EqualValues(t, *nt.typ, *typ)
	}
}

func TestSchema(t *testing.T) {
	require.NoError(t, Init(ps, "testfiles/test_schema"))
	checkSchema(t, State().predicate, []nameType{
		{"name", &types.Schema{ValueType: uint32(types.StringID)}},
		{"address", &types.Schema{ValueType: uint32(types.StringID)}},
		{"http://film.com/name", &types.Schema{ValueType: uint32(types.StringID)}},
		{"http://scalar.com/helloworld/", &types.Schema{ValueType: uint32(types.StringID)}},
		{"age", &types.Schema{ValueType: uint32(types.Int32ID)}},
		{"budget", &types.Schema{ValueType: uint32(types.Int32ID)}},
		{"http://film.com/budget", &types.Schema{ValueType: uint32(types.Int32ID)}},
		{"NumFollower", &types.Schema{ValueType: uint32(types.Int32ID)}},
		{"Person", &types.Schema{ValueType: uint32(types.UidID)}},
		{"Actor", &types.Schema{ValueType: uint32(types.UidID)}},
		{"Film", &types.Schema{ValueType: uint32(types.UidID)}},
		{"http://film.com/", &types.Schema{ValueType: uint32(types.UidID)}},
	})

	typ, err := State().TypeOf("age")
	require.NoError(t, err)
	require.Equal(t, types.Int32ID, typ)

	typ, err = State().TypeOf("agea")
	require.Error(t, err)
}

func TestSchema1_Error(t *testing.T) {
	require.Error(t, Init(ps, "testfiles/test_schema1"))
}

func TestSchema2_Error(t *testing.T) {
	require.Error(t, Init(ps, "testfiles/test_schema2"))
}

func TestSchema3_Error(t *testing.T) {
	require.Error(t, Init(ps, "testfiles/test_schema3"))
}

func TestSchema4_Error(t *testing.T) {
	require.Error(t, Init(ps, "testfiles/test_schema4"))
}

/*
func TestSchema5_Error(t *testing.T) {
	str = make(map[string]types.TypeID)
	err := Parse("testfiles/test_schema5")
	require.Error(t, err)
}

func TestSchema6_Error(t *testing.T) {
	str = make(map[string]types.TypeID)
	err := Parse("testfiles/test_schema6")
	require.Error(t, err)
}
*/
// Correct specification of indexing
func TestSchemaIndex(t *testing.T) {
	require.NoError(t, Init(ps, "testfiles/test_schema_index1"))
	require.Equal(t, 2, len(State().IndexedFields()))
}

// Indexing can't be specified inside object types.
func TestSchemaIndex_Error1(t *testing.T) {
	require.Error(t, Init(ps, "testfiles/test_schema_index2"))
}

// Object types cant be indexed.
func TestSchemaIndex_Error2(t *testing.T) {
	require.Error(t, Init(ps, "testfiles/test_schema_index3"))
}

func TestSchemaIndexCustom(t *testing.T) {
	require.NoError(t, Init(ps, "testfiles/test_schema_index4"))
	checkSchema(t, State().predicate, []nameType{
		{"name", &types.Schema{ValueType: uint32(types.StringID), Tokenizer: "exact"}},
		{"address", &types.Schema{ValueType: uint32(types.StringID), Tokenizer: "term"}},
		{"age", &types.Schema{ValueType: uint32(types.Int32ID), Tokenizer: "int"}},
		{"Person", &types.Schema{ValueType: uint32(types.UidID)}},
	})
	require.True(t, State().IsIndexed("name"))
	require.False(t, State().IsReversed("name"))
	require.Equal(t, "int", State().Tokenizer("age").Name())
	require.Equal(t, 3, len(State().IndexedFields()))
}

var ps *store.Store

func TestMain(m *testing.M) {
	x.SetTestRun()
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	ps, err = store.NewStore(dir)
	x.Check(err)
	defer os.RemoveAll(dir)
	defer ps.Close()

	os.Exit(m.Run())
}
