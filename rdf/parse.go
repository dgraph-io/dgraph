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
	"errors"
	"log"
	"strconv"
	"strings"
	"unicode"

	farm "github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/query/graph"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

var emptyEdge task.DirectedEdge
var (
	ErrEmpty = errors.New("rdf: harmless error, e.g. comment line")
)

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
	case *graph.Value_PasswordVal:
		return types.Val{types.PasswordID, val.GetPasswordVal()}
	case *graph.Value_DefaultVal:
		return types.Val{types.DefaultID, val.GetDefaultVal()}
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
		Lang:   nq.Lang,
		Entity: sid,
		Facets: nq.Facets,
	}

	switch nq.valueType() {
	case nQuadUid:
		oid := GetUid(nq.ObjectId)
		out.ValueId = oid
	case nQuadValue, nQuadTaggedValue:
		if err = copyValue(out, nq); err != nil {
			return &emptyEdge, err
		}
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
		Lang:   nq.Lang,
		Facets: nq.Facets,
	}

	switch nq.valueType() {
	case nQuadUid:
		uid = toUid(nq.ObjectId, newToUid)
		out.ValueId = uid
	case nQuadValue, nQuadTaggedValue:
		if err = copyValue(out, nq); err != nil {
			return &emptyEdge, err
		}
	}
	return out, nil
}

func copyValue(out *task.DirectedEdge, nq NQuad) error {
	var err error
	if out.Value, err = byteVal(nq); err != nil {
		return err
	}
	out.ValueType = uint32(nq.ObjectType)
	return nil
}

type nQuadTypeInfo int32

const (
	nQuadEmpty nQuadTypeInfo = iota
	nQuadUid
	nQuadValue
	nQuadTaggedValue
)

func (nq NQuad) valueType() nQuadTypeInfo {
	if nq.ObjectValue != nil {
		if len(nq.Lang) == 0 {
			return nQuadValue // value without lang tag
		} else {
			return nQuadTaggedValue // value with lang tag
		}
	} else {
		if len(nq.ObjectId) == 0 {
			return nQuadEmpty // empty NQuad - no Uid and no Value
		} else {
			return nQuadUid // Uid
		}
	}
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
	var vend bool
	isCommentLine := false
	// We read items from the l.Items channel to which the lexer sends items.
	for it.Next() {
		item := it.Item()
		switch item.Typ {
		case itemSubject:
			rnq.Subject = strings.Trim(item.Val, " ")
		case itemPredicate:
			rnq.Predicate = strings.Trim(item.Val, " ")

		case itemObject:
			rnq.ObjectId = strings.Trim(item.Val, " ")

		case itemLiteral:
			oval = item.Val
			if oval == "" {
				oval = "_nil_"
			}

		case itemLanguage:
			rnq.Lang = item.Val

			// if lang tag is specified then type is set to string
			// grammar allows either ^^ iriref or lang tag
			if len(oval) > 0 {
				rnq.ObjectValue = &graph.Value{&graph.Value_DefaultVal{oval}}
				// If no type is specified, we default to string.
				rnq.ObjectType = int32(types.StringID)
				oval = ""
			}
		case itemObjectType:
			if len(oval) == 0 {
				log.Fatalf(
					"itemObject should be emitted before itemObjectType. Input: [%s]",
					line)
			}
			val := strings.Trim(item.Val, " ")
			// TODO: Check if this condition is required.
			if strings.Trim(val, " ") == "*" {
				return rnq, x.Errorf("itemObject can't be *")
			}
			// Lets find out the storage type from the type map.
			if t, ok := typeMap[val]; ok {
				if oval == "_nil_" && t != types.StringID {
					return rnq, x.Errorf("Invalid ObjectValue")
				}
				rnq.ObjectType = int32(t)
				src := types.ValueForType(types.StringID)
				src.Value = []byte(oval)
				p, err := types.Convert(src, t)
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

		case itemComment:
			isCommentLine = true
			vend = true

		case itemValidEnd:
			vend = true

		case itemLabel:
			rnq.Label = strings.Trim(item.Val, " ")

		case itemLeftRound:
			it.Prev() // backup '('
			if err := parseFacets(it, &rnq); err != nil {
				return rnq, x.Errorf(err.Error())
			}
		}
	}

	if !vend {
		return rnq, x.Errorf("Invalid end of input. Input: [%s]", line)
	}
	if isCommentLine {
		return rnq, ErrEmpty
	}
	if len(oval) > 0 {
		rnq.ObjectValue = &graph.Value{&graph.Value_DefaultVal{oval}}
		// If no type is specified, we default to string.
		rnq.ObjectType = int32(types.DefaultID)
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

func parseFacets(it *lex.ItemIterator, rnq *graph.NQuad) error {
	if !it.Next() {
		return x.Errorf("Unexpected end of facets.")
	}
	item := it.Item()
	if item.Typ != itemLeftRound {
		return x.Errorf("Expected '(' but found %v at Facet.", item.Val)
	}
	defer func() { // always sort facets before returning.
		if rnq.Facets != nil {
			facets.SortFacets(rnq.Facets)
		}
	}()

	for it.Next() { // parse one key value pair
		// parse key
		item = it.Item()
		if item.Typ != itemText {
			return x.Errorf("Expected key but found %v.", item.Val)
		}
		facetKey := strings.TrimSpace(item.Val)
		if len(facetKey) == 0 {
			return x.Errorf("Empty facetKeys not allowed.")
		}
		// parse =
		if !it.Next() {
			return x.Errorf("Unexpected end of facets.")
		}
		item = it.Item()
		if item.Typ != itemEqual {
			return x.Errorf("Expected = after facetKey. Found %v", item.Val)
		}
		// parse value or empty value
		if !it.Next() {
			return x.Errorf("Unexpected end of facets.")
		}
		item = it.Item()
		facetVal := ""
		if item.Typ == itemText {
			facetVal = item.Val
		}
		valTyp, err := facets.ValType(facetVal)
		if err != nil {
			return err
		}
		rnq.Facets = append(rnq.Facets,
			&facets.Facet{Key: facetKey, Value: []byte(facetVal), ValType: valTyp})

		// empty value case..
		if item.Typ == itemRightRound {
			return nil
		}
		if item.Typ == itemComma {
			continue
		}
		if item.Typ != itemText {
			return x.Errorf("Expected , or ) or text but found %s", item.Val)
		}
		// value was present..
		if !it.Next() { // get either ')' or ','
			return x.Errorf("Unexpected end of facets.")
		}
		item = it.Item()
		if item.Typ == itemRightRound {
			return nil
		}
		if item.Typ == itemComma {
			continue
		}
		return x.Errorf("Expected , or ) after facet. Received %s", item.Val)
	}
	return x.Errorf("Unexpected end of facets.")
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
	"pwd:password":                                types.PasswordID,
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
