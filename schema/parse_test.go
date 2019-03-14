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
			List:      true,
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
	result, err := Parse("")
	require.NoError(t, err)
	require.Nil(t, result.Schemas)
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

func TestParseScalarList(t *testing.T) {
	reset()
	result, err := Parse(`
		jobs: [string] @index(term) .
		occupations: [string] .
		graduation: [dateTime] .
	`)
	require.NoError(t, err)
	require.Equal(t, 3, len(result.Schemas))
	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: "jobs",
		ValueType: 9,
		Directive: pb.SchemaUpdate_INDEX,
		Tokenizer: []string{"term"},
		List:      true,
	}, result.Schemas[0])

	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: "occupations",
		ValueType: 9,
		List:      true,
	}, result.Schemas[1])

	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: "graduation",
		ValueType: 5,
		List:      true,
	}, result.Schemas[2])
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
	require.Equal(t, 1, len(result.Schemas))
	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: "friend",
		ValueType: 7,
		List:      true,
	}, result.Schemas[0])
}

func TestParseUidSingleValue(t *testing.T) {
	reset()
	result, err := Parse(`
		friend: uid .
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Schemas))
	require.EqualValues(t, &pb.SchemaUpdate{
		Predicate: "friend",
		ValueType: 7,
		List:      false,
	}, result.Schemas[0])
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
		TypeName: "Person",
	}, result.Types[0])

}

func TestParseSingleType(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Name: string
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate: "Name",
				ValueType: pb.Posting_STRING,
			},
		},
	}, result.Types[0])
}

func TestParseBaseTypesCaseInsensitive(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Name: string
			LastName: String
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate: "Name",
				ValueType: pb.Posting_STRING,
			},
			&pb.SchemaUpdate{
				Predicate: "LastName",
				ValueType: pb.Posting_STRING,
			},
		},
	}, result.Types[0])
}

func TestParseCombinedSchemasAndTypes(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {

		}
        name: string .
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Schemas))
	require.Equal(t, &pb.SchemaUpdate{
		Predicate: "name",
		ValueType: 9,
	}, result.Schemas[0])
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
	}, result.Types[0])
}

func TestParseMultipleTypes(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Name: string
		}
		type Animal {
			Name: string
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 2, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate: "Name",
				ValueType: pb.Posting_STRING,
			},
		},
	}, result.Types[0])
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Animal",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate: "Name",
				ValueType: pb.Posting_STRING,
			},
		},
	}, result.Types[1])
}

func TestParseObjectType(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Father: Person
			Mother: Person
			Children: [Person]
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate:      "Father",
				ValueType:      pb.Posting_OBJECT,
				ObjectTypeName: "Person",
			},
			&pb.SchemaUpdate{
				Predicate:      "Mother",
				ValueType:      pb.Posting_OBJECT,
				ObjectTypeName: "Person",
			},
			&pb.SchemaUpdate{
				Predicate:      "Children",
				ValueType:      pb.Posting_OBJECT,
				ObjectTypeName: "Person",
				List:           true,
			},
		},
	}, result.Types[0])
}

func TestParseScalarType(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Name: string
			Nickname: [String]
			Alive: Bool 
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate: "Name",
				ValueType: pb.Posting_STRING,
			},
			&pb.SchemaUpdate{
				Predicate: "Nickname",
				ValueType: pb.Posting_STRING,
				List:      true,
			},
			&pb.SchemaUpdate{
				Predicate: "Alive",
				ValueType: pb.Posting_BOOL,
			},
		},
	}, result.Types[0])
}

func TestParseCombinedTypes(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Name: string
			Nickname: [string]
			Parents: [Person]
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate: "Name",
				ValueType: pb.Posting_STRING,
			},
			&pb.SchemaUpdate{
				Predicate: "Nickname",
				ValueType: pb.Posting_STRING,
				List:      true,
			},
			&pb.SchemaUpdate{
				Predicate:      "Parents",
				ValueType:      pb.Posting_OBJECT,
				ObjectTypeName: "Person",
				List:           true,
			},
		},
	}, result.Types[0])
}

func TestParseNonNullableScalar(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Name: string!
			Nickname: [string]
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate:   "Name",
				ValueType:   pb.Posting_STRING,
				NonNullable: true,
			},
			&pb.SchemaUpdate{
				Predicate: "Nickname",
				ValueType: pb.Posting_STRING,
				List:      true,
			},
		},
	}, result.Types[0])
}

func TestParseNonNullableList(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Name: string
			Nickname: [string]!
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate: "Name",
				ValueType: pb.Posting_STRING,
			},
			&pb.SchemaUpdate{
				Predicate:       "Nickname",
				ValueType:       pb.Posting_STRING,
				List:            true,
				NonNullableList: true,
			},
		},
	}, result.Types[0])
}

func TestParseNonNullableScalarAndList(t *testing.T) {
	reset()
	result, err := Parse(`
		type Person {
			Name: string!
			Nickname: [string!]!
		}
	`)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Types))
	require.Equal(t, &pb.TypeUpdate{
		TypeName: "Person",
		Fields: []*pb.SchemaUpdate{
			&pb.SchemaUpdate{
				Predicate:   "Name",
				ValueType:   pb.Posting_STRING,
				NonNullable: true,
			},
			&pb.SchemaUpdate{
				Predicate:       "Nickname",
				ValueType:       pb.Posting_STRING,
				List:            true,
				NonNullable:     true,
				NonNullableList: true,
			},
		},
	}, result.Types[0])
}

func TestParseTypeErrMissingNewLine(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
		}type Animal {}
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected new line after type declaration")
}

func TestParseTypeErrMissingColon(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
			Name string
		}
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Missing colon in type declaration")
}

func TestParseTypeErrMultipleTypes(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
			Name: bool string
		}
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected new line after field declaration")
}

func TestParseTypeErrMultipleExclamationMarks(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
			Name: bool!!
		}
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected new line after field declaration")
}

func TestParseTypeErrMissingSquareBracket(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
			Name: [string
		}
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Expected matching square bracket")
}

func TestParseTypeErrMultipleSquareBrackets(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
			Name: [[string]]
		}
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Missing field type in type declaration")
}

func TestParseTypeErrMissingType(t *testing.T) {
	reset()
	_, err := Parse(`
		type Person {
			Name:
		}
	`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Missing field type in type declaration")
}

var ps *badger.DB

func TestMain(m *testing.M) {
	x.Init()

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
