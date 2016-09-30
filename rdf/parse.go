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
	"log"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/uid"
	"github.com/dgraph-io/dgraph/x"
	farm "github.com/dgryski/go-farm"
)

// NQuad is the data structure used for storing rdf N-Quads.
type NQuad struct {
	Subject     string
	Predicate   string
	ObjectId    string
	ObjectValue []byte
	ObjectType  byte
	Label       string
}

// Gets the uid corresponding to an xid from the posting list which stores the
// mapping.
func getUid(xid string) (uint64, error) {
	// If string represents a UID, convert to uint64 and return.
	if strings.HasPrefix(xid, "_uid_:") {
		return strconv.ParseUint(xid[6:], 0, 64)
	}
	// Get uid from posting list in UidStore.
	return uid.Get(xid)
}

// ToEdge is useful when you want to find the UID corresponding to XID for
// just one edge. The method doesn't automatically generate a UID for an XID.
func (nq NQuad) ToEdge() (result x.DirectedEdge, rerr error) {
	sid, err := getUid(nq.Subject)
	if err != nil {
		return result, err
	}

	result.Entity = sid
	// An edge can have an id or a value.
	if len(nq.ObjectId) > 0 {
		oid, err := getUid(nq.ObjectId)
		if err != nil {
			return result, err
		}
		result.ValueId = oid
	} else {
		result.Value = nq.ObjectValue
		result.ValueType = nq.ObjectType
	}
	result.Attribute = nq.Predicate
	result.Source = nq.Label
	result.Timestamp = time.Now()
	return result, nil
}

func toUid(xid string, newToUid map[string]uint64) (uid uint64, rerr error) {
	if id, present := newToUid[xid]; present {
		return id, nil
	}
	if strings.HasPrefix(xid, "_xid_:") {
		return farm.Fingerprint64([]byte(xid[6:])), nil
	}
	if strings.HasPrefix(xid, "_uid_:") {
		return strconv.ParseUint(xid[6:], 0, 64)
	}
	return 0, fmt.Errorf("Unable to assign or find uid for: %v", xid)
}

// ToEdgeUsing determines the UIDs for the provided XIDs and populates the
// xidToUid map.
func (nq NQuad) ToEdgeUsing(
	newToUid map[string]uint64) (result x.DirectedEdge, rerr error) {
	uid, err := toUid(nq.Subject, newToUid)
	if err != nil {
		return result, err
	}
	result.Entity = uid

	if len(nq.ObjectId) == 0 {
		result.Value = nq.ObjectValue
		result.ValueType = nq.ObjectType
	} else {
		uid, err = toUid(nq.ObjectId, newToUid)
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

// This function is used to extract an IRI from an IRIREF.
func stripBracketsAndTrim(val string) string {
	if val[0] != '<' && val[len(val)-1] != '>' {
		return strings.Trim(val, " ")
	}
	return strings.Trim(val[1:len(val)-1], " ")
}

// Function to do sanity check for subject, predicate, object and label strings.
func sane(s string) bool {
	// Label and ObjectId can be "", we already check that subject and predicate
	// shouldn't be empty.
	if len(s) == 0 {
		return true
	}
	// s should have atleast one alphanumeric character.
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// Parse parses a mutation string and returns the NQuad representation for it.
func Parse(line string) (rnq NQuad, rerr error) {
	l := &lex.Lexer{}
	l.Init(line)

	go run(l)
	var oval string
	var vend bool
	// We read items from the l.Items channel to which the lexer sends items.
	for item := range l.Items {
		switch item.Typ {
		case itemSubject:
			rnq.Subject = stripBracketsAndTrim(item.Val)

		case itemPredicate:
			rnq.Predicate = stripBracketsAndTrim(item.Val)

		case itemObject:
			rnq.ObjectId = stripBracketsAndTrim(item.Val)

		case itemLiteral:
			oval = item.Val

		case itemLanguage:
			rnq.Predicate += "." + item.Val

		case itemObjectType:
			if len(oval) == 0 {
				log.Fatalf(
					"itemObject should be emitted before itemObjectType. Input: [%s]",
					line)
			}
			val := stripBracketsAndTrim(item.Val)
			if strings.Trim(val, " ") == "*" {
				return rnq, fmt.Errorf("itemObject can't be *")
			}
			if t, ok := typeMap[val]; ok {
				p, err := t.Unmarshaler.FromText([]byte(oval))
				if err != nil {
					return rnq, err
				}
				rnq.ObjectValue, err = p.MarshalBinary()
				if err != nil {
					return rnq, err
				}
				rnq.ObjectType = byte(t.ID())
				oval = ""
			} else {
				oval += "@@" + val
			}

		case lex.ItemError:
			return rnq, fmt.Errorf(item.Val)

		case itemValidEnd:
			vend = true

		case itemLabel:
			rnq.Label = stripBracketsAndTrim(item.Val)
		}
	}

	if !vend {
		return rnq, fmt.Errorf("Invalid end of input. Input: [%s]", line)
	}
	if len(oval) > 0 {
		rnq.ObjectValue = []byte(oval)
		// If no type is specified, we default to string.
		rnq.ObjectType = 0
	}
	if len(rnq.Subject) == 0 || len(rnq.Predicate) == 0 {
		return rnq, fmt.Errorf("Empty required fields in NQuad. Input: [%s]", line)
	}
	if len(rnq.ObjectId) == 0 && rnq.ObjectValue == nil {
		return rnq, fmt.Errorf("No Object in NQuad. Input: [%s]", line)
	}
	if !sane(rnq.Subject) || !sane(rnq.Predicate) || !sane(rnq.ObjectId) ||
		!sane(rnq.Label) {
		return rnq, fmt.Errorf("NQuad failed sanity check:%+v", rnq)
	}

	return rnq, nil
}

func isNewline(r rune) bool {
	return r == '\n' || r == '\r'
}

var typeMap = map[string]types.Scalar{
	"xs:string":                                 types.StringType,
	"xs:dateTime":                               types.DateTimeType,
	"xs:date":                                   types.DateType,
	"xs:int":                                    types.Int32Type,
	"xs:boolean":                                types.BooleanType,
	"xs:double":                                 types.FloatType,
	"xs:float":                                  types.FloatType,
	"geo:geojson":                               types.GeoType,
	"http://www.w3.org/2001/XMLSchema#string":   types.StringType,
	"http://www.w3.org/2001/XMLSchema#dateTime": types.DateTimeType,
	"http://www.w3.org/2001/XMLSchema#date":     types.DateType,
	"http://www.w3.org/2001/XMLSchema#int":      types.Int32Type,
	"http://www.w3.org/2001/XMLSchema#boolean":  types.BooleanType,
	"http://www.w3.org/2001/XMLSchema#double":   types.FloatType,
	"http://www.w3.org/2001/XMLSchema#float":    types.FloatType,
}
