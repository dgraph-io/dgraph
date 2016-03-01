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
	"strconv"
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

func GetUid(s string, instanceIdx uint64, numInstances uint64) (uint64, error) {
	if strings.HasPrefix(s, "_uid_:") {
		return strconv.ParseUint(s[6:], 0, 64)
	}
	return uid.GetOrAssign(s, instanceIdx, numInstances)
}

func (nq NQuad) ToEdge(instanceIdx,
	numInstances uint64) (result x.DirectedEdge, rerr error) {

	sid, err := GetUid(nq.Subject, instanceIdx, numInstances)
	if err != nil {
		return result, err
	}
	result.Entity = sid
	if len(nq.ObjectId) > 0 {
		oid, err := GetUid(nq.ObjectId, instanceIdx, numInstances)
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

func toUid(xid string, xidToUid map[string]uint64) (uid uint64, rerr error) {
	id, present := xidToUid[xid]
	if present {
		return id, nil
	}

	if !strings.HasPrefix(xid, "_uid_:") {
		return 0, fmt.Errorf("Unable to find xid: %v", xid)
	}
	return strconv.ParseUint(xid[6:], 0, 64)
}

func (nq NQuad) ToEdgeUsing(
	xidToUid map[string]uint64) (result x.DirectedEdge, rerr error) {
	uid, err := toUid(nq.Subject, xidToUid)
	if err != nil {
		return result, err
	}
	result.Entity = uid

	if len(nq.ObjectId) == 0 {
		result.Value = nq.ObjectValue
	} else {
		uid, err = toUid(nq.ObjectId, xidToUid)
		if err != nil {
			return result, err
		}
		result.ValueId = uid
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
	l := &lex.Lexer{}
	l.Init(line)

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
					"itemObject should be emitted before itemObjectType. Input: %q",
					line)
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
