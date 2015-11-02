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
	"fmt"

	"github.com/dgraph-io/dgraph/lex"
)

type NQuad struct {
	Subject     string
	Predicate   string
	ObjectId    string
	ObjectValue interface{}
	Label       string
	Language    string
}

func stripBracketsIfPresent(val string) string {
	if val[0] != '<' {
		return val
	}
	if val[len(val)-1] != '>' {
		return val
	}
	return val[1 : len(val)-1]
}

func Parse(line string) (rnq NQuad, rerr error) {
	l := lex.NewLexer(line)
	go run(l)
	var oval string
	for item := range l.Items {
		if item.Typ == itemSubject {
			rnq.Subject = stripBracketsIfPresent(item.Val)
		}
		if item.Typ == itemPredicate {
			rnq.Predicate = stripBracketsIfPresent(item.Val)
		}
		if item.Typ == itemObject {
			rnq.ObjectId = stripBracketsIfPresent(item.Val)
		}
		if item.Typ == itemLiteral {
			oval = item.Val
		}
		if item.Typ == itemLanguage {
			rnq.Language = item.Val
		}
		if item.Typ == itemObjectType {
			if len(oval) == 0 {
				glog.Fatalf(
					"itemObject should be emitted before itemObjectType. Input: %q", line)
			}
			oval += "@@" + stripBracketsIfPresent(item.Val)
		}
	}
	if len(oval) > 0 {
		rnq.ObjectValue = oval
	}
	if len(rnq.Subject) == 0 || len(rnq.Predicate) == 0 {
		return rnq, fmt.Errorf("Empty required fields in NQuad")
	}
	if len(rnq.ObjectId) == 0 && rnq.ObjectValue == nil {
		return rnq, fmt.Errorf("No Object in NQuad")
	}
	return rnq, nil
}
