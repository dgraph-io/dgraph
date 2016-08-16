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
		input: `_:alice <age> "013"^^<integer> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "age",
			ObjectId:    "",
			ObjectValue: []byte("013@@integer"),
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
		input:  "_:alice .",
		hasErr: true,
	},
	{
		input:  "_:alice knows .",
		hasErr: true,
	},
	{
		input:  "<alice> <knows> .",
		hasErr: true,
	},
	{
		input:  "_uid_: 0x01 <knows> <something> .",
		hasErr: true,
	},
	{
		input:  "<alice> <knows> _uid_: 0x01 .",
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
		input:  `<alice> <knows> * .`,
		hasErr: true,
	},
	{
		input:  `<alice> <knows> <*> .`,
		hasErr: true,
	},
	{
		input:  `<*> <knows> "stuff" .`,
		hasErr: true,
	},
	{
		input:  `<alice> <*> "stuff" .`,
		hasErr: true,
	},
	{
		input:  `<alice> < * > "stuff" .`,
		hasErr: true,
	},
	{
		input:  `<alice> <*> "stuff" .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff"^^< * > .`,
		hasErr: true,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("stuff@@xs:string"),
		},
		hasErr: false,
	},
	{
		input: `<alice> <knows> "*" .`,
		nq: NQuad{
			Subject:     "alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("*"),
		},
		hasErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> <label> .`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("stuff@@xs:string"),
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
			ObjectValue: []byte("stuff@@xs:string"),
			Label:       "_:label",
		},
		hasErr: false,
	},
	{
		input: `_:alice <knows> "stuff"^^<xs:string> _:label . # comment`,
		nq: NQuad{
			Subject:     "_:alice",
			Predicate:   "knows",
			ObjectId:    "",
			ObjectValue: []byte("stuff@@xs:string"),
			Label:       "_:label",
		},
		hasErr: false,
	},
	{
		input:  `_:alice <knows> "stuff"^^<xs:string> "label" .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff"^^<xs:string> _uid_:0x01 .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff"^^<xs:string> <quad> <pentagon> .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff"^^<xs:string> quad .`,
		hasErr: true,
	},
	{
		input:  `_:alice <knows> "stuff"^^<xs:string> <*> .`,
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
