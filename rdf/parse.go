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
	"strconv"
	"strings"
	"unicode"

	"github.com/dgraph-io/dgraph/lex"
	"github.com/dgraph-io/dgraph/protos/api"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
)

var (
	ErrEmpty      = errors.New("rdf: harmless error, e.g. comment line")
	ErrInvalidUID = errors.New("UID has to be greater than zero.")
)

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
func Parse(line string) (api.NQuad, error) {
	var rnq api.NQuad
	l := lex.Lexer{
		Input: line,
	}
	l.Run(lexText)

	it := l.NewIterator()
	var oval string
	var seenOval bool
	var vend bool
	isCommentLine := false
	// We read items from the l.Items channel to which the lexer sends items.
L:
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

			it.Next() // parse ')'

		case itemPredicate:
			rnq.Predicate = strings.Trim(item.Val, " ")

		case itemObject:
			rnq.ObjectId = strings.Trim(item.Val, " ")

		case itemStar:
			if rnq.Subject == "" {
				rnq.Subject = x.Star
			} else if rnq.Predicate == "" {
				rnq.Predicate = x.Star
			} else {
				rnq.ObjectValue = &api.Value{&api.Value_DefaultVal{x.Star}}
			}
		case itemLiteral:
			var err error
			oval, err = strconv.Unquote(item.Val)
			if err != nil {
				return rnq, x.Wrapf(err, "while unquoting")
			}
			seenOval = true

		case itemLanguage:
			rnq.Lang = item.Val

		case itemObjectType:
			if rnq.Predicate == x.Star || rnq.Subject == x.Star {
				return rnq, x.Errorf("If predicate/subject is *, value should be * as well")
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
			if oval == "" && t != types.StringID {
				return rnq, x.Errorf("Invalid ObjectValue")
			}
			src := types.ValueForType(types.StringID)
			src.Value = []byte(oval)
			p, err := types.Convert(src, t)
			if err != nil {
				return rnq, err
			}

			if rnq.ObjectValue, err = types.ObjectValue(t, p.Value); err != nil {
				return rnq, err
			}

		case lex.ItemError:
			return rnq, x.Errorf(item.Val)

		case itemComment:
			isCommentLine = true
			vend = true

		case itemValidEnd:
			vend = true
			if !it.Next() {
				return rnq, x.Errorf("Invalid end of input. Input: [%s]", line)
			}
			// RDF spec says NQuad's should be terminated with a newline. Since we break the input
			// by newline already. We should get EOF or # after dot(.)
			item = it.Item()
			if !(item.Typ == lex.ItemEOF || item.Typ == itemComment) {
				return rnq, x.Errorf("Invalid end of input. Expected newline or # after ."+
					" Input: [%s]", line)
			}
			break L

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
	// We only want to set default value if we have seen ObjectValue within "" and if we didn't
	// already set it.
	if seenOval && rnq.ObjectValue == nil {
		rnq.ObjectValue = &api.Value{&api.Value_DefaultVal{oval}}
	}
	if len(rnq.Subject) == 0 || len(rnq.Predicate) == 0 {
		return rnq, x.Errorf("Empty required fields in NQuad. Input: [%s]", line)
	}
	if len(rnq.ObjectId) == 0 && rnq.ObjectValue == nil {
		return rnq, x.Errorf("No Object in NQuad. Input: [%s]", line)
	}
	if !sane(rnq.Subject) || !sane(rnq.Predicate) ||
		!sane(rnq.ObjectId) || !sane(rnq.Label) {
		return rnq, x.Errorf("NQuad failed sanity check:%+v", rnq)
	}

	return rnq, nil
}

// ConvertToNQuads parses multi line mutation string to a list of NQuads.
func ConvertToNQuads(mutation string) ([]*api.NQuad, error) {
	var nquads []*api.NQuad
	r := strings.NewReader(mutation)
	reader := bufio.NewReader(r)

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
		if len(nq.Predicate) > 0 && nq.Predicate[0] == '_' &&
			nq.Predicate[len(nq.Predicate)-1] == '_' {
			return nil, x.Errorf("Predicates starting and ending with _ are reserved intern.y.")
		}
		if err == ErrEmpty { // special case: comment/empty line
			continue
		} else if err != nil {
			return nquads, x.Wrapf(err, "While parsing RDF: %s", strBuf.String())
		}
		nquads = append(nquads, &nq)
	}
	if err != io.EOF {
		return nquads, err
	}
	return nquads, nil
}

func parseFacets(it *lex.ItemIterator, rnq *api.NQuad) error {
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

	return nil
}

func isNewline(r rune) bool {
	return r == '\n' || r == '\r'
}

var typeMap = map[string]types.TypeID{
	"xs:string":                                        types.StringID,
	"xs:date":                                          types.DateTimeID,
	"xs:dateTime":                                      types.DateTimeID,
	"xs:int":                                           types.IntID,
	"xs:positiveInteger":                               types.IntID,
	"xs:boolean":                                       types.BoolID,
	"xs:double":                                        types.FloatID,
	"xs:float":                                         types.FloatID,
	"xs:base64Binary":                                  types.BinaryID,
	"geo:geojson":                                      types.GeoID,
	"http://www.w3.org/2001/XMLSchema#string":          types.StringID,
	"http://www.w3.org/2001/XMLSchema#dateTime":        types.DateTimeID,
	"http://www.w3.org/2001/XMLSchema#date":            types.DateTimeID,
	"http://www.w3.org/2001/XMLSchema#int":             types.IntID,
	"http://www.w3.org/2001/XMLSchema#positiveInteger": types.IntID,
	"http://www.w3.org/2001/XMLSchema#integer":         types.IntID,
	"http://www.w3.org/2001/XMLSchema#boolean":         types.BoolID,
	"http://www.w3.org/2001/XMLSchema#double":          types.FloatID,
	"http://www.w3.org/2001/XMLSchema#float":           types.FloatID,
	"http://www.w3.org/2001/XMLSchema#gYear":           types.DateTimeID,
	"http://www.w3.org/2001/XMLSchema#gYearMonth":      types.DateTimeID,
}
