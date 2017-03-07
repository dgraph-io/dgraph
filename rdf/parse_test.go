/*
 * Copyright 2015 DGraph Labs, Inc.
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

package rdf

import (
	"testing"

	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/stretchr/testify/assert"
)

var testNQuads = []struct {
	input        string
	nq           graph.NQuad
	expectedErr  bool
	shouldIgnore bool
}{
	{
		input: `<some_subject_id> <predicate> <object_id> .`,
		nq: graph.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: "<some_subject_id>\t<predicate>\t<object_id>\t.",
		nq: graph.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <predicate> <object_id> .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `<0x01> <predicate> <object_id> .`,
		nq: graph.NQuad{
			Subject:     "0x01",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `<some_subject_id> <predicate> <0x01> .`,
		nq: graph.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "0x01",
			ObjectValue: nil,
		},
	},
	{
		input: `<0x01> <predicate> <0x02> .`,
		nq: graph.NQuad{
			Subject:     "0x01",
			Predicate:   "predicate",
			ObjectId:    "0x02",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <follows> _:bob0 .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "follows",
			ObjectId:    "_:bob0",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland" .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"Alice In Wonderland"}},
			ObjectType:  0,
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"@en-0 .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			Lang:        "en-0",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"Alice In Wonderland"}},
			ObjectType:  10,
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"^^<xs:string> .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_StrVal{"Alice In Wonderland"}},
			ObjectType:  10,
		},
	},
	{
		input: `_:alice <age> "013"^^<xs:int> .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "age",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_IntVal{13}},
			ObjectType:  2,
		},
	},
	{
		input: `<http://www.w3.org/2001/sw/RDFCore/nedges/> <http://purl.org/dc/terms/title> "N-Edges"@en-US .`,
		nq: graph.NQuad{
			Subject:     "http://www.w3.org/2001/sw/RDFCore/nedges/",
			Predicate:   "http://purl.org/dc/terms/title",
			ObjectId:    "",
			Lang:        "en-US",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"N-Edges"}},
			ObjectType:  10,
		},
	},
	{
		input: `_:art <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://xmlns.com/foaf/0.1/Person> .`,
		nq: graph.NQuad{
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
		nq: graph.NQuad{
			Subject:   "_:alice",
			Predicate: "knows",
			ObjectId:  "something",
		},
		expectedErr: false,
	},
	{
		input: "_:alice <knows> <_:something> .",
		nq: graph.NQuad{
			Subject:   "_:alice",
			Predicate: "knows",
			ObjectId:  "_:something",
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
		input:       `<alice> <knows> * .`,
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
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"_nil_"}},
			ObjectType:  0,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> ""^^<xs:string> .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_StrVal{"_nil_"}},
			ObjectType:  10,
		},
		expectedErr: false,
	},
	{
		input:       `_:alice <knows> ""^^<xs:int> .`,
		expectedErr: true,
	},
	{
		input: `<alice> <knows> "*" .`,
		nq: graph.NQuad{
			Subject:     "alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"*"}},
			ObjectType:  0,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> <label> .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_StrVal{"stuff"}},
			Label:       "label",
			ObjectType:  10,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> _:label .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_StrVal{"stuff"}},
			Label:       "_:label",
			ObjectType:  10,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> _:label . # comment`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_StrVal{"stuff"}},
			Label:       "_:label",
			ObjectType:  10,
		},
		expectedErr: false,
	},
	{
		input:       `_:alice <knows> "stuff"^^<xs:string> "label" .`,
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
		input: `_:alice <knows> <bob> . <bob>`, // ignores the <bob> after dot.
		nq: graph.NQuad{
			Subject:   "_:alice",
			Predicate: "knows",
			ObjectId:  "bob",
		},
	},
	{
		input: `_:alice <likes> "mov\"enpick" .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "likes",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{`mov\"enpick`}},
			ObjectType:  0,
		},
	},
	{
		input: `<\u0021> <\U123abcdE> <\u0024> .`,
		nq: graph.NQuad{
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
		input: `_:0 <name> <good> .`,
		nq: graph.NQuad{
			Subject:   "_:0",
			Predicate: "name",
			ObjectId:  "good",
		},
	},
	{
		input: `_:0a.b <name> <good> .`,
		nq: graph.NQuad{
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
		nq: graph.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{`\u0045 wonderland`}},
			ObjectType:  0,
		},
		expectedErr: false,
	},
	{
		input:       `<alice> <lives> "\u004 wonderland" .`,
		expectedErr: true, // should have 4 hex values after \u
	},
	{
		input:       `<alice> <lives> "wonderful land"@a- .`,
		expectedErr: true, // object langtag can not end with -
	},
	{
		input: `<alice> <lives> "\t\b\n\r\f\"\'\\"@a-b .`,
		nq: graph.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			Lang:        "a-b",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{`\t\b\n\r\f\"\'\\`}},
			ObjectType:  10,
		},
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
		input: `_:alice <knows> "stuff" _:label (key1=val1,key2=13) .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"stuff"}},
			Label:       "_:label",
			ObjectType:  0,
			Facets: []*facets.Facet{
				&facets.Facet{"key1",
					[]byte("val1"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001val1"},
				},
				&facets.Facet{"key2",
					[]byte("\r\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil,
				}},
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff" _:label (key1=,key2=13) .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"stuff"}},
			Label:       "_:label",
			ObjectType:  0,
			Facets: []*facets.Facet{
				&facets.Facet{"key1",
					[]byte(""),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{},
				},
				&facets.Facet{"key2",
					[]byte("\r\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil,
				}},
		},
		expectedErr: false,
	},
	// Should parse facets even if there is no label
	{
		input: `_:alice <knows> "stuff" (key1=,key2=13) .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facets.Facet{
				&facets.Facet{"key1",
					[]byte(""),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{},
				},
				&facets.Facet{"key2",
					[]byte("\r\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil,
				}},
		},
		expectedErr: false,
	},
	// Should not fail parsing with unnecessary spaces
	{
		input: `_:alice <knows> "stuff" ( key1 = 12 , key2=value2, key3=, key4 =val4 ) .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facets.Facet{
				&facets.Facet{"key1",
					[]byte("\014\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil},
				&facets.Facet{"key2",
					[]byte("value2"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001value2"}},
				&facets.Facet{"key3",
					[]byte(""),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{}},
				&facets.Facet{"key4", []byte("val4"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001val4"}},
			},
		},
		expectedErr: false,
	},
	// Should parse all types
	{
		input: `_:alice <knows> "stuff" (key1=12,key2=value2,key3=1.2,key4=2006-01-02T15:04:05,key5=true,key6=false) .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facets.Facet{
				&facets.Facet{"key1", []byte("\014\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil},
				&facets.Facet{"key2", []byte("value2"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001value2"}},
				&facets.Facet{"key3", []byte("333333\363?"),
					facets.ValTypeForTypeID(facets.FloatID),
					nil},
				&facets.Facet{"key4", []byte("\001\000\000\000\016\273K7\345\000\000\000\000\377\377"),
					facets.ValTypeForTypeID(facets.DateTimeID),
					nil},
				&facets.Facet{"key5", []byte("\001"),
					facets.ValTypeForTypeID(facets.BoolID),
					nil},
				&facets.Facet{"key6", []byte("\000"),
					facets.ValTypeForTypeID(facets.BoolID),
					nil},
			},
		},
		expectedErr: false,
	},
	// Should parse bools for only "true" and "false"
	{
		// True, 1 , t are some valid true values in go strconv.ParseBool
		input: `_:alice <knows> "stuff" (key1=true,key2=false,key3=True,key4=False,key5=1, key6=t) .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facets.Facet{
				&facets.Facet{"key1", []byte("\001"),
					facets.ValTypeForTypeID(facets.BoolID),
					nil},
				&facets.Facet{"key2", []byte("\000"),
					facets.ValTypeForTypeID(facets.BoolID),
					nil},
				&facets.Facet{"key3", []byte("True"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001true"}},
				&facets.Facet{"key4", []byte("False"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001false"}},
				&facets.Facet{"key5", []byte("\001\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil},
				&facets.Facet{"key6", []byte("t"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001t"}},
			},
		},
		expectedErr: false,
	},
	// Should parse to string even if value start with ints if it has alphabets.
	// Only support decimal format for ints.
	{
		input: `_:alice <knows> "stuff" (key1=11adsf234,key2=11111111111111111111132333uasfk333) .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facets.Facet{
				&facets.Facet{"key1", []byte("11adsf234"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\00111adsf234"}},
				&facets.Facet{"key2", []byte("11111111111111111111132333uasfk333"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\00111111111111111111111132333uasfk333"}},
			},
		},
	},

	// Should parse dates
	{
		input: `_:alice <knows> "stuff" (key1=2002-10-02T15:00:00.05Z, key2=2006-01-02T15:04:05, key3=2006-01-02) .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facets.Facet{
				&facets.Facet{"key1", []byte("\001\000\000\000\016\265-\000\360\002\372\360\200\377\377"),
					facets.ValTypeForTypeID(facets.DateTimeID),
					nil},
				&facets.Facet{"key2", []byte("\001\000\000\000\016\273K7\345\000\000\000\000\377\377"),
					facets.ValTypeForTypeID(facets.DateTimeID),
					nil},
				&facets.Facet{"key3", []byte("\001\000\000\000\016\273Jd\000\000\000\000\000\377\377"),
					facets.ValTypeForTypeID(facets.DateTimeID),
					nil},
			},
		},
	},
	// failing tests for facets
	{
		input:       `_:alice <knows> "stuff" (key1=val1,key2) .`,
		expectedErr: true, // should fail because of no '=' after key2
	},
	{
		input:       `_:alice <knows> "stuff" (key1=val1,=) .`,
		expectedErr: true, // key can not be empty
	},
	{
		input:       `_:alice <knows> "stuff" (key1=val1,=val1) .`,
		expectedErr: true, // key can not be empty
	},
	{
		input:       `_:alice <knows> "stuff" (key1=val1,key1 val1) .`,
		expectedErr: true, // '=' should separate key and val
	},
	{
		input:       `_:alice <knows> "stuff" (key1=val1,key1= val1 .`,
		expectedErr: true, // facets should end by ')'
	},
	{
		input:       `_:alice <knows> "stuff" (key1=val1,key1= .`,
		expectedErr: true, // facets should end by ')'
	},
	{
		input:       `_:alice <knows> "stuff" (key1=val1,key1=`,
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
		input:       `_:alice <knows> "stuff" (k=1,k=2) .`,
		expectedErr: true, // Duplicate keys not allowed.
	},
	{
		input:       `_:alice <knows> "stuff" (k=1,k1=1,k=2) .`,
		expectedErr: true, // Duplicate keys not allowed.
	},
	{
		input:       `_:alice <knows> "stuff" (k=1,k1=,k=2) .`,
		expectedErr: true, // Duplicate keys not allowed.
	},
	{
		input:       `_:alice <knows> "stuff" (k=111111111111111111888888) .`,
		expectedErr: true, // integer can not fit in int32.
	},
	// Facet tests end
	{
		input:       `<alice> <password> "guess"^^<pwd:password> .`,
		expectedErr: true, // len(password) should >= 6
	},
}

func TestLex(t *testing.T) {
	for _, test := range testNQuads {
		t.Logf("Testing %v", test.input)
		rnq, err := Parse(test.input)
		if test.expectedErr && test.shouldIgnore {
			assert.Equal(t, ErrEmpty, err, "Catch an ignorable case: %v",
				err.Error())
		} else if test.expectedErr {
			assert.Error(t, err, "Expected error for input: %q. Output: %+v",
				test.input, rnq)
		} else {
			assert.NoError(t, err, "Got error for input: %q", test.input)
			assert.Equal(t, test.nq, rnq, "Mismatch for input: %q", test.input)
		}
	}
}
