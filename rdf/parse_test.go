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

package rdf

import (
	"testing"

	"github.com/dgraph-io/dgraph/protos/facetsp"
	"github.com/dgraph-io/dgraph/protos/graphp"

	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/stretchr/testify/assert"
)

var testNQuads = []struct {
	input        string
	nq           graphp.NQuad
	expectedErr  bool
	shouldIgnore bool
}{
	{
		input: `<some_subject_id> <predicate> <object_id> .`,
		nq: graphp.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: "<some_subject_id>\t<predicate>\t<object_id>\t.",
		nq: graphp.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <predicate> <object_id> .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `<0x01> <predicate> <object_id> .`,
		nq: graphp.NQuad{
			Subject:     "0x01",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `<some_subject_id> <predicate> <0x01> .`,
		nq: graphp.NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "0x01",
			ObjectValue: nil,
		},
	},
	{
		input: `<0x01> <predicate> <0x02> .`,
		nq: graphp.NQuad{
			Subject:     "0x01",
			Predicate:   "predicate",
			ObjectId:    "0x02",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <follows> _:bob0 .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "follows",
			ObjectId:    "_:bob0",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland" .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"Alice In Wonderland"}},
			ObjectType:  0,
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"@en-0 .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			Lang:        "en-0",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"Alice In Wonderland"}},
			ObjectType:  10,
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"^^<xs:string> .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_StrVal{"Alice In Wonderland"}},
			ObjectType:  10,
		},
	},
	{
		input: `_:alice <age> "013"^^<xs:int> .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "age",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_IntVal{13}},
			ObjectType:  2,
		},
	},
	{
		input: `<http://www.w3.org/2001/sw/RDFCore/nedges/> <http://purl.org/dc/terms/title> "N-Edges"@en-US .`,
		nq: graphp.NQuad{
			Subject:     "http://www.w3.org/2001/sw/RDFCore/nedges/",
			Predicate:   "http://purl.org/dc/terms/title",
			ObjectId:    "",
			Lang:        "en-US",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"N-Edges"}},
			ObjectType:  10,
		},
	},
	{
		input: `_:art <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://xmlns.com/foaf/0.1/Person> .`,
		nq: graphp.NQuad{
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
		nq: graphp.NQuad{
			Subject:   "_:alice",
			Predicate: "knows",
			ObjectId:  "something",
		},
		expectedErr: false,
	},
	{
		input: "_:alice <knows> <_:something> .",
		nq: graphp.NQuad{
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
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"_nil_"}},
			ObjectType:  0,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> ""^^<xs:string> .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_StrVal{"_nil_"}},
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
		nq: graphp.NQuad{
			Subject:     "alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"*"}},
			ObjectType:  0,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> <label> .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_StrVal{"stuff"}},
			Label:       "label",
			ObjectType:  10,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> _:label .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_StrVal{"stuff"}},
			Label:       "_:label",
			ObjectType:  10,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> _:label . # comment`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_StrVal{"stuff"}},
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
		nq: graphp.NQuad{
			Subject:   "_:alice",
			Predicate: "knows",
			ObjectId:  "bob",
		},
	},
	{
		input: `_:alice <likes> "mov\"enpick" .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "likes",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{`mov\"enpick`}},
			ObjectType:  0,
		},
	},
	{
		input: `<\u0021> <\U123abcdE> <\u0024> .`,
		nq: graphp.NQuad{
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
		nq: graphp.NQuad{
			Subject:   "_:0",
			Predicate: "name",
			ObjectId:  "good",
		},
	},
	{
		input: `_:0a.b <name> <good> .`,
		nq: graphp.NQuad{
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
		nq: graphp.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{`\u0045 wonderland`}},
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
		nq: graphp.NQuad{
			Subject:     "alice",
			Predicate:   "lives",
			Lang:        "a-b",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{`\t\b\n\r\f\"\'\\`}},
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
		input: `_:alice <knows> "stuff" _:label (key1="val1",key2=13) .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			Label:       "_:label",
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"key1",
					[]byte("val1"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001val1"},
				},
				{"key2",
					[]byte("\r\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil,
				}},
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff" _:label (key1=,key2=13) .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			Label:       "_:label",
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"key1",
					[]byte(""),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{},
				},
				{"key2",
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
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"key1",
					[]byte(""),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{},
				},
				{"key2",
					[]byte("\r\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil,
				}},
		},
		expectedErr: false,
	},
	// Should not fail parsing with unnecessary spaces
	{
		input: `_:alice <knows> "stuff" ( key1 = 12 , key2="value2", key3=, key4 ="val4" ) .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"key1",
					[]byte("\014\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil},
				{"key2",
					[]byte("value2"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001value2"}},
				{"key3",
					[]byte(""),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{}},
				{"key4", []byte("val4"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001val4"}},
			},
		},
		expectedErr: false,
	},
	// Should parse all types
	{
		input: `_:alice <knows> "stuff" (key1=12,key2="value2",key3=1.2,key4=2006-01-02T15:04:05,key5=true,key6=false) .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"key1", []byte("\014\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil},
				{"key2", []byte("value2"),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001value2"}},
				{"key3", []byte("333333\363?"),
					facets.ValTypeForTypeID(facets.FloatID),
					nil},
				{"key4", []byte("\001\000\000\000\016\273K7\345\000\000\000\000\377\377"),
					facets.ValTypeForTypeID(facets.DateTimeID),
					nil},
				{"key5", []byte("\001"),
					facets.ValTypeForTypeID(facets.BoolID),
					nil},
				{"key6", []byte("\000"),
					facets.ValTypeForTypeID(facets.BoolID),
					nil},
			},
		},
		expectedErr: false,
	},
	// Should parse dates
	{
		input: `_:alice <knows> "stuff" (key1=2002-10-02T15:00:00.05Z, key2=2006-01-02T15:04:05, key3=2006-01-02) .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"key1", []byte("\001\000\000\000\016\265-\000\360\002\372\360\200\377\377"),
					facets.ValTypeForTypeID(facets.DateTimeID),
					nil},
				{"key2", []byte("\001\000\000\000\016\273K7\345\000\000\000\000\377\377"),
					facets.ValTypeForTypeID(facets.DateTimeID),
					nil},
				{"key3", []byte("\001\000\000\000\016\273Jd\000\000\000\000\000\377\377"),
					facets.ValTypeForTypeID(facets.DateTimeID),
					nil},
			},
		},
	},
	{
		// integer can be in any valid format.
		input: `_:alice <knows> "stuff" (k=0x0D) .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"k", []byte("\r\000\000\000"),
					facets.ValTypeForTypeID(facets.Int32ID),
					nil},
			},
		},
	},
	{
		// That what can not fit in integer fits in float.
		input: `_:alice <knows> "stuff" (k=111111111111111111888888.23) .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"k", []byte("\240\250OlX\207\267D"),
					facets.ValTypeForTypeID(facets.FloatID),
					nil},
			},
		},
	},
	{
		// Quotes inside facet string values.
		input: `_:alice <knows> "stuff" (key1="\"hello world\"") .`,
		nq: graphp.NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: &graphp.Value{&graphp.Value_DefaultVal{"stuff"}},
			ObjectType:  0,
			Facets: []*facetsp.Facet{
				{"key1", []byte(`\"hello world\"`),
					facets.ValTypeForTypeID(facets.StringID),
					[]string{"\001hello", "\001world"},
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
	{
		input:       `_:alice <knows> "stuff" (k=0x1787586C4FA8A0284FF8) .`,
		expectedErr: false, // integer can not fit in int32 and also does not become float.
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
