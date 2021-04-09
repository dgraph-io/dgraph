/*
 * Copyright 2015-2018 Dgraph Labs, Inc. and Contributors
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

package chunker

import (
	"testing"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/stretchr/testify/require"
)

var testNQuads = []struct {
	input        string
	nq           api.NQuad
	expectedErr  bool
	shouldIgnore bool
}{
	{
		input: `<some_subject_id> <predicate> <object_id> .`,
		nq: api.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: "<some_subject_id>\t<predicate>\t<object_id>\t.",
		nq: api.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <predicate> <object_id> .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `<0x01> <predicate> <object_id> .`,
		nq: api.NQuad{
			Subject:     "0x01",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `<some_subject_id> <predicate> <0x01> .`,
		nq: api.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "0x01",
			ObjectValue: nil,
		},
	},
	{
		input: `<0x01> <predicate> <0x02> .`,
		nq: api.NQuad{
			Subject:     "0x01",
			Predicate:   "predicate",
			ObjectId:    "0x02",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <follows> _:bob0 .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "follows",
			ObjectId:    "_:bob0",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland" .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Alice In Wonderland"}},
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"@en-0 .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			Lang:        "en-0",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Alice In Wonderland"}},
		},
	},
	{
		input: `_:alice <name@en> "Alice In Wonderland" .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			Lang:        "en",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "Alice In Wonderland"}},
		},
	},
	{
		input: `_:alice <name@en> * .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			Lang:        "en",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "_STAR_ALL"}},
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"^^<xs:string> .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "Alice In Wonderland"}},
		},
	},
	{
		input: `_:alice <age> "013"^^<xs:int> .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "age",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_IntVal{IntVal: 13}},
		},
	},
	{
		input: `_:alice <age> "013"^^<xs:integer> .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "age",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_IntVal{IntVal: 13}},
		},
	},
	{
		input: `_:alice <secret> "password1"^^<xs:password> .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "secret",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_PasswordVal{PasswordVal: "password1"}},
		},
	},
	{
		input: `<http://www.w3.org/2001/sw/RDFCore/nedges/> <http://purl.org/dc/terms/title> "N-Edges"@en-US .`,
		nq: api.NQuad{
			Subject:     "http://www.w3.org/2001/sw/RDFCore/nedges/",
			Predicate:   "http://purl.org/dc/terms/title",
			ObjectId:    "",
			Lang:        "en-US",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "N-Edges"}},
		},
	},
	{
		input: `_:art <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://xmlns.com/foaf/0.1/Person> .`,
		nq: api.NQuad{
			Subject:     "_:art",
			Predicate:   "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
			ObjectId:    "http://xmlns.com/foaf/0.1/Person",
			ObjectValue: nil,
		},
	},
	{
		input:       "_:alice .",
		expectedErr: true,
	},
	{
		input:       "_:alice knows .",
		expectedErr: true,
	},
	{
		input: "<_:alice> <knows> <something> .",
		nq: api.NQuad{
			Subject:   "_:alice",
			Predicate: "knows",
			ObjectId:  "something",
		},
		expectedErr: false,
	},
	{
		input: "_:alice <knows> <_:something> .",
		nq: api.NQuad{
			Subject:   "_:alice",
			Predicate: "knows",
			ObjectId:  "_:something",
		},
		expectedErr: false,
	},
	{
		input: `<alice> <knows> * .`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		},
		expectedErr: false,
	},
	{
		input: `<alice> * * .`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   x.Star,
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		},
		expectedErr: false,
	},
	{
		input:       "<alice> <knows> .",
		expectedErr: true,
	},
	{
		input:       " 0x01 <knows> <something> .",
		expectedErr: true,
	},
	{
		input:       "<alice> <knows>  0x01 .",
		expectedErr: true,
	},
	{
		input:       `_:alice "knows" stuff .`,
		expectedErr: true,
	},
	{
		input:       "_:alice <knows> stuff .",
		expectedErr: true,
	},
	{
		input:       "_:alice <knows> <stuff>",
		expectedErr: true,
	},
	{
		input:       `"_:alice" <knows> <stuff> .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"@-en .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"^<string> .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"^^xs:string .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <age> "thirteen"^^<xs:int> .`,
		expectedErr: true,
	},
	{
		input:       `<alice> <knows> <*> .`,
		expectedErr: true,
	},
	{
		input:       `<*> <knows> "stuff" .`,
		expectedErr: true,
	},
	{
		input:       `<alice> <*> "stuff" .`,
		expectedErr: true,
	},
	{
		input:       `<alice> < * > "stuff" .`,
		expectedErr: true,
	},
	{
		input:       `<alice> <* *> "stuff" .`,
		expectedErr: true,
	},
	{
		input:       `<alice> <*> "stuff" .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"^^< * > .`,
		expectedErr: true,
	},
	{
		input: `_:alice <knows> "" .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: ""}},
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> ""^^<xs:string> .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: ""}},
		},
		expectedErr: false,
	},
	{
		input:       `_:alice <knows> ""^^<xs:int> .`,
		expectedErr: true,
	},
	{
		input: `<alice> <knows> "*" .`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "*"}},
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> <0xf2> .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "stuff"}},
			Namespace:   0xf2,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> <0xf2> .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "stuff"}},
			Namespace:   0xf2,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> <10> . # comment`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_StrVal{StrVal: "stuff"}},
			Namespace:   10,
		},
		expectedErr: false,
	},
	{
		input:       `_:alice <knows> "stuff"^^<xs:string> "0xf2" .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"^^<xs:string> 0x01 .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"^^<xs:string> <quad> <pentagon> .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"^^<xs:string> quad .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"^^<xs:string> <*> .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> <bob> . <bob>`, // throws error because of <bob> after dot.
		expectedErr: true,
	},
	{
		input: `_:alice <likes> "mov\"enpick" .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "likes",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: `mov"enpick`}},
		},
	},
	{
		input: `<\u0021> <\U123abcdE> <\u0024> .`,
		nq: api.NQuad{
			Subject:   `\u0021`,
			Predicate: `\U123abcdE`,
			ObjectId:  `\u0024`,
		},
	},
	{
		input:       `<\u0021> <\U123abcdg> <\u0024> .`,
		expectedErr: true, // `g` is not a Hex char
	},
	{
		input:       `<messi with space> <friend> <ronaldo> .`,
		expectedErr: true, // should fail because of spaces in subject
	},
	{
		input:       `<with<> <with> <with> .`,
		expectedErr: true, // should fail because of < after with in subject
	},
	{
		input:       `<wi>th> <with> <with> .`,
		expectedErr: true, // should fail
	},
	{
		input:       `<"with> <with> <with> .`,
		expectedErr: true, // should fail because of "
	},
	{
		input:       `<{with> <with> <with> .`,
		expectedErr: true, // should fail because of {
	},
	{
		input:       `<wi{th> <with> <with> .`,
		expectedErr: true, // should fail because of }
	},
	{
		input:       `<with|> <with> <with> .`,
		expectedErr: true, // should fail because of |
	},
	{
		input:       `<wit^h> <with> <with> .`,
		expectedErr: true, // should fail because of ^
	},
	{
		input:       "<w`ith> <with> <with> .",
		expectedErr: true, // should fail because of `
	},
	{
		input:       `<wi\th> <with> <with> .`,
		expectedErr: true, // should fail because of \
	},
	{
		input:       `_:|alice <abc> <abc> .`,
		expectedErr: true, // | is not allowed first char in blanknode.
	},
	{
		input:       "_:al\u00d7ice <abc> <abc> .",
		expectedErr: true, // 0xd7 is not allowed
	},
	{
		input:       `_:gabe <name> "Gabe' .`,
		expectedErr: true,
	},
	{
		input:       `_:gabe <name> "Gabe'^^<xs:yo> .`,
		expectedErr: true,
	},
	{
		input: `_:0 <name> <good> .`,
		nq: api.NQuad{
			Subject:   "_:0",
			Predicate: "name",
			ObjectId:  "good",
		},
	},
	{
		input: `_:0a.b <name> <good> .`,
		nq: api.NQuad{
			Subject:   "_:0a.b",
			Predicate: "name",
			ObjectId:  "good",
		},
	},
	{
		input:       `_:0a. <name> <bad> .`,
		expectedErr: true, // blanknode can not end with .
	},
	{
		input:       `<alice> <lives> "wonder \a land" .`,
		expectedErr: true, // \a not valid escape char.
	},
	{
		input: `<alice> <lives> "\u0045 wonderland" .`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: `E wonderland`}},
		},
		expectedErr: false,
	},
	{
		input: `<alice> <lives> "wonderland" (friend="hatter").`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: `wonderland`}},
			Facets: []*api.Facet{{Key: "friend", Value: []byte("hatter"),
				Tokens: []string{"\001hatter"}}},
		},
		expectedErr: false,
	},
	{
		input: `<alice> <lives> "wonderland" (friend="hatter \u0045") .`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: `wonderland`}},
			Facets: []*api.Facet{{Key: "friend", Value: []byte("hatter E"),
				Tokens: []string{"\001e", "\001hatter"}}},
		},
		expectedErr: false,
	},
	{
		input:       `<alice> <lives> "\u004 wonderland" .`,
		expectedErr: true, // should have 4 hex values after \u
	},
	{
		input: `<alice> <lives> "\x02 wonderland" .`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "\x02 wonderland"}},
		},
		expectedErr: false,
	},
	{
		input:       `<alice> <lives> "\x2 wonderland" .`,
		expectedErr: true, // should have 2 hex values after \x
	},
	{
		input:       `<alice> <lives> "wonderful land"@a- .`,
		expectedErr: true, // object langtag can not end with -
	},
	{
		input: `<alice> <lives> "\v\t\b\n\r\f\"\\"@a-b .`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			Lang:        "a-b",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "\v\t\b\n\r\f\"\\"}},
		},
	},
	{
		input:       `<alice> <lives> "\'" .`,
		expectedErr: true, // \' isn't a valid escape sequence
	},
	{
		input:       `<alice> <lives> "\a" .`,
		expectedErr: true, // \a is not valid escape char
	},
	{
		input:        `# nothing happened`,
		expectedErr:  true,
		shouldIgnore: true,
	},
	{
		input:       `<some_subject_id> # <predicate> <object_id> .`,
		expectedErr: true,
	},
	{
		input:       `<some_subject_id> <predicate> <object_id> # .`,
		expectedErr: true,
	},
	{
		input:       `check me as error`,
		expectedErr: true,
	},
	{
		input:        `   `,
		expectedErr:  true,
		shouldIgnore: true,
	},

	// Edge Facets test.
	{
		input: `_:alice <knows> "stuff" <0x10> (key1="val1",key2=13) .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Namespace:   0x10,
			Facets: []*api.Facet{
				{
					Key:     "key1",
					Value:   []byte("val1"),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001val1"},
				},
				{
					Key:     "key2",
					Value:   []byte("\r\000\000\000\000\000\000\000"),
					ValType: facets.ValTypeForTypeID(facets.IntID),
					Tokens:  nil,
				}},
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff" <0x12> (key1=,key2=13) .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Namespace:   0x12,
			Facets: []*api.Facet{
				{
					Key:     "key1",
					Value:   []byte(""),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001"},
				},
				{
					Key:     "key2",
					Value:   []byte("\r\000\000\000\000\000\000\000"),
					ValType: facets.ValTypeForTypeID(facets.IntID),
					Tokens:  nil,
				}},
		},
		expectedErr: false,
	},
	// Should parse facets even if there is no label
	{
		input: `_:alice <knows> "stuff" (key1=,key2=13) .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Facets: []*api.Facet{
				{
					Key:     "key1",
					Value:   []byte(""),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001"},
				},
				{
					Key:     "key2",
					Value:   []byte("\r\000\000\000\000\000\000\000"),
					ValType: facets.ValTypeForTypeID(facets.IntID),
					Tokens:  nil,
				}},
		},
		expectedErr: false,
	},
	// Should not fail parsing with unnecessary spaces
	{
		input: `_:alice <knows> "stuff" ( key1 = 12 , key2="value2", key3=, key4 ="val4" ) .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Facets: []*api.Facet{
				{
					Key:     "key1",
					Value:   []byte("\014\000\000\000\000\000\000\000"),
					ValType: facets.ValTypeForTypeID(facets.IntID),
					Tokens:  nil,
				},

				{
					Key:     "key2",
					Value:   []byte("value2"),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001value2"},
				},
				{
					Key:     "key3",
					Value:   []byte(""),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001"},
				},
				{
					Key:     "key4",
					Value:   []byte("val4"),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001val4"},
				},
			},
		},
		expectedErr: false,
	},
	// Should parse all types
	{
		input: `_:alice <knows> "stuff" (key1=12,key2="value2",key3=1.2,key4=2006-01-02T15:04:05,key5=true,key6=false) .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Facets: []*api.Facet{
				{
					Key:     "key1",
					Value:   []byte("\014\000\000\000\000\000\000\000"),
					ValType: facets.ValTypeForTypeID(facets.IntID),
					Tokens:  nil,
				},
				{
					Key:     "key2",
					Value:   []byte("value2"),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001value2"},
				},
				{
					Key:     "key3",
					Value:   []byte("333333\363?"),
					ValType: facets.ValTypeForTypeID(facets.FloatID),
					Tokens:  nil,
				},
				{
					Key:     "key4",
					Value:   []byte("\001\000\000\000\016\273K7\345\000\000\000\000\377\377"),
					ValType: facets.ValTypeForTypeID(facets.DateTimeID),
					Tokens:  nil,
				},
				{
					Key:     "key5",
					Value:   []byte("\001"),
					ValType: facets.ValTypeForTypeID(facets.BoolID),
					Tokens:  nil,
				},
				{
					Key:     "key6",
					Value:   []byte("\000"),
					ValType: facets.ValTypeForTypeID(facets.BoolID),
					Tokens:  nil,
				},
			},
		},
		expectedErr: false,
	},
	// Should parse dates
	{
		input: `_:alice <knows> "stuff" (key1=2002-10-02T15:00:00.05Z, key2=2006-01-02T15:04:05, key3=2006-01-02T00:00:00Z) .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Facets: []*api.Facet{
				{
					Key:     "key1",
					Value:   []byte("\001\000\000\000\016\265-\000\360\002\372\360\200\377\377"),
					ValType: facets.ValTypeForTypeID(facets.DateTimeID),
					Tokens:  nil,
				},
				{
					Key:     "key2",
					Value:   []byte("\001\000\000\000\016\273K7\345\000\000\000\000\377\377"),
					ValType: facets.ValTypeForTypeID(facets.DateTimeID),
					Tokens:  nil,
				},
				{
					Key:     "key3",
					Value:   []byte("\001\000\000\000\016\273Jd\000\000\000\000\000\377\377"),
					ValType: facets.ValTypeForTypeID(facets.DateTimeID),
					Tokens:  nil,
				},
			},
		},
	},
	{
		// integer can be in any valid format.
		input: `_:alice <knows> "stuff" (k=0x0D) .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Facets: []*api.Facet{
				{
					Key:     "k",
					Value:   []byte("\r\000\000\000\000\000\000\000"),
					ValType: facets.ValTypeForTypeID(facets.IntID),
					Tokens:  nil,
				},
			},
		},
	},
	{
		// That what can not fit in integer fits in float.
		input: `_:alice <knows> "stuff" (k=111111111111111111888888.23) .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Facets: []*api.Facet{
				{
					Key:     "k",
					Value:   []byte("\240\250OlX\207\267D"),
					ValType: facets.ValTypeForTypeID(facets.FloatID),
					Tokens:  nil,
				},
			},
		},
	},
	{
		// Quotes inside facet string values.
		input: `_:alice <knows> "stuff" (key1="\"hello world\"",key2="LineA\nLineB") .`,
		nq: api.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "stuff"}},
			Facets: []*api.Facet{
				{
					Key:     "key1",
					Value:   []byte(`"hello world"`),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001hello", "\001world"},
				},
				{
					Key:     "key2",
					Value:   []byte("LineA\nLineB"),
					ValType: facets.ValTypeForTypeID(facets.StringID),
					Tokens:  []string{"\001linea", "\001lineb"},
				},
			},
		},
	},
	// failing tests for facets
	{
		input:       `_:alice <knows> "stuff" (key1="val1",key2) .`,
		expectedErr: true, // should fail because of no '=' after key2
	},
	{
		input:       `_:alice <knows> "stuff" (key1="val1",=) .`,
		expectedErr: true, // key can not be empty
	},
	{
		input:       `_:alice <knows> "stuff" (key1="val1",="val1") .`,
		expectedErr: true, // key can not be empty
	},
	{
		input:       `_:alice <knows> "stuff" (key1="val1",key1 "val1") .`,
		expectedErr: true, // '=' should separate key and val
	},
	{
		input:       `_:alice <knows> "stuff" (key1="val1",key1= "val1" .`,
		expectedErr: true, // facets should end by ')'
	},
	{
		input:       `_:alice <knows> "stuff" (key1="val1",key1= .`,
		expectedErr: true, // facets should end by ')'
	},
	{
		input:       `_:alice <knows> "stuff" (key1="val1",key1=`,
		expectedErr: true, // facets should end by ')'
	},
	{
		input:       `_:alice <knows> "stuff" (k==)`,
		expectedErr: true, // equal not allowed in value
	},
	{
		input:       `_:alice <knows> "stuff" (k=,) .`,
		expectedErr: true, // comma should be followed by another key-value pair.
	},
	{
		input:       `_:alice <knows> "stuff" (k=111111111111111111888888) .`,
		expectedErr: true, // integer can not fit in int64.
	},
	{
		input:       `_:alice <knows> "stuff" (k=0x1787586C4FA8A0284FF8) .`,
		expectedErr: true, // integer can not fit in int32 and also does not become float.
	},
	// Facet tests end
	{
		input:       `<alice> <password> "guess123"^^<pwd:password> .`,
		expectedErr: true,
	},
	{
		input: `* <pred> * .`,
		nq: api.NQuad{
			Subject:     x.Star,
			Predicate:   "pred",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		},
	},
	{
		input:       `* <pred> "random"^^<int> .`,
		expectedErr: true,
	},
	{
		input:       `_:company <name> "TurfBytes" . _:company <owner> _:owner . _:owner <name> "Jason" .  `,
		expectedErr: true,
	},
	{
		input: `<alice> <lives> "A\tB" .`,
		nq: api.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "A\tB"}},
		},
	},
	{
		input:       `<alice> <age> "NaN"^^<xs:double> .`,
		expectedErr: true,
	},
	{
		input:       `<alice> <age> "13"^^<xs:double> (salary=NaN) .`,
		expectedErr: true,
	},
	{
		input: `uid(v) <lives> "\x02 wonderland" .`,
		nq: api.NQuad{
			Subject:     "uid(v)",
			Predicate:   "lives",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "\x02 wonderland"}},
		},
		expectedErr: false,
	},
	{
		input: `uid  (  v  ) <lives> "vrinadavan" .`,
		nq: api.NQuad{
			Subject:     "uid(v)",
			Predicate:   "lives",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "vrinadavan"}},
		},
		expectedErr: false,
	},
	{
		input: `uid  (  val  ) <lives> "vrinadavan" .`,
		nq: api.NQuad{
			Subject:     "uid(val)",
			Predicate:   "lives",
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: "vrinadavan"}},
		},
		expectedErr: false,
	},
	{
		input: `uid  (  val  ) <lives> uid(g) .`,
		nq: api.NQuad{
			Subject:   "uid(val)",
			Predicate: "lives",
			ObjectId:  "uid(g)",
		},
		expectedErr: false,
	},
	{
		input: `uid  (  val  ) <lives> uid ( g )  .`,
		nq: api.NQuad{
			Subject:   "uid(val)",
			Predicate: "lives",
			ObjectId:  "uid(g)",
		},
		expectedErr: false,
	},
	{
		input:       `uid  (  val   <lives> uid ( g )  .`,
		expectedErr: true,
	},
	{
		input:       `uid   val )   <lives> uid ( g )  .`,
		expectedErr: true,
	},
	{
		input:       `ui(uid)   <lives> uid ( g )  .`,
		expectedErr: true,
	},
	{
		input:       `uid())   <lives> uid ( g )  .`,
		expectedErr: true,
	},
	{
		input:       `uid()   <lives> uid ( g )  .`,
		expectedErr: true,
	},
	{
		input:       `uid(a)   <lives> uid (  )  .`,
		expectedErr: true,
	},
	{
		input:       `uid(a)   lives> uid (  )  .`,
		expectedErr: true,
	},
}

func TestLex(t *testing.T) {
	l := &lex.Lexer{}
	for _, test := range testNQuads {
		l.Reset(test.input)
		rnq, err := ParseRDF(test.input, l)
		switch {
		case test.expectedErr && test.shouldIgnore:
			require.Equal(t, ErrEmpty, err, "Catch an ignorable case: %v",
				err.Error())
		case test.expectedErr:
			require.Error(t, err, "Expected error for input: %q. Output: %+v",
				test.input, rnq)
		default:
			require.NoError(t, err, "Got error for input: %q", test.input)
			require.Equal(t, test.nq, rnq, "Mismatch for input: %q", test.input)
		}
	}
}
