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

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/store"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type nameType struct {
	name string
	typ  *typesp.Schema
}

func checkSchema(t *testing.T, h map[string]*typesp.Schema, expected []nameType) {
	require.Len(t, h, len(expected))
	for _, nt := range expected {
		typ, found := h[nt.name]
		require.True(t, found, nt)
		require.EqualValues(t, *nt.typ, *typ)
	}
}

func TestSchema(t *testing.T) {
	require.NoError(t, ReloadData("testfiles/test_schema", 1))
	checkSchema(t, State().get(1).predicate, []nameType{
		{"name", &typesp.Schema{ValueType: uint32(types.StringID)}},
		{"address", &typesp.Schema{ValueType: uint32(types.StringID)}},
		{"http://scalar.com/helloworld/", &typesp.Schema{ValueType: uint32(types.StringID)}},
		{"age", &typesp.Schema{ValueType: uint32(types.Int32ID)}},
	})

	typ, err := State().TypeOf("age")
	require.NoError(t, err)
	require.Equal(t, types.Int32ID, typ)

	typ, err = State().TypeOf("agea")
	require.Error(t, err)
}

func TestSchema1_Error(t *testing.T) {
	require.Error(t, ReloadData("testfiles/test_schema1", 1))
}

func TestSchema2_Error(t *testing.T) {
	require.Error(t, ReloadData("testfiles/test_schema2", 1))
}

func TestSchema3_Error(t *testing.T) {
	require.Error(t, ReloadData("testfiles/test_schema3", 1))
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
	require.NoError(t, ReloadData("testfiles/test_schema_index1", 1))
	require.Equal(t, 2, len(State().IndexedFields(1)))
}

// Indexing can't be specified inside object types.
func TestSchemaIndex_Error1(t *testing.T) {
	require.Error(t, ReloadData("testfiles/test_schema_index2", 1))
}

// Object types cant be indexed.
func TestSchemaIndex_Error2(t *testing.T) {
	require.Error(t, ReloadData("testfiles/test_schema_index3", 1))
}

func TestSchemaIndexCustom(t *testing.T) {
	require.NoError(t, ReloadData("testfiles/test_schema_index4", 1))
	checkSchema(t, State().get(1).predicate, []nameType{
		{"name", &typesp.Schema{ValueType: uint32(types.StringID), Tokenizer: []string{"exact"}, Directive: typesp.Schema_INDEX}},
		{"address", &typesp.Schema{ValueType: uint32(types.StringID), Tokenizer: []string{"term"}, Directive: typesp.Schema_INDEX}},
		{"age", &typesp.Schema{ValueType: uint32(types.Int32ID), Tokenizer: []string{"int"}, Directive: typesp.Schema_INDEX}},
		{"id", &typesp.Schema{ValueType: uint32(types.StringID), Tokenizer: []string{"exact", "term"}, Directive: typesp.Schema_INDEX}},
	})
	require.True(t, State().IsIndexed("name"))
	require.False(t, State().IsReversed("name"))
	require.Equal(t, "int", State().Tokenizer("age")[0].Name())
	require.Equal(t, 4, len(State().IndexedFields(1)))
}

func TestParse(t *testing.T) {
	reset()
	schemas, err := Parse("age:int @index name:string")
	require.NoError(t, err)
	require.Equal(t, "int", schemas[0].Tokenizer[0])
	require.Equal(t, 2, len(schemas))
}

func TestParse2(t *testing.T) {
	reset()
	schemas, err := Parse("")
	require.NoError(t, err)
	require.Nil(t, schemas)
}

func TestParse3_Error(t *testing.T) {
	reset()
	schemas, err := Parse("age:uid @index")
	require.Error(t, err)
	require.Nil(t, schemas)
}

var ps *store.Store

func TestMain(m *testing.M) {
	x.SetTestRun()
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	ps, err = store.NewStore(dir)
	x.Check(err)
	x.Check(group.ParseGroupConfig("groups.conf"))
	Init(ps)
	defer os.RemoveAll(dir)
	defer ps.Close()

	os.Exit(m.Run())
}
