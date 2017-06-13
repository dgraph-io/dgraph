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

	"github.com/dgraph-io/badger/badger"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type nameType struct {
	name string
	typ  *protos.SchemaUpdate
}

func checkSchema(t *testing.T, h map[string]*protos.SchemaUpdate, expected []nameType) {
	require.Len(t, h, len(expected))
	for _, nt := range expected {
		typ, found := h[nt.name]
		require.True(t, found, nt)
		require.EqualValues(t, *nt.typ, *typ)
	}
}

var schemaVal = `
age:int .

name: string .
 address: string .
<http://scalar.com/helloworld/> : string .
`

func TestSchema(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaVal), 1))
	checkSchema(t, State().get(1).predicate, []nameType{
		{"name", &protos.SchemaUpdate{
			ValueType: uint32(types.StringID),
		}},
		{"address", &protos.SchemaUpdate{ValueType: uint32(types.StringID)}},
		{"http://scalar.com/helloworld/", &protos.SchemaUpdate{
			ValueType: uint32(types.StringID),
		}},
		{"age", &protos.SchemaUpdate{
			ValueType: uint32(types.IntID),
		}},
	})

	typ, err := State().TypeOf("age")
	require.NoError(t, err)
	require.Equal(t, types.IntID, typ)

	typ, err = State().TypeOf("agea")
	require.Error(t, err)
}

var schemaVal1 = `
age:int .

name: string .
address: string .

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
age:int @index .

name: string .
address: string @index .`

func TestSchemaIndex(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaIndexVal1), 1))
	require.Equal(t, 2, len(State().IndexedFields(1)))
}

var schemaIndexVal2 = `
name: string @index(exact, exact) .
address: string @index(term) .
id: id @index(exact, term, exact) .
`

// Duplicate tokenizers
func TestSchemaIndex_Error1(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaIndexVal2), 1))
}

var schemaIndexVal3Uid = `
person:uid @index .
`

var schemaIndexVal3Default = `
value:default @index .
`

var schemaIndexVal3Password = `
pass:password @index .
`

// Object types cant be indexed.
func TestSchemaIndex_Error2(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaIndexVal3Uid), 1))
	require.Error(t, ParseBytes([]byte(schemaIndexVal3Default), 1))
	require.Error(t, ParseBytes([]byte(schemaIndexVal3Password), 1))
}

var schemaIndexVal4 = `
name:string @index(exact term) .
`

// Missing comma.
func TestSchemaIndex_Error3(t *testing.T) {
	require.Error(t, ParseBytes([]byte(schemaIndexVal4), 1))
}

var schemaIndexVal5 = `
age:int @index(int) .

name: string @index(exact) .
address: string @index(term) .
id: id @index(exact, term) .
`

func TestSchemaIndexCustom(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaIndexVal5), 1))
	checkSchema(t, State().get(1).predicate, []nameType{
		{"name", &protos.SchemaUpdate{
			ValueType: uint32(types.StringID),
			Tokenizer: []string{"exact"},
			Directive: protos.SchemaUpdate_INDEX,
		}},
		{"address", &protos.SchemaUpdate{
			ValueType: uint32(types.StringID),
			Tokenizer: []string{"term"},
			Directive: protos.SchemaUpdate_INDEX,
		}},
		{"age", &protos.SchemaUpdate{
			ValueType: uint32(types.IntID),
			Tokenizer: []string{"int"},
			Directive: protos.SchemaUpdate_INDEX,
		}},
		{"id", &protos.SchemaUpdate{
			ValueType: uint32(types.StringID),
			Tokenizer: []string{"exact", "term"},
			Directive: protos.SchemaUpdate_INDEX,
		}},
	})
	require.True(t, State().IsIndexed("name"))
	require.False(t, State().IsReversed("name"))
	require.Equal(t, "int", State().Tokenizer("age")[0].Name())
	require.Equal(t, 4, len(State().IndexedFields(1)))
}

func TestParse(t *testing.T) {
	reset()
	_, err := Parse("age:int @index . name:string")
	require.Error(t, err)
}

func TestParse2(t *testing.T) {
	reset()
	schemas, err := Parse("")
	require.NoError(t, err)
	require.Nil(t, schemas)
}

func TestParse3_Error(t *testing.T) {
	reset()
	schemas, err := Parse("age:uid @index .")
	require.Error(t, err)
	require.Nil(t, schemas)
}

func TestParse4_Error(t *testing.T) {
	reset()
	schemas, err := Parse("alive:bool @index(geo) .")
	require.Equal(t, "Tokenizer: geo isn't valid for predicate: alive of type: bool",
		err.Error())
	require.Nil(t, schemas)
}

func TestParse4_NoError(t *testing.T) {
	reset()
	schemas, err := Parse("name:string @index(fulltext) .")
	require.NotNil(t, schemas)
	require.Nil(t, err)
}

func TestParse5_Error(t *testing.T) {
	reset()
	schemas, err := Parse("value:default @index .")
	require.Error(t, err)
	require.Nil(t, schemas)
}

func TestParse6_Error(t *testing.T) {
	reset()
	schemas, err := Parse("pass:password @index .")
	require.Error(t, err)
	require.Nil(t, schemas)
}

var ps *badger.KV

func TestMain(m *testing.M) {
	x.SetTestRun()
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	kvOpt := badger.DefaultOptions
	kvOpt.Dir = dir
	kvOpt.ValueDir = dir
	ps, err = badger.NewKV(&kvOpt)
	x.Check(err)
	x.Check(group.ParseGroupConfig("groups.conf"))
	Init(ps)
	defer os.RemoveAll(dir)
	defer ps.Close()

	os.Exit(m.Run())
}

func TestParseUnderscore(t *testing.T) {
	reset()
	_, err := Parse("_share_:string @index .")
	require.NoError(t, err)
}
