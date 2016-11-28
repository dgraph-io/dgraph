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

	"github.com/stretchr/testify/assert"
)

var testNQuads = []struct {
	input       string
	nq          NQuad
	expectedErr bool
}{
	{
		input: `<some_subject_id> <predicate> <object_id> .`,
		nq: NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: []byte(nil),
		},
	},
	{
		input: "<some_subject_id>\t<predicate>\t<object_id>\t.",
		nq: NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: []byte(nil),
		},
	},
	{
		input: `_:alice <predicate> <object_id> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: []byte(nil),
		},
	},
	{
		input: `_uid_:0x01 <predicate> <object_id> .`,
		nq: NQuad{
			Subject:     "_uid_:0x01",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: []byte(nil),
		},
	},
	{
		input: `<some_subject_id> <predicate> _uid_:0x01 .`,
		nq: NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "_uid_:0x01",
			ObjectValue: []byte(nil),
		},
	},
	{
		input: `_uid_:0x01 <predicate> _uid_:0x02 .`,
		nq: NQuad{
			Subject:     "_uid_:0x01",
			Predicate:   "predicate",
			ObjectId:    "_uid_:0x02",
			ObjectValue: []byte(nil),
		},
	},
	{
		input: `_:alice <follows> _:bob0 .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "follows",
			ObjectId:    "_:bob0",
			ObjectValue: []byte(nil),
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland" .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: []byte("Alice In Wonderland"),
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"@en-0 .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "name.en-0",
			ObjectId:    "",
			ObjectValue: []byte("Alice In Wonderland"),
		},
	},
	{
		input: `_:alice <age> "013"^^<xs:int> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "age",
			ObjectId:    "",
			ObjectValue: []byte{13, 0, 0, 0},
			ObjectType:  1,
		},
	},
	{
		input: `<http://www.w3.org/2001/sw/RDFCore/nedges/> <http://purl.org/dc/terms/title> "N-Edges"@en-US .`,
		nq: NQuad{
			Subject:     "http://www.w3.org/2001/sw/RDFCore/nedges/",
			Predicate:   "http://purl.org/dc/terms/title.en-US",
			ObjectId:    "",
			ObjectValue: []byte("N-Edges"),
		},
	},
	{
		input: `_:art <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://xmlns.com/foaf/0.1/Person> .`,
		nq: NQuad{
			Subject:     "_:art",
			Predicate:   "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
			ObjectId:    "http://xmlns.com/foaf/0.1/Person",
			ObjectValue: []byte(nil),
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
		input:       "<alice> <knows> .",
		expectedErr: true,
	},
	{
		input:       "_uid_: 0x01 <knows> <something> .",
		expectedErr: true,
	},
	{
		input:       "<alice> <knows> _uid_: 0x01 .",
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
		input: `_:alice <knows> "stuff"^^<xs:string> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("stuff"),
			ObjectType:  5,
		},
		expectedErr: false,
	},
	{
		input: `<alice> <knows> "*" .`,
		nq: NQuad{
			Subject:     "alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("*"),
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> <label> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("stuff"),
			Label:       "label",
			ObjectType:  5,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> _:label .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("stuff"),
			Label:       "_:label",
			ObjectType:  5,
		},
		expectedErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> _:label . # comment`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("stuff"),
			Label:       "_:label",
			ObjectType:  5,
		},
		expectedErr: false,
	},
	{
		input:       `_:alice <knows> "stuff"^^<xs:string> "label" .`,
		expectedErr: true,
	},
	{
		input:       `_:alice <knows> "stuff"^^<xs:string> _uid_:0x01 .`,
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
		nq: NQuad{
			Subject:   "_:alice",
			Predicate: "knows",
			ObjectId:  "bob",
		},
	},
	{
		input: `_:alice <likes> "mov\"enpick" .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "likes",
			ObjectValue: []byte(`mov\"enpick`),
		},
	},
}

func TestLex(t *testing.T) {
	for _, test := range testNQuads {
		rnq, err := Parse(test.input)
		if test.expectedErr {
			assert.Error(t, err, "Expected error for input: %q. Output: %+v",
				test.input, rnq)
		} else {
			assert.NoError(t, err, "Got error for input: %q", test.input)
			assert.Equal(t, test.nq, rnq, "Mismatch for input: %q", test.input)
		}
	}
}
