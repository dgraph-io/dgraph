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
	"unicode"

	farm "github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
)

var emptyEdge task.DirectedEdge

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
func GetUid(xid string) (uint64, error) {
	// If string represents a UID, convert to uint64 and return.
	if strings.HasPrefix(xid, "_uid_:") {
		return strconv.ParseUint(xid[6:], 0, 64)
	}
	return farm.Fingerprint64([]byte(xid)), nil
}

// ToEdge is useful when you want to find the UID corresponding to XID for
// just one edge. The method doesn't automatically generate a UID for an XID.
func (nq NQuad) ToEdge() (*task.DirectedEdge, error) {
	sid, err := GetUid(nq.Subject)
	if err != nil {
		return &emptyEdge, err
	}

	out := &task.DirectedEdge{
		Attr:   nq.Predicate,
		Label:  nq.Label,
		Entity: sid,
	}

	// An edge can have an id or a value.
	if len(nq.ObjectId) > 0 {
		oid, err := GetUid(nq.ObjectId)
		if err != nil {
			return &emptyEdge, err
		}
		out.ValueId = oid
	} else {
		out.Value = nq.ObjectValue
		out.ValueType = uint32(nq.ObjectType)
	}
	return out, nil
}

func toUid(xid string, newToUid map[string]uint64) (uid uint64, rerr error) {
	if id, present := newToUid[xid]; present {
		return id, nil
	}
	return GetUid(xid)
}

// ToEdgeUsing determines the UIDs for the provided XIDs and populates the
// xidToUid map.
func (nq NQuad) ToEdgeUsing(newToUid map[string]uint64) (*task.DirectedEdge, error) {
	uid, err := toUid(nq.Subject, newToUid)
	if err != nil {
		return &emptyEdge, err
	}

	out := &task.DirectedEdge{
		Entity: uid,
		Attr:   nq.Predicate,
		Label:  nq.Label,
	}

	if len(nq.ObjectId) == 0 {
		out.Value = nq.ObjectValue
		out.ValueType = uint32(nq.ObjectType)
	} else {
		uid, err = toUid(nq.ObjectId, newToUid)
		if err != nil {
			return &emptyEdge, err
		}
		out.ValueId = uid
	}
	return out, nil
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
				p := types.ValueForType(t)
				err := p.UnmarshalText([]byte(oval))
				if err != nil {
					return rnq, err
				}
				rnq.ObjectValue, err = p.MarshalBinary()
				if err != nil {
					return rnq, err
				}
				rnq.ObjectType = byte(t)
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

var typeMap = map[string]types.TypeID{
	"xs:string":                                   types.StringID,
	"xs:dateTime":                                 types.DateTimeID,
	"xs:date":                                     types.DateID,
	"xs:int":                                      types.Int32ID,
	"xs:boolean":                                  types.BoolID,
	"xs:double":                                   types.FloatID,
	"xs:float":                                    types.FloatID,
	"geo:geojson":                                 types.GeoID,
	"http://www.w3.org/2001/XMLSchema#string":     types.StringID,
	"http://www.w3.org/2001/XMLSchema#dateTime":   types.DateTimeID,
	"http://www.w3.org/2001/XMLSchema#date":       types.DateID,
	"http://www.w3.org/2001/XMLSchema#int":        types.Int32ID,
	"http://www.w3.org/2001/XMLSchema#boolean":    types.BoolID,
	"http://www.w3.org/2001/XMLSchema#double":     types.FloatID,
	"http://www.w3.org/2001/XMLSchema#float":      types.FloatID,
	"http://www.w3.org/2001/XMLSchema#gYear":      types.DateID,
	"http://www.w3.org/2001/XMLSchema#gYearMonth": types.DateID,
}
