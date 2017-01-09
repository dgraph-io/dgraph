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
	"log"
	"strconv"
	"strings"
	"unicode"

	farm "github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var emptyEdge task.DirectedEdge

// Gets the uid corresponding to an xid from the posting list which stores the
// mapping.
func GetUid(xid string) uint64 {
	// If string represents a UID, convert to uint64 and return.
	uid, err := strconv.ParseUint(xid, 0, 64)
	if err != nil {
		return farm.Fingerprint64([]byte(xid))
	}
	return uid
}

type NQuad struct {
	*graph.NQuad
}

func typeValFrom(val *graph.Value) types.Val {
	switch val.Val.(type) {
	case *graph.Value_BytesVal:
		return types.Val{types.BinaryID, val.GetBytesVal()}
	case *graph.Value_IntVal:
		return types.Val{types.Int32ID, val.GetIntVal()}
	case *graph.Value_StrVal:
		return types.Val{types.StringID, val.GetStrVal()}
	case *graph.Value_BoolVal:
		return types.Val{types.BoolID, val.GetBoolVal()}
	case *graph.Value_DoubleVal:
		return types.Val{types.FloatID, val.GetDoubleVal()}
	case *graph.Value_GeoVal:
		return types.Val{types.GeoID, val.GetGeoVal()}
	case *graph.Value_DateVal:
		return types.Val{types.DateID, val.GetDateVal()}
	case *graph.Value_DatetimeVal:
		return types.Val{types.DateTimeID, val.GetDatetimeVal()}
	}
	return types.Val{types.StringID, ""}
}

func byteVal(nq NQuad) ([]byte, error) {
	// We infer object type from type of value. We set appropriate type in parse
	// function or the Go client has already set.
	p := typeValFrom(nq.ObjectValue)
	// These three would have already been marshalled to bytes by the client or
	// in parse function.
	if p.Tid == types.GeoID || p.Tid == types.DateID || p.Tid == types.DateTimeID {
		return p.Value.([]byte), nil
	}

	p1 := types.ValueForType(types.BinaryID)
	if err := types.Marshal(p, &p1); err != nil {
		return []byte{}, err
	}
	return []byte(p1.Value.([]byte)), nil
}

// ToEdge is useful when you want to find the UID corresponding to XID for
// just one edge. The method doesn't automatically generate a UID for an XID.
func (nq NQuad) ToEdge() (*task.DirectedEdge, error) {
	var err error
	sid := GetUid(nq.Subject)

	out := &task.DirectedEdge{
		Attr:   nq.Predicate,
		Label:  nq.Label,
		Entity: sid,
	}

	// An edge can have an id or a value.
	if len(nq.ObjectId) > 0 {
		oid := GetUid(nq.ObjectId)
		out.ValueId = oid
	} else {
		if out.Value, err = byteVal(nq); err != nil {
			return &emptyEdge, err
		}
		out.ValueType = uint32(nq.ObjectType)
	}
	return out, nil
}

func toUid(xid string, newToUid map[string]uint64) (uid uint64) {
	if id, present := newToUid[xid]; present {
		return id
	}
	return GetUid(xid)
}

// ToEdgeUsing determines the UIDs for the provided XIDs and populates the
// xidToUid map.
func (nq NQuad) ToEdgeUsing(newToUid map[string]uint64) (*task.DirectedEdge, error) {
	var err error
	uid := toUid(nq.Subject, newToUid)

	out := &task.DirectedEdge{
		Entity: uid,
		Attr:   nq.Predicate,
		Label:  nq.Label,
	}

	if len(nq.ObjectId) == 0 {
		if out.Value, err = byteVal(nq); err != nil {
			return &emptyEdge, err
		}
		out.ValueType = uint32(nq.ObjectType)
	} else {
		uid = toUid(nq.ObjectId, newToUid)
		out.ValueId = uid
	}
	return out, nil
}

// This function is used to extract an IRI from an IRIREF.
func stripBracketsAndTrim(val string) (string, bool) {
	if val[0] != '<' && val[len(val)-1] != '>' {
		return strings.Trim(val, " "), false
	}
	return strings.Trim(val[1:len(val)-1], " "), true
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
func Parse(line string) (rnq graph.NQuad, rerr error) {
	l := lex.NewLexer(line).Run(lexText)
	it := l.NewIterator()
	var oval string
	var vend, hasBrackets bool
	// We read items from the l.Items channel to which the lexer sends items.
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemSubject:
			rnq.Subject, hasBrackets = stripBracketsAndTrim(item.Val)
			if hasBrackets && len(rnq.Subject) > 1 && rnq.Subject[0:2] == "_:" {
				return rnq, x.Errorf("Blank nodes cannot be in IRI")
			}

		case itemPredicate:
			rnq.Predicate, _ = stripBracketsAndTrim(item.Val)

		case itemObject:
			rnq.ObjectId, hasBrackets = stripBracketsAndTrim(item.Val)
			if hasBrackets && len(rnq.ObjectId) > 1 && rnq.ObjectId[0:2] == "_:" {
				return rnq, x.Errorf("Blank nodes cannot be in IRI")
			}

		case itemLiteral:
			oval = item.Val
			if oval == "" {
				oval = "_nil_"
			}

		case itemLanguage:
			rnq.Predicate += "." + item.Val

		case itemObjectType:
			if len(oval) == 0 {
				log.Fatalf(
					"itemObject should be emitted before itemObjectType. Input: [%s]",
					line)
			}
			val, _ := stripBracketsAndTrim(item.Val)
			// TODO: Check if this condition is required.
			if strings.Trim(val, " ") == "*" {
				return rnq, x.Errorf("itemObject can't be *")
			}
			if t, ok := typeMap[val]; ok {
				if oval == "_nil_" && t != types.StringID {
					return rnq, x.Errorf("Invalid ObjectValue")
				}
				rnq.ObjectType = int32(t)
				p := types.ValueForType(t)
				src := types.ValueForType(types.StringID)
				src.Value = []byte(oval)
				err := types.Convert(src, &p)
				if err != nil {
					return rnq, err
				}

				if rnq.ObjectValue, err = types.ObjectValue(t, p.Value); err != nil {
					return rnq, err
				}
				oval = ""
			} else {
				oval += "@@" + val
			}

		case lex.ItemError:
			return rnq, x.Errorf(item.Val)

		case itemValidEnd:
			vend = true

		case itemLabel:
			rnq.Label, _ = stripBracketsAndTrim(item.Val)
		}
	}

	if !vend {
		return rnq, x.Errorf("Invalid end of input. Input: [%s]", line)
	}
	if len(oval) > 0 {
		rnq.ObjectValue = &graph.Value{&graph.Value_StrVal{oval}}
		// If no type is specified, we default to string.
		rnq.ObjectType = int32(0)
	}
	if len(rnq.Subject) == 0 || len(rnq.Predicate) == 0 {
		return rnq, x.Errorf("Empty required fields in NQuad. Input: [%s]", line)
	}
	if len(rnq.ObjectId) == 0 && rnq.ObjectValue == nil {
		return rnq, x.Errorf("No Object in NQuad. Input: [%s]", line)
	}
	if !sane(rnq.Subject) || !sane(rnq.Predicate) || !sane(rnq.ObjectId) ||
		!sane(rnq.Label) {
		return rnq, x.Errorf("NQuad failed sanity check:%+v", rnq)
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
