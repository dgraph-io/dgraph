/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package rdf

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log"
	"strconv"
	"strings"
	"unicode"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

var emptyEdge protos.DirectedEdge
var (
	ErrEmpty      = errors.New("rdf: harmless error, e.g. comment line")
	ErrInvalidUID = errors.New("UID has to be greater than one.")
)

// Gets the uid corresponding
func ParseUid(xid string) (uint64, error) {
	// If string represents a UID, convert to uint64 and return.
	uid, err := strconv.ParseUint(xid, 0, 64)
	if err != nil {
		return 0, err
	}
	if uid == 0 {
		return 0, ErrInvalidUID
	}
	return uid, nil
}

type NQuad struct {
	*protos.NQuad
}

func typeValFrom(val *protos.Value) types.Val {
	switch val.Val.(type) {
	case *protos.Value_BytesVal:
		return types.Val{types.BinaryID, val.GetBytesVal()}
	case *protos.Value_IntVal:
		return types.Val{types.IntID, val.GetIntVal()}
	case *protos.Value_StrVal:
		return types.Val{types.StringID, val.GetStrVal()}
	case *protos.Value_BoolVal:
		return types.Val{types.BoolID, val.GetBoolVal()}
	case *protos.Value_DoubleVal:
		return types.Val{types.FloatID, val.GetDoubleVal()}
	case *protos.Value_GeoVal:
		return types.Val{types.GeoID, val.GetGeoVal()}
	case *protos.Value_DateVal:
		return types.Val{types.DateID, val.GetDateVal()}
	case *protos.Value_DatetimeVal:
		return types.Val{types.DateTimeID, val.GetDatetimeVal()}
	case *protos.Value_PasswordVal:
		return types.Val{types.PasswordID, val.GetPasswordVal()}
	case *protos.Value_DefaultVal:
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
func (nq NQuad) ToEdge() (*protos.DirectedEdge, error) {
	var err error
	sid, err := ParseUid(nq.Subject)
	if err != nil {
		return nil, err
	}
	out := &protos.DirectedEdge{
		Attr:   nq.Predicate,
		Label:  nq.Label,
		Lang:   nq.Lang,
		Entity: sid,
		Facets: nq.Facets,
	}

	switch nq.valueType() {
	case x.ValueUid:
		oid, err := ParseUid(nq.ObjectId)
		if err != nil {
			return nil, err
		}
		out.ValueId = oid
	case x.ValuePlain, x.ValueMulti:
		if err = copyValue(out, nq); err != nil {
			return &emptyEdge, err
		}
	}

	return out, nil
}

func toUid(subject string, newToUid map[string]uint64) (uid uint64, err error) {
	if id, err := ParseUid(subject); err == nil || err == ErrInvalidUID {
		return id, err
	}
	// It's an xid
	if id, present := newToUid[subject]; present {
		return id, err
	}
	return 0, x.Errorf("uid not found/generated for xid %s\n", subject)
}

// ToEdgeUsing determines the UIDs for the provided XIDs using the
// xidToUid map.
func (nq NQuad) ToEdgeUsing(newToUid map[string]uint64) (*protos.DirectedEdge, error) {
	var err error
	uid, err := toUid(nq.Subject, newToUid)
	if err != nil {
		return nil, err
	}
	out := &protos.DirectedEdge{
		Entity: uid,
		Attr:   nq.Predicate,
		Label:  nq.Label,
		Lang:   nq.Lang,
		Facets: nq.Facets,
	}

	switch nq.valueType() {
	case x.ValueUid:
		uid, err := toUid(nq.ObjectId, newToUid)
		if err != nil {
			return nil, err
		}
		out.ValueId = uid
	case x.ValuePlain, x.ValueMulti:
		if err = copyValue(out, nq); err != nil {
			return &emptyEdge, err
		}
	}
	return out, nil
}

func copyValue(out *protos.DirectedEdge, nq NQuad) error {
	var err error
	if out.Value, err = byteVal(nq); err != nil {
		return err
	}
	out.ValueType = uint32(nq.ObjectType)
	return nil
}

func (nq NQuad) valueType() x.ValueTypeInfo {
	hasValue := nq.ObjectValue != nil
	hasLang := len(nq.Lang) > 0
	hasSpecialId := len(nq.ObjectId) == 0
	return x.ValueType(hasValue, hasLang, hasSpecialId)
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
func Parse(line string) (rnq protos.NQuad, rerr error) {
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
		case itemVarKeyword:
			it.Next()
			if item = it.Item(); item.Typ != itemLeftRound {
				return rnq, x.Errorf("Expected '(', found: %s", item.Val)
			}
			it.Next()
			if item = it.Item(); item.Typ != itemVarName {
				return rnq, x.Errorf("Expected variable name, found: %s", item.Val)
			}
			rnq.SubjectVar = item.Val

			it.Next() // parse ')'

		case itemPredicate:
			rnq.Predicate = strings.Trim(item.Val, " ")

		case itemObject:
			rnq.ObjectId = strings.Trim(item.Val, " ")

		case itemStar:
			// This is a special case for predicate or object.
			if rnq.Predicate == "" {
				rnq.Predicate = x.DeleteAllPredicates
			} else {
				rnq.ObjectValue = &protos.Value{&protos.Value_DefaultVal{x.DeleteAllObjects}}
			}
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
				rnq.ObjectValue = &protos.Value{&protos.Value_DefaultVal{oval}}
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
			if rnq.Predicate == x.DeleteAllPredicates {
				return rnq, x.Errorf("If predicate is *, value should be * as well")
			}

			val := strings.Trim(item.Val, " ")
			// TODO: Check if this condition is required.
			if strings.Trim(val, " ") == "*" {
				return rnq, x.Errorf("itemObject can't be *")
			}
			// Lets find out the storage type from the type map.
			t, ok := typeMap[val]
			if !ok {
				return rnq, x.Errorf("Unrecognized rdf type %s", val)
			}
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
		rnq.ObjectValue = &protos.Value{&protos.Value_DefaultVal{oval}}
		// If no type is specified, we default to string.
		rnq.ObjectType = int32(types.DefaultID)
	}
	if (len(rnq.Subject) == 0 && len(rnq.SubjectVar) == 0) || len(rnq.Predicate) == 0 {
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

// ConvertToNQuads parses multi line mutation string to a list of NQuads.
func ConvertToNQuads(mutation string) ([]*protos.NQuad, error) {
	var nquads []*protos.NQuad
	r := strings.NewReader(mutation)
	reader := bufio.NewReader(r)
	// x.Trace(ctx, "Converting to NQuad")

	var strBuf bytes.Buffer
	var err error
	for {
		err = x.ReadLine(reader, &strBuf)
		if err != nil {
			break
		}
		ln := strings.Trim(strBuf.String(), " \t")
		if len(ln) == 0 {
			continue
		}
		nq, err := Parse(ln)
		if err == ErrEmpty { // special case: comment/empty line
			continue
		} else if err != nil {
			// x.TraceError(ctx, x.Wrapf(err, "Error while parsing RDF"))
			return nquads, err
		}
		nquads = append(nquads, &nq)
	}
	if err != io.EOF {
		return nquads, err
	}
	return nquads, nil
}

func parseFacets(it *lex.ItemIterator, rnq *protos.NQuad) error {
	if !it.Next() {
		return x.Errorf("Unexpected end of facets.")
	}
	item := it.Item()
	if item.Typ != itemLeftRound {
		return x.Errorf("Expected '(' but found %v at Facet.", item.Val)
	}

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
		facet, err := facets.FacetFor(facetKey, facetVal)
		if err != nil {
			return err
		}
		rnq.Facets = append(rnq.Facets, facet)

		// empty value case..
		if item.Typ == itemRightRound {
			break
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
			break
		}
		if item.Typ == itemComma {
			continue
		}
		return x.Errorf("Expected , or ) after facet. Received %s", item.Val)
	}

	if rnq.Facets != nil {
		facets.SortFacets(rnq.Facets)
	}
	for i := 1; i < len(rnq.Facets); i++ {
		if rnq.Facets[i-1].Key == rnq.Facets[i].Key {
			return x.Errorf("Repeated keys are not allowed in facets. But got %s",
				rnq.Facets[i].Key)
		}
	}
	return nil
}

func isNewline(r rune) bool {
	return r == '\n' || r == '\r'
}

var typeMap = map[string]types.TypeID{
	"xs:string":                                   types.StringID,
	"xs:dateTime":                                 types.DateTimeID,
	"xs:date":                                     types.DateID,
	"xs:int":                                      types.IntID,
	"xs:boolean":                                  types.BoolID,
	"xs:double":                                   types.FloatID,
	"xs:float":                                    types.FloatID,
	"geo:geojson":                                 types.GeoID,
	"pwd:password":                                types.PasswordID,
	"http://www.w3.org/2001/XMLSchema#string":     types.StringID,
	"http://www.w3.org/2001/XMLSchema#dateTime":   types.DateTimeID,
	"http://www.w3.org/2001/XMLSchema#date":       types.DateID,
	"http://www.w3.org/2001/XMLSchema#int":        types.IntID,
	"http://www.w3.org/2001/XMLSchema#integer":    types.IntID,
	"http://www.w3.org/2001/XMLSchema#boolean":    types.BoolID,
	"http://www.w3.org/2001/XMLSchema#double":     types.FloatID,
	"http://www.w3.org/2001/XMLSchema#float":      types.FloatID,
	"http://www.w3.org/2001/XMLSchema#gYear":      types.DateID,
	"http://www.w3.org/2001/XMLSchema#gYearMonth": types.DateID,
}
