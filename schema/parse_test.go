/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
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

package schema

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type nameType struct {
	name string
	typ  *pb.SchemaUpdate
}

func checkSchema(t *testing.T, h map[string]*pb.SchemaUpdate, expected []nameType) {
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
	checkSchema(t, State().predicate, []nameType{
		{"name", &pb.SchemaUpdate{
			Predicate: "name",
			ValueType: pb.Posting_STRING,
		}},
		{"_predicate_", &pb.SchemaUpdate{
			ValueType: pb.Posting_STRING,
			List:      true,
		}},
		{"address", &pb.SchemaUpdate{
			Predicate: "address",
			ValueType: pb.Posting_STRING,
		}},
		{"http://scalar.com/helloworld/", &pb.SchemaUpdate{
			Predicate: "http://scalar.com/helloworld/",
			ValueType: pb.Posting_STRING,
		}},
		{"age", &pb.SchemaUpdate{
			Predicate: "age",
			ValueType: pb.Posting_INT,
		}},
	})

	typ, err := State().TypeOf("age")
	require.NoError(t, err)
	require.Equal(t, types.IntID, typ)

	_, err = State().TypeOf("agea")
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
age:int @index(int) .

name: string .
address: string @index(term) .`

func TestSchemaIndex(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaIndexVal1), 1))
	require.Equal(t, 2, len(State().IndexedFields()))
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
age     : int @index(int) .
name    : string @index(exact) @count .
address : string @index(term) .
friend  : uid @reverse @count .
`

func TestSchemaIndexCustom(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaIndexVal5), 1))
	checkSchema(t, State().predicate, []nameType{
		{"_predicate_", &pb.SchemaUpdate{
			ValueType: pb.Posting_STRING,
			List:      true,
		}},
		{"name", &pb.SchemaUpdate{
			Predicate: "name",
			ValueType: pb.Posting_STRING,
			Tokenizer: []string{"exact"},
			Directive: pb.SchemaUpdate_INDEX,
			Count:     true,
		}},
		{"address", &pb.SchemaUpdate{
			Predicate: "address",
			ValueType: pb.Posting_STRING,
			Tokenizer: []string{"term"},
			Directive: pb.SchemaUpdate_INDEX,
		}},
		{"age", &pb.SchemaUpdate{
			Predicate: "age",
			ValueType: pb.Posting_INT,
			Tokenizer: []string{"int"},
			Directive: pb.SchemaUpdate_INDEX,
		}},
		{"friend", &pb.SchemaUpdate{
			ValueType: pb.Posting_UID,
			Predicate: "friend",
			Directive: pb.SchemaUpdate_REVERSE,
			Count:     true,
		}},
	})
	require.True(t, State().IsIndexed("name"))
	require.False(t, State().IsReversed("name"))
	require.Equal(t, "int", State().Tokenizer("age")[0].Name())
	require.Equal(t, 3, len(State().IndexedFields()))
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

func TestParse7_Error(t *testing.T) {
	reset()
	schemas, err := Parse("name:string @index .")
	require.Error(t, err)
	require.Nil(t, schemas)
}

func TestParse8_Error(t *testing.T) {
	reset()
	schemas, err := Parse("dob:dateTime @index .")
	require.Error(t, err)
	require.Nil(t, schemas)
}

func TestParseScalarList(t *testing.T) {
	reset()
	schemas, err := Parse(`
		jobs: [string] @index(term) .
		occupations: [string] .
		graduation: [dateTime] .
	`)
	require.NoError(t, err)
	require.Equal(t, 3, len(schemas))
	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: "jobs",
		ValueType: 9,
		Directive: pb.SchemaUpdate_INDEX,
		Tokenizer: []string{"term"},
		List:      true,
	}, schemas[0])

	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: "occupations",
		ValueType: 9,
		List:      true,
	}, schemas[1])

	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: "graduation",
		ValueType: 5,
		List:      true,
	}, schemas[2])
}

func TestParseScalarListError1(t *testing.T) {
	reset()
	schemas, err := Parse(`
		friend: [uid] .
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected scalar type inside []. Got: [uid] for attr: [friend].")
	require.Nil(t, schemas)
}

func TestParseScalarListError2(t *testing.T) {
	reset()
	schemas, err := Parse(`
		friend: [string .
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unclosed [ while parsing schema for: friend")
	require.Nil(t, schemas)
}

func TestParseScalarListError3(t *testing.T) {
	reset()
	schemas, err := Parse(`
		friend: string] .
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid ending")
	require.Nil(t, schemas)
}

func TestParseScalarListError4(t *testing.T) {
	reset()
	_, err := Parse(`
		friend: [bool] .
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported type for list: [bool]")
}

var ps *badger.DB

func TestMain(m *testing.M) {
	x.Init()

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	kvOpt := badger.DefaultOptions(dir)
	ps, err = badger.OpenManaged(kvOpt)
	x.Check(err)
	Init(ps)

	r := m.Run()

	ps.Close()
	os.RemoveAll(dir)
	os.Exit(r)
}

func TestParseUnderscore(t *testing.T) {
	reset()
	_, err := Parse("_share_:string @index(term) .")
	require.NoError(t, err)
}

func TestParseUpsert(t *testing.T) {
	reset()
	_, err := Parse(`
		jobs : string @index(exact) @upsert .
		age  : int @index(int) @upsert .
	`)
	require.NoError(t, err)
}
