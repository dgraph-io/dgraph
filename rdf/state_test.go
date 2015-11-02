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
	"testing"

	"github.com/dgraph-io/dgraph/lex"
)

var testNQuads = []struct {
	input   string
	entity  string
	attr    string
	valueid string
	value   interface{}
}{
	{
		input:   `<some_subject_id> <predicate> <Object_id> .`,
		entity:  "<some_subject_id>",
		attr:    "<predicate>",
		valueid: "<Object_id>",
	},
	{
		input:   `_:alice <predicate> <Object_id> .`,
		entity:  "_:alice",
		attr:    "<predicate>",
		valueid: "<Object_id>",
	},
	{
		input:   `_:alice <follows> _:bob0 .`,
		entity:  "_:alice",
		attr:    "<follows>",
		valueid: "_:bob0",
	},
	{
		input:   `_:alice <name> "Alice In Wonderland" .`,
		entity:  "_:alice",
		attr:    "<name>",
		valueid: "Alice In Wonderland",
	},
	{
		input:   `_:alice <name> "Alice In Wonderland"@en-0 .`,
		entity:  "_:alice",
		attr:    "<name>",
		valueid: "Alice In Wonderland",
	},
	{
		input:   `_:alice <age> "013"^^<integer> .`,
		entity:  "_:alice",
		attr:    "<age>",
		valueid: "Alice In Wonderland",
	},
	{
		input:   `<http://www.w3.org/2001/sw/RDFCore/ntriples/> <http://purl.org/dc/terms/title> "N-Triples"@en-US .`,
		entity:  "<http://www.w3.org/2001/sw/RDFCore/ntriples/>",
		attr:    "<http://purl.org/dc/terms/title>",
		valueid: "Alice In Wonderland",
	},
}

func TestLex(t *testing.T) {
	for _, test := range testNQuads {
		l := lex.NewLexer(test.input)
		go run(l)
		for item := range l.Items {
			t.Logf("Item: %v", item)
			if item.Typ == itemSubject {
				if item.Val != test.entity {
					t.Errorf("Expected: %v. Got: %v", test.entity, item.Val)
				} else {
					t.Logf("Subject matches")
				}
			}
			if item.Typ == itemPredicate {
				if item.Val != test.attr {
					t.Errorf("Expected: %v. Got: %v", test.attr, item.Val)
				} else {
					t.Logf("Predicate matches")
				}
			}
			if item.Typ == itemObject {
				if item.Val != test.valueid {
					t.Errorf("Expected: %v. Got: %v", test.valueid, item.Val)
				} else {
					t.Logf("Object matches")
				}
			}
		}
	}
}
