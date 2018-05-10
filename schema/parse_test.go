/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package schema

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/badger"
	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

type nameType struct {
	name string
	typ  *intern.SchemaUpdate
}

func checkSchema(t *testing.T, h map[string]*intern.SchemaUpdate, expected []nameType) {
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
		{"name", &intern.SchemaUpdate{
			Predicate: "name",
			ValueType: intern.Posting_STRING,
		}},
		{"_predicate_", &intern.SchemaUpdate{
			ValueType: intern.Posting_STRING,
			List:      true,
		}},
		{"address", &intern.SchemaUpdate{
			Predicate: "address",
			ValueType: intern.Posting_STRING,
		}},
		{"http://scalar.com/helloworld/", &intern.SchemaUpdate{
			Predicate: "http://scalar.com/helloworld/",
			ValueType: intern.Posting_STRING,
		}},
		{"age", &intern.SchemaUpdate{
			Predicate: "age",
			ValueType: intern.Posting_INT,
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
		{"_predicate_", &intern.SchemaUpdate{
			ValueType: intern.Posting_STRING,
			List:      true,
		}},
		{"name", &intern.SchemaUpdate{
			Predicate: "name",
			ValueType: intern.Posting_STRING,
			Tokenizer: []string{"exact"},
			Directive: intern.SchemaUpdate_INDEX,
			Count:     true,
		}},
		{"address", &intern.SchemaUpdate{
			Predicate: "address",
			ValueType: intern.Posting_STRING,
			Tokenizer: []string{"term"},
			Directive: intern.SchemaUpdate_INDEX,
		}},
		{"age", &intern.SchemaUpdate{
			Predicate: "age",
			ValueType: intern.Posting_INT,
			Tokenizer: []string{"int"},
			Directive: intern.SchemaUpdate_INDEX,
		}},
		{"friend", &intern.SchemaUpdate{
			ValueType: intern.Posting_UID,
			Predicate: "friend",
			Directive: intern.SchemaUpdate_REVERSE,
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
	require.EqualValues(t, &intern.SchemaUpdate{
		Predicate: "jobs",
		ValueType: 9,
		Directive: intern.SchemaUpdate_INDEX,
		Tokenizer: []string{"term"},
		List:      true,
	}, schemas[0])

	require.EqualValues(t, &intern.SchemaUpdate{
		Predicate: "occupations",
		ValueType: 9,
		List:      true,
	}, schemas[1])

	require.EqualValues(t, &intern.SchemaUpdate{
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

var ps *badger.ManagedDB

func TestMain(m *testing.M) {
	x.Init(true)

	dir, err := ioutil.TempDir("", "storetest_")
	x.Check(err)
	kvOpt := badger.DefaultOptions
	kvOpt.Dir = dir
	kvOpt.ValueDir = dir
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
