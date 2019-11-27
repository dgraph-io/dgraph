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
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/dgraph-io/dgo/v2/protos/api"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
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

func parseFacetsJSON(m map[string]interface{}, prefix string) ([]*api.Facet, error) {
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

				// the FacetFor function already converts the value to binary
				// so there is no need for the conversion again after the switch block
				facetsForPred = append(facetsForPred, facet)
				continue
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
		case bool:
			jsonValue = v
			valueType = api.Facet_BOOL
		default:
			return nil, errors.Errorf("Facet value for key: %s can only be string/float64/bool.",
				fname)
		}

		// convert facet val interface{} to binary
		binaryValueFacet, err := facets.ToBinary(key, jsonValue, valueType)
		if err != nil {
			return nil, err
		}
		facetsForPred = append(facetsForPred, binaryValueFacet)
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
	metadata  ParseMetadata
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
func (buf *NQuadBuffer) Metadata() ParseMetadata {
	return buf.metadata
}

// PushMetadata pushes and aggregates metadata derived during the parsing. This
// metadata is expected to be a lot smaller than the set of NQuads so it's not
// necessary to send them in batches. Instead, the metadata is sent to the channel
// when Flush is called.
func (buf *NQuadBuffer) PushMetadata(metadata *ParseMetadata) {
	if metadata == nil {
		return
	}

	buf.metadata.ForcedSinglePreds = append(buf.metadata.ForcedSinglePreds,
		metadata.ForcedSinglePreds...)
	buf.metadata.ForcedListPreds = append(buf.metadata.ForcedListPreds,
		metadata.ForcedListPreds...)
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
		if pred == "uid" || strings.Index(pred, x.FacetDelimeter) > 0 {
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

		prefix := pred + x.FacetDelimeter
		// TODO - Maybe do an initial pass and build facets for all predicates. Then we don't have
		// to call parseFacets everytime.
		fts, err := parseFacetsJSON(m, prefix)
		if err != nil {
			return mr, err
		}

		nq := api.NQuad{
			Subject:   mr.uid,
			Predicate: pred,
			Facets:    fts,
		}

		// Here we split predicate and lang directive (ex: "name@en"), if needed. With JSON
		// mutations that's the only way to send language for a value.
		nq.Predicate, nq.Lang = x.PredicateLang(nq.Predicate)

		switch v := v.(type) {
		case string, json.Number, bool:
			if err := handleBasicType(pred, v, op, &nq); err != nil {
				return mr, err
			}
			buf.Push(&nq)
			buf.PushMetadata(&ParseMetadata{ForcedSinglePreds: []string{pred}})
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
				buf.PushMetadata(&ParseMetadata{ForcedSinglePreds: []string{pred}})
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
			buf.PushMetadata(&ParseMetadata{ForcedSinglePreds: []string{pred}})
		case []interface{}:
			buf.PushMetadata(&ParseMetadata{ForcedListPreds: []string{pred}})
			for _, item := range v {
				nq := api.NQuad{
					Subject:   mr.uid,
					Predicate: pred,
				}

				switch iv := item.(type) {
				case string, float64, json.Number:
					if err := handleBasicType(pred, iv, op, &nq); err != nil {
						return mr, err
					}
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

	fts, err := parseFacetsJSON(m, parentPred+x.FacetDelimeter)
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

		buffer.Reset()  // The previous contents are used. Reset here.
		buffer.Write(b) // Rewrite b into buffer, so it can be consumed.
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
func ParseJSON(b []byte, op int) ([]*api.NQuad, ParseMetadata, error) {
	buf := NewNQuadBuffer(-1)
	err := buf.ParseJSON(b, op)
	if err != nil {
		return nil, ParseMetadata{}, err
	}

	buf.Flush()
	nqs := <-buf.Ch()
	metadata := buf.Metadata()
	return nqs, metadata, nil
}
