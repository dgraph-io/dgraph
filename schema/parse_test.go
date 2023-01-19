/*
 * Copyright 2016-2022 Dgraph Labs, Inc. and Contributors
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
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgraph-io/badger/v3"
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
		{x.GalaxyAttr("name"), &pb.SchemaUpdate{
			Predicate: x.GalaxyAttr("name"),
			ValueType: pb.Posting_STRING,
		}},
		{x.GalaxyAttr("address"), &pb.SchemaUpdate{
			Predicate: x.GalaxyAttr("address"),
			ValueType: pb.Posting_STRING,
		}},
		{x.GalaxyAttr("http://scalar.com/helloworld/"), &pb.SchemaUpdate{
			Predicate: x.GalaxyAttr("http://scalar.com/helloworld/"),
			ValueType: pb.Posting_STRING,
		}},
		{x.GalaxyAttr("age"), &pb.SchemaUpdate{
			Predicate: x.GalaxyAttr("age"),
			ValueType: pb.Posting_INT,
		}},
	})

	typ, err := State().TypeOf(x.GalaxyAttr("age"))
	require.NoError(t, err)
	require.Equal(t, types.IntID, typ)

	_, err = State().TypeOf(x.GalaxyAttr("agea"))
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
person: uid @index .
`

var schemaIndexVal3Default = `
value: default @index .
`

var schemaIndexVal3Password = `
pass: password @index .
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
friend  : [uid] @reverse @count .
`

func TestSchemaIndexCustom(t *testing.T) {
	require.NoError(t, ParseBytes([]byte(schemaIndexVal5), 1))
	checkSchema(t, State().predicate, []nameType{
		{x.GalaxyAttr("name"), &pb.SchemaUpdate{
			Predicate: x.GalaxyAttr("name"),
			ValueType: pb.Posting_STRING,
			Tokenizer: []string{"exact"},
			Directive: pb.SchemaUpdate_INDEX,
			Count:     true,
		}},
		{x.GalaxyAttr("address"), &pb.SchemaUpdate{
			Predicate: x.GalaxyAttr("address"),
			ValueType: pb.Posting_STRING,
			Tokenizer: []string{"term"},
			Directive: pb.SchemaUpdate_INDEX,
		}},
		{x.GalaxyAttr("age"), &pb.SchemaUpdate{
			Predicate: x.GalaxyAttr("age"),
			ValueType: pb.Posting_INT,
			Tokenizer: []string{"int"},
			Directive: pb.SchemaUpdate_INDEX,
		}},
		{x.GalaxyAttr("friend"), &pb.SchemaUpdate{
			ValueType: pb.Posting_UID,
			Predicate: x.GalaxyAttr("friend"),
			Directive: pb.SchemaUpdate_REVERSE,
			Count:     true,
			List:      true,
		}},
	})
	require.True(t, State().IsIndexed(context.Background(), x.GalaxyAttr("name")))
	require.False(t, State().IsReversed(context.Background(), x.GalaxyAttr("name")))
	require.Equal(t, "int", State().Tokenizer(context.Background(), x.GalaxyAttr("age"))[0].Name())
}

func TestParse(t *testing.T) {
	reset()
	_, err := Parse("age:int @index . name:string")
	require.Error(t, err)
}

func TestParse2(t *testing.T) {
	reset()
	result, err := Parse("")
	require.NoError(t, err)
	require.Nil(t, result.Preds)
}

func TestParse3_Error(t *testing.T) {
	reset()
	result, err := Parse("age:uid @index .")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestParse4_Error(t *testing.T) {
	reset()
	result, err := Parse("alive:bool @index(geo) .")
	require.Contains(t, err.Error(),
		"Tokenizer: geo isn't valid for predicate: alive of type: bool")
	require.Nil(t, result)
}

func TestParse4_NoError(t *testing.T) {
	reset()
	result, err := Parse("name:string @index(fulltext) .")
	require.NotNil(t, result)
	require.Nil(t, err)
}

func TestParse5_Error(t *testing.T) {
	reset()
	result, err := Parse("value:default @index .")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestParse6_Error(t *testing.T) {
	reset()
	result, err := Parse("pass:password @index .")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestParse7_Error(t *testing.T) {
	reset()
	result, err := Parse("name:string @index .")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestParse8_Error(t *testing.T) {
	reset()
	result, err := Parse("dob:dateTime @index .")
	require.Error(t, err)
	require.Nil(t, result)
}

func TestParse9_Error(t *testing.T) {
	reset()
	result, err := Parse("age:uid @noconflict .")
	require.NotNil(t, result)
	require.NoError(t, err)
}

func TestParseScalarList(t *testing.T) {
	reset()
	result, err := Parse(`
		jobs: [string] @index(term) .
		occupations: [string] .
		graduation: [dateTime] .
	`)
	require.NoError(t, err)
	require.Equal(t, 3, len(result.Preds))
	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("jobs"),
		ValueType: 9,
		Directive: pb.SchemaUpdate_INDEX,
		Tokenizer: []string{"term"},
		List:      true,
	}, result.Preds[0])

	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("occupations"),
		ValueType: 9,
		List:      true,
	}, result.Preds[1])

	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("graduation"),
		ValueType: 5,
		List:      true,
	}, result.Preds[2])
}

func TestParseScalarListError1(t *testing.T) {
	reset()
	result, err := Parse(`
		friend: [string .
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unclosed [ while parsing schema for: friend")
	require.Nil(t, result)
}

func TestParseScalarListError2(t *testing.T) {
	reset()
	result, err := Parse(`
		friend: string] .
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid ending")
	require.Nil(t, result)
}

func TestParseScalarListError3(t *testing.T) {
	reset()
	_, err := Parse(`
		friend: [bool] .
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported type for list: [bool]")
}

func TestParseUidList(t *testing.T) {
	reset()
	result, err := Parse(`
		friend: [uid] .
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Preds))
	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("friend"),
		ValueType: 7,
		List:      true,
	}, result.Preds[0])
}

func TestParseUidSingleValue(t *testing.T) {
	reset()
	result, err := Parse(`
		friend: uid .
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Preds))
	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("friend"),
		ValueType: 7,
		List:      false,
	}, result.Preds[0])
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

func TestParseEmptyType(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {

		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.GalaxyAttr("Person"),
	}, result.Types[0])

}

func TestParseTypeEOF(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
		}`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.GalaxyAttr("Person"),
	}, result.Types[0])

}

func TestParseSingleType(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			name
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.GalaxyAttr("Person"),
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
			},
		},
	}, result.Types[0])
}

func TestParseCombinedSchemasAndTypes(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			name
		}
        name: string .
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Preds))
	require.Equal(t, &pb.SchemaUpdate{
		Predicate: x.GalaxyAttr("name"),
		ValueType: 9,
	}, result.Preds[0])
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.GalaxyAttr("Person"),
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
			},
		},
	}, result.Types[0])
}

func TestParseMultipleTypes(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			name
		}
		type Animal {
			name
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 2, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.GalaxyAttr("Person"),
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
			},
		},
	}, result.Types[0])
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.GalaxyAttr("Animal"),
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
			},
		},
	}, result.Types[1])
}

func TestParseTypeDuplicateFields(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
			name
			name
		}
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Duplicate fields with name: name")
}

func TestOldTypeFormat(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			name: [string!]!
			address: string!
			children: [Person]
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.GalaxyAttr("Person"),
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
			},
			{
				Predicate: x.GalaxyAttr("address"),
			},
			{
				Predicate: x.GalaxyAttr("children"),
			},
		},
	}, result.Types[0])
}

func TestOldAndNewTypeFormat(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			name: [string!]!
			address
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.GalaxyAttr("Person"),
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.GalaxyAttr("name"),
			},
			{
				Predicate: x.GalaxyAttr("address"),
			},
		},
	}, result.Types[0])
}

func TestParseTypeErrMissingNewLine(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
		}type Animal {}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected new line or EOF after type declaration")
}

func TestParseComments(t *testing.T) {
	reset()
	_, err := Parse(`
		#
		# This is a test
		#
		user: bool .
		user.name: string @index(exact) . # this should be unique
		user.email: string @index(exact) . # this should be unique and lower-cased
		user.password: password .
		user.code: string . # for password recovery (can be null)
		node: bool .
		node.hashid: string @index(exact) . # @username/hashid
		node.owner: uid @reverse . # (can be null)
		node.parent: uid . # [uid] (use facet)
		node.xdata: string . # store custom json data
		#
		# End of test
		#
	`)
	require.NoError(t, err)
}

func TestParseCommentsNoop(t *testing.T) {
	reset()
	_, err := Parse(`
# Spicy jalapeno bacon ipsum dolor amet t-bone kevin spare ribs sausage jowl cow pastrami short.
# Leberkas alcatra kielbasa chicken pastrami swine bresaola. Spare ribs landjaeger meatloaf.
# Chicken biltong boudin porchetta jowl swine burgdoggen cow kevin ground round landjaeger ham.
# Tongue buffalo cow filet mignon boudin sirloin pancetta pork belly beef ribs. Cow landjaeger.
	`)
	require.NoError(t, err)
}

func TestParseCommentsErrMissingType(t *testing.T) {
	reset()
	_, err := Parse(`
		# The definition below should trigger an error
		# because we commented out its type.
		node: # bool .
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Missing Type")
}

func TestParseTypeComments(t *testing.T) {
	reset()
	_, err := Parse(`
		# User is a service user
		type User {
			# TODO: add more fields
			name # e.g., srfrog
									 # expanded comment
									 # embedded # comments # here
		}
		# /User
	`)
	require.NoError(t, err)
}

func TestParseWithNamespace(t *testing.T) {
	reset()
	result, err := Parse(`
		[10] jobs: [string] @index(term) .
		[0x123] occupations: [string] .
		[0xf2] graduation: [dateTime] .
		[0xf2] type Person {
			name: [string!]!
			address: string!
			children: [Person]
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 3, len(result.Preds))
	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: x.NamespaceAttr(10, "jobs"),
		ValueType: 9,
		Directive: pb.SchemaUpdate_INDEX,
		Tokenizer: []string{"term"},
		List:      true,
	}, result.Preds[0])

	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: x.NamespaceAttr(0x123, "occupations"),
		ValueType: 9,
		List:      true,
	}, result.Preds[1])

	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: x.NamespaceAttr(0xf2, "graduation"),
		ValueType: 5,
		List:      true,
	}, result.Preds[2])

	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: x.NamespaceAttr(0xf2, "Person"),
		Fields: []*pb.SchemaUpdate{
			{
				Predicate: x.NamespaceAttr(0xf2, "name"),
			},
			{
				Predicate: x.NamespaceAttr(0xf2, "address"),
			},
			{
				Predicate: x.NamespaceAttr(0xf2, "children"),
			},
		},
	}, result.Types[0])
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
