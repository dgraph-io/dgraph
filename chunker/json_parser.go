/*
 * Copyright 2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package chunker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/dgraph-io/dgo/v200/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/minio/simdjson-go"
	"github.com/pkg/errors"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func stripSpaces(str string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}

		return r
	}, str)
}

// handleBasicFacetsType parses a facetVal to string/float64/bool/datetime type.
func handleBasicFacetsType(key string, facetVal interface{}) (*api.Facet, error) {
	var jsonValue interface{}
	var valueType api.Facet_ValType
	switch v := facetVal.(type) {
	case string:
		if t, err := types.ParseTime(v); err == nil {
			valueType = api.Facet_DATETIME
			jsonValue = t
		} else {
			facet, err := facets.FacetFor(key, strconv.Quote(v))
			if err != nil {
				return nil, err
			}

			// FacetFor function already converts the value to binary so there is no need
			// for the conversion again after the switch block.
			return facet, nil
		}
	case json.Number:
		number := facetVal.(json.Number)
		if strings.Contains(number.String(), ".") {
			jsonFloat, err := number.Float64()
			if err != nil {
				return nil, err
			}
			jsonValue = jsonFloat
			valueType = api.Facet_FLOAT
		} else {
			jsonInt, err := number.Int64()
			if err != nil {
				return nil, err
			}
			jsonValue = jsonInt
			valueType = api.Facet_INT
		}
	case int64:
		jsonValue = v
		valueType = api.Facet_INT
	case float64:
		jsonValue = v
		valueType = api.Facet_FLOAT
	case bool:
		jsonValue = v
		valueType = api.Facet_BOOL
	default:
		return nil, errors.Errorf("facet value can only be string/number/bool.")
	}

	// Convert facet val interface{} to binary.
	binaryValueFacet, err := facets.ToBinary(key, jsonValue, valueType)
	if err != nil {
		return nil, err
	}

	return binaryValueFacet, nil
}

// parseMapFacets parses facets which are of map type. Facets for scalar list predicates are
// specified in map format. For example below predicate nickname and kind facet associated with it.
// Here nickname "bob" doesn't have any facet associated with it.
// {
//		"nickname": ["alice", "bob", "josh"],
//		"nickname|kind": {
//			"0": "friends",
//			"2": "official"
// 		}
// }
// Parsed response would a slice of maps[int]*api.Facet, one map for each facet.
// Map key would be the index of scalar value for respective facets.
func parseMapFacets(m map[string]interface{}, prefix string) ([]map[int]*api.Facet, error) {
	// This happens at root.
	if prefix == "" {
		return nil, nil
	}

	var mapSlice []map[int]*api.Facet
	for fname, facetVal := range m {
		if facetVal == nil {
			continue
		}
		if !strings.HasPrefix(fname, prefix) {
			continue
		}

		fm, ok := facetVal.(map[string]interface{})
		if !ok {
			return nil, errors.Errorf("facets format should be of type map for "+
				"scalarlist predicates, found: %v for facet: %v", facetVal, fname)
		}

		idxMap := make(map[int]*api.Facet, len(fm))
		for sidx, val := range fm {
			key := fname[len(prefix):]
			facet, err := handleBasicFacetsType(key, val)
			if err != nil {
				return nil, errors.Wrapf(err, "facet: %s, index: %s", fname, sidx)
			}
			idx, err := strconv.Atoi(sidx)
			if err != nil {
				return nil, errors.Wrapf(err, "facet: %s, index: %s", fname, sidx)
			}
			idxMap[idx] = facet
		}
		mapSlice = append(mapSlice, idxMap)
	}

	return mapSlice, nil
}

// parseScalarFacets parses facets which should be of type string/json.Number/bool.
// It returns []*api.Facet, one *api.Facet for each facet.
func parseScalarFacets(m map[string]interface{}, prefix string) ([]*api.Facet, error) {
	// This happens at root.
	if prefix == "" {
		return nil, nil
	}

	var facetsForPred []*api.Facet
	for fname, facetVal := range m {
		if facetVal == nil {
			continue
		}
		if !strings.HasPrefix(fname, prefix) {
			continue
		}

		key := fname[len(prefix):]
		facet, err := handleBasicFacetsType(key, facetVal)
		if err != nil {
			return nil, errors.Wrapf(err, "facet: %s", fname)
		}
		facetsForPred = append(facetsForPred, facet)
	}

	return facetsForPred, nil
}

// This is the response for a map[string]interface{} i.e. a struct.
type mapResponse struct {
	uid  string       // uid retrieved or allocated for the node.
	fcts []*api.Facet // facets on the edge connecting this node to the source if any.
}

func handleBasicType(k string, v interface{}, op int, nq *api.NQuad) error {
	switch v := v.(type) {
	case json.Number:
		if strings.ContainsAny(v.String(), ".Ee") {
			f, err := v.Float64()
			if err != nil {
				return err
			}
			nq.ObjectValue = &api.Value{Val: &api.Value_DoubleVal{DoubleVal: f}}
			return nil
		}
		i, err := v.Int64()
		if err != nil {
			return err
		}
		nq.ObjectValue = &api.Value{Val: &api.Value_IntVal{IntVal: i}}

	case int64:
		if v == 0 && op == DeleteNquads {
			nq.ObjectValue = &api.Value{Val: &api.Value_IntVal{IntVal: v}}
			return nil
		}
		nq.ObjectValue = &api.Value{Val: &api.Value_IntVal{IntVal: v}}

	case string:
		// Default value is considered as S P * deletion.
		if v == "" && op == DeleteNquads {
			nq.ObjectValue = &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}}
			return nil
		}

		// Handle the uid function in upsert block
		s := stripSpaces(v)
		if strings.HasPrefix(s, "uid(") || strings.HasPrefix(s, "val(") {
			if !strings.HasSuffix(s, ")") {
				return errors.Errorf("While processing '%s', brackets are not closed properly", s)
			}
			nq.ObjectId = s
			return nil
		}

		// In RDF, we assume everything is default (types.DefaultID), but in JSON we assume string
		// (StringID). But this value will be checked against the schema so we don't overshadow a
		// password value (types.PasswordID) - Issue#2623
		nq.ObjectValue = &api.Value{Val: &api.Value_StrVal{StrVal: v}}

	case float64:
		if v == 0 && op == DeleteNquads {
			nq.ObjectValue = &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}}
			return nil
		}
		nq.ObjectValue = &api.Value{Val: &api.Value_DoubleVal{DoubleVal: v}}

	case bool:
		if !v && op == DeleteNquads {
			nq.ObjectValue = &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}}
			return nil
		}
		nq.ObjectValue = &api.Value{Val: &api.Value_BoolVal{BoolVal: v}}

	default:
		return errors.Errorf("Unexpected type for val for attr: %s while converting to nquad", k)
	}
	return nil

}

func (buf *NQuadBuffer) checkForDeletion(mr mapResponse, m map[string]interface{}, op int) {
	// Since uid is the only key, this must be S * * deletion.
	if op == DeleteNquads && len(mr.uid) > 0 && len(m) == 1 {
		buf.Push(&api.NQuad{
			Subject:     mr.uid,
			Predicate:   x.Star,
			ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
		})
	}
}

func handleGeoType(val map[string]interface{}, nq *api.NQuad) (bool, error) {
	_, hasType := val["type"]
	_, hasCoordinates := val["coordinates"]
	if len(val) == 2 && hasType && hasCoordinates {
		b, err := json.Marshal(val)
		if err != nil {
			return false, errors.Errorf("Error while trying to parse value: %+v as geo val", val)
		}
		ok, err := tryParseAsGeo(b, nq)
		if err != nil && ok {
			return true, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func tryParseAsGeo(b []byte, nq *api.NQuad) (bool, error) {
	var g geom.T
	err := geojson.Unmarshal(b, &g)
	if err != nil {
		return false, nil
	}

	geo, err := types.ObjectValue(types.GeoID, g)
	if err != nil {
		return false, errors.Errorf("Couldn't convert value: %s to geo type", string(b))
	}

	nq.ObjectValue = geo
	return true, nil
}

// NQuadBuffer batches up batchSize NQuads per push to channel, accessible via Ch(). If batchSize is
// negative, it only does one push to Ch() during Flush.
type NQuadBuffer struct {
	batchSize int
	nquads    []*api.NQuad
	nqCh      chan []*api.NQuad
	predHints map[string]pb.Metadata_HintType
}

// NewNQuadBuffer returns a new NQuadBuffer instance with the specified batch size.
func NewNQuadBuffer(batchSize int) *NQuadBuffer {
	buf := &NQuadBuffer{
		batchSize: batchSize,
		nqCh:      make(chan []*api.NQuad, 10),
	}
	if buf.batchSize > 0 {
		buf.nquads = make([]*api.NQuad, 0, batchSize)
	}
	buf.predHints = make(map[string]pb.Metadata_HintType)
	return buf
}

// Ch returns a channel containing slices of NQuads which can be consumed by the caller.
func (buf *NQuadBuffer) Ch() <-chan []*api.NQuad {
	return buf.nqCh
}

// Push can be passed one or more NQuad pointers, which get pushed to the buffer.
func (buf *NQuadBuffer) Push(nqs ...*api.NQuad) {
	for _, nq := range nqs {
		buf.nquads = append(buf.nquads, nq)
		if buf.batchSize > 0 && len(buf.nquads) >= buf.batchSize {
			buf.nqCh <- buf.nquads
			buf.nquads = make([]*api.NQuad, 0, buf.batchSize)
		}
	}
}

// Metadata returns the parse metadata that has been aggregated so far..
func (buf *NQuadBuffer) Metadata() *pb.Metadata {
	return &pb.Metadata{
		PredHints: buf.predHints,
	}
}

// PushPredHint pushes and aggregates hints about the type of the predicate derived
// during the parsing. This  metadata is expected to be a lot smaller than the set of
// NQuads so it's not  necessary to send them in batches.
func (buf *NQuadBuffer) PushPredHint(pred string, hint pb.Metadata_HintType) {
	if oldHint, ok := buf.predHints[pred]; ok && hint != oldHint {
		hint = pb.Metadata_LIST
	}
	buf.predHints[pred] = hint
}

// Flush must be called at the end to push out all the buffered NQuads to the channel. Once Flush is
// called, this instance of NQuadBuffer should no longer be used.
func (buf *NQuadBuffer) Flush() {
	if len(buf.nquads) > 0 {
		buf.nqCh <- buf.nquads
		buf.nquads = nil
	}
	close(buf.nqCh)
}

// nextIdx is the index that is used to generate blank node ids for a json map object
// when the map object does not have a "uid" field.
// It should only be accessed through the atomic APIs.
var nextIdx uint64

// randomID will be used to generate blank node ids.
// We use a random number to avoid collision with user specified uids.
var randomID uint32

func init() {
	randomID = rand.Uint32()
}

func getNextBlank() string {
	id := atomic.AddUint64(&nextIdx, 1)
	return fmt.Sprintf("_:dg.%d.%d", randomID, id)
}

// TODO - Abstract these parameters to a struct.
func (buf *NQuadBuffer) mapToNquads(m map[string]interface{}, op int, parentPred string) (
	mapResponse, error) {
	var mr mapResponse

	// move all facets from global map to smaller mf map
	mf := make(map[string]interface{})
	for k, v := range m {
		if strings.Contains(k, x.FacetDelimeter) {
			mf[k] = v
			delete(m, k)
		}
	}

	// Check field in map.
	if uidVal, ok := m["uid"]; ok {
		var uid uint64

		switch uidVal := uidVal.(type) {
		case json.Number:
			ui, err := uidVal.Int64()
			if err != nil {
				return mr, err
			}
			uid = uint64(ui)

		case int64:
			uid = uint64(uidVal)

		case string:
			s := stripSpaces(uidVal)
			if len(uidVal) == 0 {
				uid = 0
			} else if ok := strings.HasPrefix(uidVal, "_:"); ok {
				mr.uid = uidVal
			} else if ok := strings.HasPrefix(s, "uid("); ok {
				mr.uid = s
			} else if u, err := strconv.ParseUint(uidVal, 0, 64); err == nil {
				uid = u
			} else {
				return mr, err
			}
		}
		if uid > 0 {
			mr.uid = fmt.Sprintf("%d", uid)
		}
	}

	if len(mr.uid) == 0 {
		if op == DeleteNquads {
			// Delete operations with a non-nil value must have a uid specified.
			return mr, errors.Errorf("UID must be present and non-zero while deleting edges.")
		}
		mr.uid = getNextBlank()
	}

	for pred, v := range m {
		// We have already extracted the uid above so we skip that edge.
		// v can be nil if user didn't set a value and if omitEmpty was not supplied as JSON
		// option.
		// We also skip facets here because we parse them with the corresponding predicate.
		if pred == "uid" {
			continue
		}

		if v == nil {
			if op == DeleteNquads {
				// This corresponds to edge deletion.
				nq := &api.NQuad{
					Subject:     mr.uid,
					Predicate:   pred,
					ObjectValue: &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}},
				}
				// Here we split predicate and lang directive (ex: "name@en"), if needed. With JSON
				// mutations that's the only way to send language for a value.
				nq.Predicate, nq.Lang = x.PredicateLang(nq.Predicate)
				buf.Push(nq)
				continue
			}

			// If op is SetNquads, ignore this triplet and continue.
			continue
		}

		nq := api.NQuad{
			Subject:   mr.uid,
			Predicate: pred,
		}

		prefix := pred + x.FacetDelimeter
		if _, ok := v.([]interface{}); !ok {
			fts, err := parseScalarFacets(mf, prefix)
			if err != nil {
				return mr, err
			}
			nq.Facets = fts
		}

		// Here we split predicate and lang directive (ex: "name@en"), if needed. With JSON
		// mutations that's the only way to send language for a value.
		nq.Predicate, nq.Lang = x.PredicateLang(nq.Predicate)

		switch v := v.(type) {
		case int64, float64:
			if err := handleBasicType(pred, v, op, &nq); err != nil {
				return mr, err
			}
			buf.Push(&nq)
			buf.PushPredHint(pred, pb.Metadata_SINGLE)
		case string, json.Number, bool:
			if err := handleBasicType(pred, v, op, &nq); err != nil {
				return mr, err
			}
			buf.Push(&nq)
			buf.PushPredHint(pred, pb.Metadata_SINGLE)
		case map[string]interface{}:
			if len(v) == 0 {
				continue
			}

			ok, err := handleGeoType(v, &nq)
			if err != nil {
				return mr, err
			}
			if ok {
				buf.Push(&nq)
				buf.PushPredHint(pred, pb.Metadata_SINGLE)
				continue
			}

			cr, err := buf.mapToNquads(v, op, pred)
			if err != nil {
				return mr, err
			}

			// Add the connecting edge beteween the entities.
			nq.ObjectId = cr.uid
			nq.Facets = cr.fcts
			buf.Push(&nq)
			buf.PushPredHint(pred, pb.Metadata_SINGLE)
		case []interface{}:
			buf.PushPredHint(pred, pb.Metadata_LIST)
			// TODO(Ashish): We need to call this only in case of scalarlist, for other lists
			// this can be avoided.
			facetsMapSlice, err := parseMapFacets(mf, prefix)
			if err != nil {
				return mr, err
			}

			for idx, item := range v {
				nq := api.NQuad{
					Subject:   mr.uid,
					Predicate: pred,
				}

				switch iv := item.(type) {
				case string, float64, json.Number, int64:
					if err := handleBasicType(pred, iv, op, &nq); err != nil {
						return mr, err
					}
					// Here populate facets from facetsMapSlice. Each map has mapping for single
					// facet from item(one of predicate value) idx to *api.Facet.
					// {
					// 	"friend": ["Joshua", "David", "Josh"],
					// 	"friend|from": {
					// 		"0": "school"
					// 	},
					// 	"friend|age": {
					// 		"1": 20
					// 	}
					// }
					// facetMapSlice looks like below. First map is for friend|from facet and second
					// map is for friend|age facet.
					// [
					// 		map[int]*api.Facet{
					//			0: *api.Facet
					// 		},
					// 		map[int]*api.Facet{
					//			1: *api.Facet
					// 		}
					// ]
					var fts []*api.Facet
					for _, fm := range facetsMapSlice {
						if ft, ok := fm[idx]; ok {
							fts = append(fts, ft)
						}
					}
					nq.Facets = fts
					buf.Push(&nq)
				case map[string]interface{}:
					// map[string]interface{} can mean geojson or a connecting entity.
					ok, err := handleGeoType(item.(map[string]interface{}), &nq)
					if err != nil {
						return mr, err
					}
					if ok {
						buf.Push(&nq)
						continue
					}

					cr, err := buf.mapToNquads(iv, op, pred)
					if err != nil {
						return mr, err
					}
					nq.ObjectId = cr.uid
					nq.Facets = cr.fcts
					buf.Push(&nq)
				default:
					return mr,
						errors.Errorf("Got unsupported type for list: %s", pred)
				}
			}
		default:
			return mr, errors.Errorf("Unexpected type for val for attr: %s while converting to nquad", pred)
		}
	}

	fts, err := parseScalarFacets(mf, parentPred+x.FacetDelimeter)
	mr.fcts = fts

	return mr, err
}

const (
	// SetNquads is the constant used to indicate that the parsed NQuads are meant to be added.
	SetNquads = iota
	// DeleteNquads is the constant used to indicate that the parsed NQuads are meant to be
	// deleted.
	DeleteNquads
)

// ParseJSON parses the given byte slice and pushes the parsed NQuads into the buffer.
func (buf *NQuadBuffer) ParseJSON(b []byte, op int) error {
	buffer := bytes.NewBuffer(b)
	dec := json.NewDecoder(buffer)
	dec.UseNumber()
	ms := make(map[string]interface{})
	var list []interface{}
	if err := dec.Decode(&ms); err != nil {
		// Couldn't parse as map, lets try to parse it as a list.
		buffer.Reset() // The previous contents are used. Reset here.
		// Rewrite b into buffer, so it can be consumed.
		if _, err := buffer.Write(b); err != nil {
			return err
		}
		if err = dec.Decode(&list); err != nil {
			return err
		}
	}
	if len(list) == 0 && len(ms) == 0 {
		return nil
	}
	if len(list) > 0 {
		for _, obj := range list {
			if _, ok := obj.(map[string]interface{}); !ok {
				return errors.Errorf("Only array of map allowed at root.")
			}
			mr, err := buf.mapToNquads(obj.(map[string]interface{}), op, "")
			if err != nil {
				return err
			}
			buf.checkForDeletion(mr, obj.(map[string]interface{}), op)
		}
		return nil
	}
	mr, err := buf.mapToNquads(ms, op, "")
	buf.checkForDeletion(mr, ms, op)
	return err
}

// ParseJSON is a convenience wrapper function to get all NQuads in one call. This can however, lead
// to high memory usage. So be careful using this.
func ParseJSON(b []byte, op int) ([]*api.NQuad, *pb.Metadata, error) {
	buf := NewNQuadBuffer(-1)
	err := buf.ParseJSON(b, op)
	if err != nil {
		return nil, nil, err
	}
	buf.Flush()
	nqs := <-buf.Ch()
	metadata := buf.Metadata()
	return nqs, metadata, nil
}

// FastParseJSON currently parses NQuads about 230% faster than ParseJSON.
//
// NOTE: FastParseJSON uses simdjson which has "minor floating point number
//       imprecisions"
func (buf *NQuadBuffer) FastParseJSON(b []byte, op int) error {
	if !simdjson.SupportedCPU() {
		return errors.New("CPU doesn't support simdjson for fast parsing")
	}
	parser := NewParser(op)
	if err := parser.Run(b); err != nil {
		return err
	}
	buf.nquads = parser.Quads
	return nil
}

func NewNQuad() *api.NQuad {
	return &api.NQuad{
		Facets: make([]*api.Facet, 0),
	}
}

type ParserState func(byte) (ParserState, error)

type Parser struct {
	Op           int
	Cursor       uint64
	StringCursor uint64
	Quad         *api.NQuad
	Facet        *api.Facet
	Quads        []*api.NQuad
	Levels       *ParserLevels
	Parsed       *simdjson.ParsedJson
	FacetPred    string
	FacetId      uint64
	FacetEnd     uint64
	Iter         simdjson.Iter
}

func NewParser(op int) *Parser {
	return &Parser{
		Op:     op,
		Cursor: 1,
		Quad:   NewNQuad(),
		Quads:  make([]*api.NQuad, 0),
		Levels: NewParserLevels(),
		Facet:  &api.Facet{},
	}
}

func (p *Parser) Run(d []byte) (err error) {
	if p.Parsed, err = simdjson.Parse(d, nil); err != nil {
		return
	}
	p.Iter = p.Parsed.Iter()
	for state := p.Root; state != nil; p.Cursor++ {
		if p.Cursor >= uint64(len(p.Parsed.Tape)) {
			return
		}
		p.Iter.AdvanceInto()
		//t := p.Iter.AdvanceInto()
		//fmt.Printf("%v %d %c\n", t, p.Cursor, p.Parsed.Tape[p.Cursor]>>56)
		if state, err = state(byte(p.Parsed.Tape[p.Cursor] >> 56)); err != nil {
			return
		}
	}
	return
}

// String is called when we encounter a '"' (string) node and want to get the
// value from the string buffer. In the simdjson Tape, the string length is
// immediately after the '"' node, so we first have to increment the Cursor
// by one and then we use the Tape value as the string length, and create
// a byte slice from the string buffer.
func (p *Parser) String() string {
	p.Cursor++
	length := p.Parsed.Tape[p.Cursor]
	s := p.Parsed.Strings[p.StringCursor : p.StringCursor+length]
	p.StringCursor += length
	return string(s)
}

// Root is the initial state of the Parser. It should only look for '{' or '['
// nodes, anything else is bad JSON.
func (p *Parser) Root(n byte) (ParserState, error) {
	switch n {
	case '{':
		p.Levels.Deeper(false)
		return p.Object, nil
	case '[':
		p.Levels.Deeper(true)
		return p.Array, nil
	}
	return nil, nil
}

// Object is the most common state for the Parser to be in--we're usually in an
// object of some kind.
func (p *Parser) Object(n byte) (ParserState, error) {
	switch n {
	case '{':
		p.Levels.Deeper(false)
		return p.Object, nil
	case '}':
		l := p.Levels.Get(0)
		// check if the current level has anything waiting to be pushed, if the
		// current level is scalars we don't push anything
		if l.Wait != nil && !l.Scalars {
			p.Quad = l.Wait
			p.Quad.ObjectId = l.Subject
			p.Quads = append(p.Quads, p.Quad)
			p.Quad = NewNQuad()
		} else {
			if p.Levels.InArray() {
				a := p.Levels.Get(1)
				if a.Array && a.Wait != nil && !a.Scalars {
					p.Quad.Subject = a.Wait.Subject
					p.Quad.Predicate = a.Wait.Predicate
					p.Quad.ObjectId = l.Subject
					p.Quad.Facets = a.Wait.Facets
					p.Quads = append(p.Quads, p.Quad)
					p.Quad = NewNQuad()
				}
			}
		}
		l = p.Levels.Pop()
		if !l.FoundUid && p.Op == DeleteNquads {
			return nil, errors.New("UID must be present and non-zero while deleting edges.")
		}
		p.Quads = append(p.Quads, l.Quads...)
		return p.Object, nil
	case ']':
		l := p.Levels.Pop()
		p.Quads = append(p.Quads, l.Quads...)
		return p.Object, nil
	case '"':
		s := p.String()
		if s == "uid" {
			return p.Uid, nil
		}
		// check if this is a facet definition
		if strings.Contains(s, "|") {
			e := strings.Split(s, "|")
			if len(e) == 2 {
				p.FacetPred = e[0]
				p.Facet.Key = e[1]
				// peek at the next node to see if it's a scalar facet or map
				next := byte(p.Parsed.Tape[p.Cursor+1] >> 56)
				if next == '{' {
					// go into the object so MapFacet can immediately check the
					// keys
					p.Cursor++
					p.Iter.AdvanceInto()
					return p.MapFacet, nil
				} else if next == 'n' {
					p.Cursor++
					p.Iter.AdvanceInto()
					return p.Object, nil
				} else if next == '[' {
					return nil, errors.New("facet values should not be list")
				}
				return p.ScalarFacet, nil
			}
		} else {
			// found a normal nquad
			p.Quad.Subject = p.Levels.Subject()
			p.Quad.Predicate = s
			return p.Value, nil
		}
		// not sure what this string is, try again
		return p.Object, nil
	}
	return nil, nil
}

func (p *Parser) MapFacet(n byte) (ParserState, error) {
	// map facet keys must be (numerical) strings
	if n != '"' {
		return p.Object, nil
	}
	id, err := strconv.ParseUint(p.String(), 10, 64)
	if err != nil {
		return nil, err
	}
	p.FacetId = id
	return p.MapFacetVal, nil
}

func (p *Parser) MapFacetVal(n byte) (ParserState, error) {
	// getFacet fills the p.Facet struct
	if err := p.getFacet(n); err != nil {
		return nil, err
	}
	// TODO: move this to a cache so we only have to grab referenced quads once
	//       per facet map definition, rather than for each index-value
	//
	// find every quad that could be referenced by the facet
	l := p.Levels.Get(0)
	quads := make([]*api.NQuad, 0)
	for i := len(l.Quads) - 1; i >= 0; i-- {
		if l.Quads[i].Predicate == p.FacetPred {
			quads = append(quads, l.Quads[i])
		}
	}
	if p.FacetId > uint64(len(quads)) {
		p.Facet = &api.Facet{}
		return p.MapFacet, nil
	}
	if len(quads) == 0 {
		p.Facet = &api.Facet{}
		return p.MapFacet, nil
	}
	if len(quads) == 1 {
		if quads[0].ObjectValue.Size() == 0 {
			p.Facet = &api.Facet{}
			return p.MapFacet, nil
		}
		return nil, errors.New("predicate should be list")
	}
	for i := uint64(len(quads) - 1); i >= 0; i-- {
		if i == uint64(len(quads)-1)-p.FacetId {
			quads[i].Facets = append(quads[i].Facets, p.Facet)
			p.Facet = &api.Facet{}
			return p.MapFacet, nil
		}
	}
	return p.MapFacet, nil
}

func (p *Parser) ScalarFacet(n byte) (ParserState, error) {
	// getFacet fills the p.Facet struct
	if err := p.getFacet(n); err != nil {
		return nil, err
	}
	// because this is a scalar facet and you can reference parent quads, we
	// first have to check if any of the quads waiting on a Level match the
	// facet predicate
	if p.Levels.FoundScalarFacet(p.FacetPred, p.Facet) {
		return p.Object, nil
	}
	// we didn't find the predicate waiting on a Level, so go through quads
	// in reverse order (it's most likely that the referenced quad is near
	// the end of the l.Quads slice)
	l := p.Levels.Get(0)
	for i := len(l.Quads) - 1; i >= 0; i-- {
		if l.Quads[i].Predicate == p.FacetPred {
			if i != 0 && l.Quads[i-1].Predicate == p.FacetPred {
				return nil, errors.New("scalar facet should be map")
			}
			l.Quads[i].Facets = append(l.Quads[i].Facets, p.Facet)
			p.Facet = &api.Facet{}
			return p.Object, nil
		}
	}
	return p.Object, nil
}

func (p *Parser) Array(n byte) (ParserState, error) {
	// get current array level
	a := p.Levels.Get(0)
	// if this is a scalar array (we won't know until we get to the switch
	// statement and set a.Scalars = true) we'll need the waiting info so we can
	// generate a quad for each scalar
	//
	// for example:
	//
	//     "friend": ["karl", "megan", "sarah"]
	//
	// will generate three quads each with the same subject (unknown) and
	// predicate ("friend")
	if a.Wait != nil {
		p.Quad.Subject = a.Wait.Subject
		p.Quad.Predicate = a.Wait.Predicate
	}
	switch n {
	case '{':
		if p.isGeo() {
			l := p.Levels.Deeper(false)
			l.Wait = p.Quad
			p.Quad = NewNQuad()
			if err := p.getGeoValue(); err != nil {
				return nil, err
			}
			p.Levels.Pop()
			return p.Object, nil
		}
		p.Levels.Deeper(false)
		return p.Object, nil
	case '}':
		return p.Object, nil
	case '[':
		p.Levels.Deeper(false)
		return p.Array, nil
	case ']':
		// return to Object rather than Array because it's the default state
		return p.Object, nil
	case '"', 'l', 'u', 'd', 't', 'f', 'n':
		a.Scalars = true
		p.getScalarValue(n)
	}
	return p.Array, nil
}

func (p *Parser) Value(n byte) (ParserState, error) {
	switch n {
	case '{':
		if p.isGeo() {
			l := p.Levels.Deeper(false)
			l.Wait = p.Quad
			p.Quad = NewNQuad()
			if err := p.getGeoValue(); err != nil {
				return nil, err
			}
			p.Levels.Pop()
			return p.Object, nil
		}
		return p.openValueLevel('}', false, p.Object), nil
	case '[':
		return p.openValueLevel(']', true, p.Array), nil
	case '"', 'l', 'u', 'd', 't', 'f', 'n':
		if err := p.getScalarValue(n); err != nil {
			return nil, err
		}
	}
	return p.Object, nil
}

// Uid is called when a "uid" string is encountered within Object. Its only job
// is to set the uid on the current (top) Level.
func (p *Parser) Uid(n byte) (ParserState, error) {
	switch n {
	case 'l':
		p.Cursor++
		n := int64(p.Parsed.Tape[p.Cursor])
		if n < 0 {
			return nil, errors.Errorf("expected positive uid number, instead found: %c\n", n)
		}
		p.Levels.FoundSubject(fmt.Sprintf("%d", n))
	case 'u':
		p.Cursor++
		p.Levels.FoundSubject(fmt.Sprintf("%d", p.Parsed.Tape[p.Cursor]))
	case '"':
		uid := uint64(0)
		uidVal := p.String()
		s := stripSpaces(uidVal)
		if len(uidVal) == 0 {
			uid = 0
		} else if ok := strings.HasPrefix(uidVal, "-"); ok {
			return nil, errors.New("negative uid")
		} else if ok := strings.HasPrefix(uidVal, "0x"); ok {
			u, err := strconv.ParseUint(uidVal[2:], 16, 64)
			if err != nil {
				return nil, err
			}
			uid = u
		} else if ok := strings.HasPrefix(uidVal, "_:"); ok {
			p.Levels.FoundSubject(uidVal)
			return p.Object, nil
		} else if ok := strings.HasPrefix(s, "uid("); ok {
			p.Levels.FoundSubject(s)
			return p.Object, nil
		} else if u, err := strconv.ParseUint(uidVal, 0, 64); err == nil {
			uid = u
		}
		if uid > 0 {
			p.Levels.FoundSubject(fmt.Sprintf("%d", uid))
		}
	default:
		return nil, errors.Errorf("expected uid string or number, instead found: %c\n", n)
	}
	return p.Object, nil
}

// openValueLevel is used by Value when a non-scalar value is found.
func (p *Parser) openValueLevel(closing byte, array bool, next ParserState) ParserState {
	// peek the next node to see if it's an empty object or array
	if byte(p.Parsed.Tape[p.Cursor+1]>>56) == closing {
		// it is an empty {} or [], so skip past it
		p.Cursor++
		p.Iter.AdvanceInto()
		// we always return to Object even if array = true because it's the
		// default state where most of the work gets done
		return p.Object
	}
	// add a new level to the stack
	l := p.Levels.Deeper(array)
	// the current quad is waiting until the object is done being parsed because
	// we have to wait until we find/generate a uid
	l.Wait = p.Quad
	p.Quad = NewNQuad()
	// either return to Object or Array, depending on the type
	return next
}

// getScalarValue is used by Value and Array
func (p *Parser) getScalarValue(n byte) error {
	switch n {
	case '"':
		// default value is considered as S P * deletion
		s := p.String()
		if s == "" && p.Op == DeleteNquads {
			p.Quad.ObjectValue = &api.Value{Val: &api.Value_DefaultVal{x.Star}}
			break
		}
		// handle uid function in upsert block
		s = stripSpaces(s)
		if strings.HasPrefix(s, "uid(") || strings.HasPrefix(s, "val(") {
			if !strings.HasSuffix(s, ")") {
				return errors.Errorf(
					"While processing '%s', brackets are not closed properly", s)
			}
			p.Quad.ObjectId = s
			break
		}
		// normal string value
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_StrVal{s}}
	case 'l', 'u':
		p.Cursor++
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_IntVal{int64(p.Parsed.Tape[p.Cursor])}}
	case 'd':
		p.Cursor++
		n := math.Float64frombits(p.Parsed.Tape[p.Cursor])
		// default value is considered as S P * deletion
		if n == 0 && p.Op == DeleteNquads {
			p.Quad.ObjectValue = &api.Value{Val: &api.Value_DefaultVal{x.Star}}
			break
		}
		// normal float value
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_DoubleVal{n}}
	case 't':
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_BoolVal{true}}
	case 'f':
		// default value is considered as S P * deletion
		if p.Op == DeleteNquads {
			p.Quad.ObjectValue = &api.Value{Val: &api.Value_DefaultVal{x.Star}}
			break
		}
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_BoolVal{false}}
	case 'n':
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_BytesVal{nil}}
	}
	l := p.Levels.Get(0)
	l.Quads = append(l.Quads, p.Quad)
	p.Quad = NewNQuad()
	return nil
}

func (p *Parser) getFacet(n byte) error {
	var err error
	var val interface{}
	switch n {
	case '"':
		s := p.String()
		t, err := types.ParseTime(s)
		if err == nil {
			p.Facet.ValType = api.Facet_DATETIME
			val = t
		} else {
			if p.Facet, err = facets.FacetFor(p.Facet.Key, strconv.Quote(s)); err != nil {
				return err
			}
			return nil
		}
	case 'l', 'u', 'd', 't', 'f', 'n':
		val = p.getFacetValue(n)
	}
	if val == nil {
		return errors.New("only scalar values in facet map")
	}
	if p.Facet, err = facets.ToBinary(p.Facet.Key, val, p.Facet.ValType); err != nil {
		return err
	}
	return nil
}

func (p *Parser) getFacetValue(n byte) interface{} {
	var val interface{}
	switch n {
	case 'u':
		// NOTE: dgraph doesn't have uint64 facet type, so we just convert it to
		//       int64
		fallthrough
	case 'l':
		p.Facet.ValType = api.Facet_INT
		p.Cursor++
		val = int64(p.Parsed.Tape[p.Cursor])
	case 'd':
		p.Facet.ValType = api.Facet_FLOAT
		p.Cursor++
		val = math.Float64frombits(p.Parsed.Tape[p.Cursor])
	case 't':
		p.Facet.ValType = api.Facet_BOOL
		val = true
	case 'f':
		p.Facet.ValType = api.Facet_BOOL
		val = false
	// TODO: can facets have null values?
	case 'n':
	}
	return val
}

// TODO: allow "type" definition to be anywhere in the object, not just first
func (p *Parser) isGeo() bool {
	if uint64(len(p.Parsed.Tape))-p.Cursor < 3 {
		return false
	}
	if byte(p.Parsed.Tape[p.Cursor+1]>>56) != '"' {
		return false
	}
	if byte(p.Parsed.Tape[p.Cursor+3]>>56) != '"' {
		return false
	}
	totalStringSize := uint64(0)
	p.Cursor++
	maybeGeoType := p.String()
	totalStringSize += uint64(len(maybeGeoType))
	if maybeGeoType != "type" {
		p.Cursor -= 2
		p.StringCursor -= uint64(len(maybeGeoType))
		return false
	}
	p.Cursor++
	maybeGeoType = p.String()
	totalStringSize += uint64(len(maybeGeoType))
	switch maybeGeoType {
	case "Point", "MultiPoint":
	case "LineString", "MultiLineString":
	case "Polygon", "MultiPolygon":
	case "GeometryCollection":
	default:
		p.Cursor -= 2
		p.StringCursor -= uint64(len(maybeGeoType))
		return false
	}
	p.Cursor -= 4
	p.StringCursor -= totalStringSize
	return true
}

func (p *Parser) getGeoValue() error {
	// skip over the geo object
	next := uint64(((p.Parsed.Tape[p.Cursor] << 8) >> 8) - 1)
	stringSize := uint64(0)
	for i := p.Cursor; i < next; i++ {
		c := byte(p.Parsed.Tape[i] >> 56)
		if c == '"' {
			stringSize += p.Parsed.Tape[i+1]
		}
		if c == '"' || c == 'l' || c == 'u' || c == 'd' {
			i++
		}
	}
	// adjust both cursors to the end of this object
	p.StringCursor += stringSize
	p.Cursor = next
	// get an iterator only containing the geo object
	var geoIter simdjson.Iter
	if _, err := p.Iter.AdvanceIter(&geoIter); err != nil {
		return err
	}
	// convert the geo object into json bytes
	object, err := geoIter.MarshalJSON()
	if err != nil {
		return err
	}
	var geoStruct geom.T
	if err = geojson.Unmarshal(object, &geoStruct); err != nil {
		return err
	}
	var geoVal *api.Value
	if geoVal, err = types.ObjectValue(types.GeoID, geoStruct); err != nil {
		return err
	}
	l := p.Levels.Get(0)
	l.Wait.ObjectValue = geoVal
	p.Quads = append(p.Quads, l.Wait)
	//l.Quads = append(l.Quads, l.Wait)
	return nil
}

type ParserLevels struct {
	Levels []*ParserLevel
}

type ParserLevel struct {
	Array    bool
	FoundUid bool
	Subject  string
	Scalars  bool
	Wait     *api.NQuad
	Quads    []*api.NQuad
}

func NewParserLevels() *ParserLevels {
	return &ParserLevels{
		Levels: make([]*ParserLevel, 0),
	}
}

func (p *ParserLevels) FoundScalarFacet(predicate string, facet *api.Facet) bool {
	for i := len(p.Levels) - 1; i >= 0; i-- {
		if p.Levels[i].Scalars {
			continue
		}
		if p.Levels[i].Wait != nil && p.Levels[i].Wait.Predicate == predicate {
			p.Levels[i].Wait.Facets = append(p.Levels[i].Wait.Facets, facet)
			return true
		}
	}
	return false
}

func (p *ParserLevels) Pop() *ParserLevel {
	if len(p.Levels) == 0 {
		return nil
	}
	l := p.Levels[len(p.Levels)-1]
	p.Levels = p.Levels[:len(p.Levels)-1]
	return l
}

func (p *ParserLevels) Get(n int) *ParserLevel {
	if len(p.Levels) <= n {
		return nil
	}
	return p.Levels[len(p.Levels)-1-n]
}

func (p *ParserLevels) InArray() bool {
	if len(p.Levels) < 2 {
		return false
	}
	return p.Levels[len(p.Levels)-2].Array
}

// Deeper is called when we encounter a '{' or '[' node and are going "deeper"
// into the nested JSON objects. It's important to set the 'array' param to
// true when we encounter '[' nodes because we only want to increment the
// global Subject counter for objects.
func (p *ParserLevels) Deeper(array bool) *ParserLevel {
	var subject string
	if !array {
		// TODO: use dgraph prefix and random number
		subject = getNextBlank()
	}
	level := &ParserLevel{
		Array:   array,
		Subject: subject,
		Quads:   make([]*api.NQuad, 0),
	}
	p.Levels = append(p.Levels, level)
	return level
}

// Subject returns the current subject based on how deeply nested we are. We
// iterate through the Levels in reverse order (it's a stack) to find a
// non-array Level with a subject.
func (p *ParserLevels) Subject() string {
	for i := len(p.Levels) - 1; i >= 0; i-- {
		if !p.Levels[i].Array {
			return p.Levels[i].Subject
		}
	}
	return ""
}

// FoundSubject is called when the Parser is in the Uid state and finds a valid
// uid.
func (p *ParserLevels) FoundSubject(s string) {
	l := p.Levels[len(p.Levels)-1]
	l.Subject = s
	l.FoundUid = true
	// TODO: verify
	for _, quad := range l.Quads {
		quad.Subject = l.Subject
	}
}
