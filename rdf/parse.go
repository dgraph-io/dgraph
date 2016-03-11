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
}

func getUid(xid string) (uint64, error) {
	if strings.HasPrefix(xid, "_uid_:") {
		return strconv.ParseUint(xid[6:], 0, 64)
	}
	return uid.Get(xid)
}

// ToEdge is useful when you want to find the UID corresponding to XID for
// just one edge. ToEdgeUsing(map) is useful when you do this conversion
// in bulk, say over a network call. None of these methods generate a UID
// for an XID.
func (nq NQuad) ToEdge() (result x.DirectedEdge, rerr error) {

	sid, err := getUid(nq.Subject)
	if err != nil {
		return result, err
	}
	result.Entity = sid
	if len(nq.ObjectId) > 0 {
		oid, err := getUid(nq.ObjectId)
		if err != nil {
			return result, err
		}
		result.ValueId = oid
	} else {
		result.Value = nq.ObjectValue
	}
	result.Attribute = nq.Predicate
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
	result.Attribute = nq.Predicate
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
			rnq.Predicate += "." + item.Val
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
