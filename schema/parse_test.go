/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
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

var schemaVal = `
age:int

name: string
 address: string
<http://scalar.com/helloworld/> : string
`

func TestSchema(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaVal), 1))
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

var schemaVal1 = `
age:int

name: string
address: string

)
`

func TestSchema1_Error(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaVal1), 1))
}

var schemaVal2 = `
name: ( string
`

func TestSchema2_Error(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaVal2), 1))
}

var schemaVal3 = `
test test: int
`

func TestSchema3_Error(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaVal3), 1))
}

var schemaIndexVal1 = `
age:int @index

name: string 
address: string @index
`

func TestSchemaIndex(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaIndexVal1), 1))
	require.Equal(t, 2, len(State().IndexedFields(1)))
}

var schemaIndexVal2 = `
age:uid @index(int)

name: string @index(exact, exact)
address: string @index(term)
id: id @index(exact, term, exact)
`

// Duplicate tokenizers
func TestSchemaIndex_Error1(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaIndexVal2), 1))
}

var schemaIndexVal3 = `
person:uid @index
`

// Object types cant be indexed.
func TestSchemaIndex_Error2(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaIndexVal3), 1))
}

var schemaIndexVal4 = `
name:string @index(exact term)
`

// Missing comma.
func TestSchemaIndex_Error3(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaIndexVal4), 1))
}

var schemaIndexVal5 = `
age:int @index(int)

name: string @index(exact)
address: string @index(term)
id: id @index(exact, term)
`

func TestSchemaIndexCustom(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaIndexVal5), 1))
	checkSchema(t, State().get(1).predicate, []nameType{
		{"name", &typesp.Schema{
			ValueType: uint32(types.StringID),
			Tokenizer: []string{"exact"},
			Directive: typesp.Schema_INDEX,
		}},
		{"address", &typesp.Schema{
			ValueType: uint32(types.StringID),
			Tokenizer: []string{"term"},
			Directive: typesp.Schema_INDEX,
		}},
		{"age", &typesp.Schema{
			ValueType: uint32(types.Int32ID),
			Tokenizer: []string{"int"},
			Directive: typesp.Schema_INDEX,
		}},
		{"id", &typesp.Schema{
			ValueType: uint32(types.StringID),
			Tokenizer: []string{"exact", "term"},
			Directive: typesp.Schema_INDEX,
		}},
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
