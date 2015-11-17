/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
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
	"reflect"
	"testing"
)

var testNQuads = []struct {
	input  string
	nq     NQuad
	hasErr bool
}{
	{
		input: `<some_subject_id> <predicate> <object_id> .`,
		nq: NQuad{
			Subject:     "some_subject_id",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <predicate> <object_id> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "predicate",
			ObjectId:    "object_id",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <follows> _:bob0 .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "follows",
			ObjectId:    "_:bob0",
			ObjectValue: nil,
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland" .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: "Alice In Wonderland",
		},
	},
	{
		input: `_:alice <name> "Alice In Wonderland"@en-0 .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "name",
			ObjectId:    "",
			ObjectValue: "Alice In Wonderland",
			Language:    "en-0",
		},
	},
	{
		input: `_:alice <age> "013"^^<integer> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "age",
			ObjectId:    "",
			ObjectValue: "013@@integer",
		},
	},
	{
		input: `<http://www.w3.org/2001/sw/RDFCore/nedges/> <http://purl.org/dc/terms/title> "N-Edges"@en-US .`,
		nq: NQuad{
			Subject:     "http://www.w3.org/2001/sw/RDFCore/nedges/",
			Predicate:   "http://purl.org/dc/terms/title",
			ObjectId:    "",
			ObjectValue: "N-Edges",
			Language:    "en-US",
		},
	},
	{
		input: `_:art <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://xmlns.com/foaf/0.1/Person> .`,
		nq: NQuad{
			Subject:     "_:art",
			Predicate:   "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
			ObjectId:    "http://xmlns.com/foaf/0.1/Person",
			ObjectValue: nil,
		},
	},
	{
		input:  "_:alice .",
		hasErr: true,
	},
	{
		input:  "_:alice knows .",
		hasErr: true,
	},
	{
		input:  `_:alice "knows" stuff .`,
		hasErr: true,
	},
	{
		input:  "_:alice <knows> stuff .",
		hasErr: true,
	},
	{
		input:  "_:alice <knows> <stuff>",
		hasErr: true,
	},
	{
		input:  `"_:alice" <knows> <stuff> .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff"@-en .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff"^<string> .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff"^^xs:string .`,
		hasErr: true,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: "stuff@@xs:string",
		},
		hasErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> <label> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: "stuff@@xs:string",
			Label:       "label",
		},
		hasErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> _:label .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: "stuff@@xs:string",
			Label:       "_:label",
		},
		hasErr: false,
	},
	{
		input:  `_:alice <knows> "stuff"^^<xs:string> "label" .`,
		hasErr: true,
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
		input: `_:alice <likes> "mov\"enpick" .`, // ignores the <bob> after dot.
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "likes",
			ObjectValue: `mov\"enpick`,
		},
	},
}

func TestLex(t *testing.T) {
	for _, test := range testNQuads {
		rnq, err := Parse(test.input)
		if test.hasErr {
			if err == nil {
				t.Errorf("Expected error for input: %q. Output: %+v", test.input, rnq)
			}
			continue
		} else {
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		}

		if !reflect.DeepEqual(rnq, test.nq) {
			t.Errorf("Expected %v. Got: %v", test.nq, rnq)
		}
	}
}
