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
			ObjectValue: &graph.Value{&graph.Value_StrVal{"Alice In Wonderland"}},
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"@en-0 .`,
		nq: graph.NQuad{
			Subject:     "_:alice",
			Predicate:   "name.en-0",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_StrVal{"Alice In Wonderland"}},
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
			Predicate:   "http://purl.org/dc/terms/title.en-US",
			ObjectId:    "",
			ObjectValue: &graph.Value{&graph.Value_StrVal{"N-Edges"}},
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
		input:       "<_:alice> <knows> <something> .",
		expectedErr: true,
	},
	{
		input:       "_:alice <knows> <_:something> .",
		expectedErr: true,
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
			ObjectValue: &graph.Value{&graph.Value_StrVal{"_nil_"}},
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
			ObjectType:  0,
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
			ObjectValue: &graph.Value{&graph.Value_StrVal{"*"}},
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
			ObjectType:  0,
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
			ObjectType:  0,
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
			ObjectType:  0,
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
			ObjectValue: &graph.Value{&graph.Value_StrVal{`mov\"enpick`}},
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
			ObjectValue: &graph.Value{&graph.Value_StrVal{`\u0045 wonderland`}},
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
			Predicate:   "lives.a-b",
			ObjectValue: &graph.Value{&graph.Value_StrVal{`\t\b\n\r\f\"\'\\`}},
		},
	},
	{
		input:       `<alice> <lives> "\a" .`,
		expectedErr: true, // \a is not valid escape char
  },
  {
		input: `# nothing happened`,
		expectedErr: true,
		shouldIgnore: true,
	},
	{
		input: `<some_subject_id> # <predicate> <object_id> .`,
		expectedErr: true,
	},
	{
		input: `<some_subject_id> <predicate> <object_id> # .`,
		expectedErr: true,
	},
	{
		input: `check me as error`,
		expectedErr: true,
	},
	{
		input: `   `,
		expectedErr: true,
		shouldIgnore: true,
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
