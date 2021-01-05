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
	simdjson "github.com/minio/simdjson-go"
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
	quads, err := NewParser(false).Parse(b)
	if err != nil {
		return err
	}
	buf.nquads = quads
	return nil
}

type ParserState uint8

const (
	NONE ParserState = iota
	PREDICATE
	SCALAR
	OBJECT
	ARRAY
	UID
	GEO
)

func (s ParserState) String() string {
	switch s {
	case NONE:
		return "NONE"
	case PREDICATE:
		return "PREDICATE"
	case SCALAR:
		return "SCALAR"
	case OBJECT:
		return "OBJECT"
	case ARRAY:
		return "ARRAY"
	case UID:
		return "UID"
	case GEO:
		return "GEO"
	}
	return "?"
}

type (
	ParserQueue struct {
		Waiting []*QueueQuad
	}
	QueueQuad struct {
		Type ParserState
		Quad *api.NQuad
	}
)

func NewParserQueue() *ParserQueue {
	return &ParserQueue{
		Waiting: make([]*QueueQuad, 0),
	}
}

func (q *ParserQueue) Recent(t ParserState) bool {
	return q.Waiting[len(q.Waiting)-1].Type == t
}

func (q *ParserQueue) Pop(t ParserState) *api.NQuad {
	waiting := q.Waiting[len(q.Waiting)-1]
	if waiting.Type != t {
		return nil
	}
	q.Waiting = q.Waiting[:len(q.Waiting)-1]
	return waiting.Quad
}

func (q *ParserQueue) Add(t ParserState, quad *api.NQuad) {
	q.Waiting = append(q.Waiting, &QueueQuad{
		Type: t,
		Quad: quad,
	})
}

func (q *ParserQueue) Empty() bool {
	return len(q.Waiting) == 0
}

type (
	Depth struct {
		Levels []*DepthLevel
	}
	DepthLevel struct {
		Type   ParserState
		Uids   []string
		Uid    string
		Closes uint64
	}
)

func NewDepthLevel(t ParserState, counter, closes uint64) *DepthLevel {
	return &DepthLevel{
		Type:   t,
		Uids:   make([]string, 0),
		Uid:    fmt.Sprintf("_:dg.%d.%d", randomID, counter),
		Closes: closes,
	}
}

func (l *DepthLevel) Subject() string {
	if len(l.Uids) == 0 {
		return l.Uid
	}
	return l.Uids[len(l.Uids)-1]
}

func NewDepth() *Depth {
	return &Depth{
		Levels: make([]*DepthLevel, 0),
	}
}

func (d *Depth) Closes() uint64 {
	if len(d.Levels) < 1 {
		return 0
	}
	return d.Levels[len(d.Levels)-1].Closes
}

func (d *Depth) ArrayObject() bool {
	if len(d.Levels) < 2 {
		return false
	}
	return d.Levels[len(d.Levels)-2].Type == ARRAY
}

func (d *Depth) ArrayUid(uid string) {
	if len(d.Levels) < 2 {
		return
	}
	array := d.Levels[len(d.Levels)-2]
	array.Uids = append(array.Uids, uid)
}

func (d *Depth) Uid(uid string) {
	curr := d.Levels[len(d.Levels)-1]
	curr.Uids = append(curr.Uids, uid)
}

func (d *Depth) Subject() string {
	return d.Levels[len(d.Levels)-1].Subject()
}

func (d *Depth) Increase(t ParserState, closes uint64) {
	if t == OBJECT {
		atomic.AddUint64(&nextIdx, 1)
	}
	d.Levels = append(d.Levels, NewDepthLevel(t, nextIdx, closes))
}

func (d *Depth) Decrease(t ParserState) *DepthLevel {
	top := d.Levels[len(d.Levels)-1]
	d.Levels = d.Levels[:len(d.Levels)-1]
	return top
}

type Parser struct {
	State  ParserState
	Parsed *simdjson.ParsedJson
	Quads  []*api.NQuad
	Queue  *ParserQueue
	Depth  *Depth
	Quad   *api.NQuad
	Skip   bool
	Logs   bool

	stringOffset uint64
}

func NewParser(logs bool) *Parser {
	return &Parser{
		State: NONE,
		Quads: make([]*api.NQuad, 0),
		Quad:  &api.NQuad{},
		Queue: NewParserQueue(),
		Depth: NewDepth(),
		Logs:  logs,
	}
}

func (p *Parser) Parse(d []byte) ([]*api.NQuad, error) {
	var err error
	if p.Parsed, err = simdjson.Parse(d, nil); err != nil {
		return nil, err
	}
	return p.Quads, p.Walk()
}

func (p *Parser) String(l uint64) string {
	s := string(p.Parsed.Strings[p.stringOffset : p.stringOffset+l])
	p.stringOffset += l
	return s
}

func (p *Parser) Log(i uint64, c uint64, n byte) {
	if p.Logs {
		switch byte(c >> 56) {
		case 'r', 'n', 't', 'f', 'l', 'u', 'd', '"', '[', ']', '{', '}':
			fmt.Printf("%2d: %c %c %s\n", i, byte(c>>56), n, p.State)
		default:
		}
	}
}

func (p *Parser) LogMore(s string) {
	if p.Logs {
		fmt.Printf("\n        %s\n\n", s)
	}
}

func (p *Parser) Walk() (err error) {
	n := byte('n')

	for i := uint64(0); i < uint64(len(p.Parsed.Tape))-1; i++ {
		// c is the current node on the tape
		c := p.Parsed.Tape[i]

		// skip over things like {} and []
		if p.Skip {
			p.Log(i, c, 0)
			p.Skip = false
			continue
		}

		// switch over the current node type
		switch byte(c >> 56) {

		// string
		case '"':
			// p.String grabs the string value from the string buffer and
			// increments p.stringOffset to account for the length
			s := p.String(p.Parsed.Tape[i+1])
			// n is the next node type
			n = byte(p.Parsed.Tape[i+2] >> 56)

			switch p.State {
			case PREDICATE:
				p.FoundPredicate(s)
				switch n {
				case '{':
					p.State = OBJECT
					p.FoundSubject(OBJECT, p.Depth.Subject())
				case '[':
					p.State = ARRAY
					p.FoundSubject(ARRAY, p.Depth.Subject())
				default:
					switch p.Quad.Predicate {
					case "uid":
						p.State = UID
					case "type":
						p.State = GEO
					default:
						p.State = SCALAR
					}
				}

			case SCALAR:
				p.State = PREDICATE
				p.FoundValue(s)

			case UID:
				p.State = PREDICATE
				p.FoundUid(s)

			case GEO:
				switch s {
				case "Point", "MultiPoint":
					fallthrough
				case "LineString", "MultiLineString":
					fallthrough
				case "Polygon", "MultiPolygon":
					fallthrough
				case "GeometryCollection":
					// TODO: parsing geojson is hard so right now we skip over
					//       the object
					i = p.Depth.Closes()
					p.LogMore(fmt.Sprintf("skipping %s geo object", s))
					p.State = PREDICATE
				}
			}

		// array open
		case '[':
			n = byte(p.Parsed.Tape[i+1] >> 56)
			if n != ']' {
				p.Depth.Increase(ARRAY, (c<<8)>>8-1)
			}

			p.LogMore(fmt.Sprintf("closing [ at %d", (c<<8)>>8-1))

			switch n {
			case '[':
				p.State = ARRAY
			case ']':
				p.Queue.Pop(ARRAY)
				p.State = PREDICATE
				p.Skip = true
			case '{':
				p.State = OBJECT
			default:
				p.State = SCALAR
			}

		// array close
		case ']':
			n = byte(p.Parsed.Tape[i+1] >> 56)

			if !p.Queue.Empty() {
				if waiting := p.Queue.Pop(ARRAY); waiting != nil {
					uids := p.Depth.Decrease(ARRAY).Uids
					for _, uid := range uids {
						p.Quads = append(p.Quads, &api.NQuad{
							Subject:   p.Depth.Subject(),
							Predicate: waiting.Predicate,
							ObjectId:  uid,
						})
					}
				}
			}

			switch n {
			case '[':
				p.State = ARRAY
			case '{':
				p.State = OBJECT
			case '"', '}':
				p.State = PREDICATE
			}

		// object open
		case '{':
			n = byte(p.Parsed.Tape[i+1] >> 56)
			if n != '}' {
				p.Depth.Increase(OBJECT, (c<<8)>>8-1)
			}

			p.LogMore(fmt.Sprintf("closing { at %d", (c<<8)>>8-1))

			switch n {
			case '{':
				p.State = OBJECT
			case '}':
				p.State = PREDICATE
				p.Queue.Pop(OBJECT)
				p.Skip = true
			case '[':
				p.State = ARRAY
			case '"':
				p.State = PREDICATE
			}

		// object close
		case '}':
			n = byte(p.Parsed.Tape[i+1] >> 56)
			if p.Depth.ArrayObject() {
				p.Depth.ArrayUid(p.Depth.Subject())
			}
			objectId := p.Depth.Decrease(OBJECT).Subject()
			if !p.Queue.Empty() {
				if waiting := p.Queue.Pop(OBJECT); waiting != nil {
					p.Quads = append(p.Quads, &api.NQuad{
						Subject:   p.Depth.Subject(),
						Predicate: waiting.Predicate,
						ObjectId:  objectId,
					})
				}
			}

			switch n {
			case '{':
				p.State = OBJECT
			case '"', '}', ']':
				p.State = PREDICATE
			}

		// root
		case 'r':
			n = byte(p.Parsed.Tape[i+1] >> 56)

			switch n {
			case '{':
				p.State = OBJECT
			case '[':
				p.State = ARRAY
			}

		// null
		case 'n':
			n = byte(p.Parsed.Tape[i+1] >> 56)

		// true
		case 't':
			n = byte(p.Parsed.Tape[i+1] >> 56)

			switch p.State {
			case SCALAR:
				p.State = PREDICATE
				p.FoundValue(true)
			}

		// false
		case 'f':
			n = byte(p.Parsed.Tape[i+1] >> 56)

			switch p.State {
			case SCALAR:
				p.State = PREDICATE
				p.FoundValue(false)
			}

		// int64
		case 'l':
			n = byte(p.Parsed.Tape[i+2] >> 56)

			switch p.State {
			case SCALAR:
				p.State = PREDICATE
				// int64 value is stored after the current node (i + 1)
				p.FoundValue(int64(p.Parsed.Tape[i+1]))
			}

		// uint64
		case 'u':
			n = byte(p.Parsed.Tape[i+2] >> 56)

			switch p.State {
			case SCALAR:
				p.State = PREDICATE
				// uint64 value is stored after the current node (i + 1)
				p.FoundValue(p.Parsed.Tape[i+1])
			}

		// float64
		case 'd':
			n = byte(p.Parsed.Tape[i+2] >> 56)

			switch p.State {
			case SCALAR:
				p.State = PREDICATE
				// float64 value is stored after the current node (i + 1)
				p.FoundValue(math.Float64frombits(p.Parsed.Tape[i+1]))
			}
		}

		p.Log(i, c, n)
	}
	return
}

func (p *Parser) FoundUid(s string) {
	p.Depth.Uid(s)
	p.Quad = &api.NQuad{}
}

func (p *Parser) FoundSubject(t ParserState, s string) {
	p.Queue.Add(t, p.Quad)
	p.Quad = &api.NQuad{}
}

func (p *Parser) FoundPredicate(s string) {
	p.Quad.Predicate = s
}

func (p *Parser) FoundValue(v interface{}) {
	p.Quad.Subject = p.Depth.Subject()
	switch val := v.(type) {
	case string:
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_StrVal{val}}
	case float64:
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_DoubleVal{val}}
	case int64:
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_IntVal{val}}
	case uint64:
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_IntVal{int64(val)}}
	case bool:
		p.Quad.ObjectValue = &api.Value{Val: &api.Value_BoolVal{val}}
	}
	p.Quads = append(p.Quads, p.Quad)
	p.Quad = &api.NQuad{}
}
