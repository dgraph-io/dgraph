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
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
)

type NQuad struct {
	Subject     string
	Predicate   string
	ObjectId    string
	ObjectValue interface{}
	Label       string
	Language    string
}

func (nq NQuad) ToEdge() (result x.DirectedEdge, rerr error) {
	sid, err := uid.GetOrAssign(nq.Subject)
	if err != nil {
		return result, err
	}
	result.Entity = sid
	if len(nq.ObjectId) > 0 {
		oid, err := uid.GetOrAssign(nq.ObjectId)
		if err != nil {
			return result, err
		}
		result.ValueId = oid
	} else {
		result.Value = nq.ObjectValue
	}
	if len(nq.Language) > 0 {
		result.Attribute = nq.Predicate + "." + nq.Language
	} else {
		result.Attribute = nq.Predicate
	}
	result.Source = nq.Label
	result.Timestamp = time.Now()
	return result, nil
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
	var vend bool
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
			// TODO: Strictly parse common types like integers, floats etc.
			if len(oval) == 0 {
				glog.Fatalf(
					"itemObject should be emitted before itemObjectType. Input: %q", line)
			}
			oval += "@@" + stripBracketsIfPresent(item.Val)
		}
		if item.Typ == lex.ItemError {
			return rnq, fmt.Errorf(item.Val)
		}
		if item.Typ == itemValidEnd {
			vend = true
		}
		if item.Typ == itemLabel {
			rnq.Label = stripBracketsIfPresent(item.Val)
		}
	}
	if !vend {
		return rnq, fmt.Errorf("Invalid end of input")
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

func isNewline(r rune) bool {
	return r == '\n' || r == '\r'
}

func ParseStream(reader io.Reader, cnq chan NQuad, done chan error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.Trim(line, " \t")
		if len(line) == 0 {
			continue
		}

		glog.Debugf("Got line: %q", line)
		nq, err := Parse(line)
		if err != nil {
			x.Err(glog, err).Errorf("While parsing: %q", line)
			done <- err
			return
		}
		cnq <- nq
	}
	if err := scanner.Err(); err != nil {
		x.Err(glog, err).Error("While scanning input")
		done <- err
		return
	}
	done <- nil
}
