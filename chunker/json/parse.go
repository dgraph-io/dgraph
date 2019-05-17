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

package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgraph/query"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/types/facets"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func parseFacets(m map[string]interface{}, prefix string) ([]*api.Facet, error) {
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
	nquads []*api.NQuad // nquads at this level including the children.
	uid    string       // uid retrieved or allocated for the node.
	fcts   []*api.Facet // facets on the edge connecting this node to the source if any.
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
		if v == false && op == DeleteNquads {
			nq.ObjectValue = &api.Value{Val: &api.Value_DefaultVal{DefaultVal: x.Star}}
			return nil
		}
		nq.ObjectValue = &api.Value{Val: &api.Value_BoolVal{BoolVal: v}}

	default:
		return errors.Errorf("Unexpected type for val for attr: %s while converting to nquad", k)
	}
	return nil

}

func checkForDeletion(mr *mapResponse, m map[string]interface{}, op int) {
	// Since uid is the only key, this must be S * * deletion.
	if op == DeleteNquads && len(mr.uid) > 0 && len(m) == 1 {
		mr.nquads = append(mr.nquads, &api.NQuad{
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
	if err == nil {
		geo, err := types.ObjectValue(types.GeoID, g)
		if err != nil {
			return false, errors.Errorf("Couldn't convert value: %s to geo type", string(b))
		}

		nq.ObjectValue = geo
		return true, nil
	}
	return false, nil
}

// TODO - Abstract these parameters to a struct.
func mapToNquads(m map[string]interface{}, idx *int, op int, parentPred string) (mapResponse, error) {
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
			if len(uidVal) == 0 {
				uid = 0
			} else if ok := strings.HasPrefix(uidVal, "_:"); ok {
				mr.uid = uidVal
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

		mr.uid = fmt.Sprintf("_:blank-%d", *idx)
		*idx++
	}

	for pred, v := range m {
		// We have already extracted the uid above so we skip that edge.
		// v can be nil if user didn't set a value and if omitEmpty was not supplied as JSON
		// option.
		// We also skip facets here because we parse them with the corresponding predicate.
		if pred == "uid" || strings.Index(pred, query.FacetDelimeter) > 0 {
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

				mr.nquads = append(mr.nquads, nq)
				continue
			}

			// If op is SetNquads, ignore this triplet and continue.
			continue
		}

		prefix := pred + query.FacetDelimeter
		// TODO - Maybe do an initial pass and build facets for all predicates. Then we don't have
		// to call parseFacets everytime.
		fts, err := parseFacets(m, prefix)
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
			mr.nquads = append(mr.nquads, &nq)
		case map[string]interface{}:
			if len(v) == 0 {
				continue
			}

			ok, err := handleGeoType(v, &nq)
			if err != nil {
				return mr, err
			}
			if ok {
				mr.nquads = append(mr.nquads, &nq)
				continue
			}

			cr, err := mapToNquads(v, idx, op, pred)
			if err != nil {
				return mr, err
			}

			// Add the connecting edge beteween the entities.
			nq.ObjectId = cr.uid
			nq.Facets = cr.fcts
			mr.nquads = append(mr.nquads, &nq)
			// Add the nquads that we got for the connecting entity.
			mr.nquads = append(mr.nquads, cr.nquads...)
		case []interface{}:
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
					mr.nquads = append(mr.nquads, &nq)
				case map[string]interface{}:
					// map[string]interface{} can mean geojson or a connecting entity.
					ok, err := handleGeoType(item.(map[string]interface{}), &nq)
					if err != nil {
						return mr, err
					}
					if ok {
						mr.nquads = append(mr.nquads, &nq)
						continue
					}

					cr, err := mapToNquads(iv, idx, op, pred)
					if err != nil {
						return mr, err
					}
					nq.ObjectId = cr.uid
					nq.Facets = cr.fcts
					mr.nquads = append(mr.nquads, &nq)
					// Add the nquads that we got for the connecting entity.
					mr.nquads = append(mr.nquads, cr.nquads...)
				default:
					return mr,
						errors.Errorf("Got unsupported type for list: %s", pred)
				}
			}
		default:
			return mr, errors.Errorf("Unexpected type for val for attr: %s while converting to nquad", pred)
		}
	}

	fts, err := parseFacets(m, parentPred+query.FacetDelimeter)
	mr.fcts = fts
	return mr, err
}

const (
	SetNquads = iota
	DeleteNquads
)

func Parse(b []byte, op int) ([]*api.NQuad, error) {
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
			return nil, err
		}
	}

	if len(list) == 0 && len(ms) == 0 {
		return nil, fmt.Errorf("Couldn't parse json as a map or an array")
	}

	var idx int
	var nquads []*api.NQuad
	if len(list) > 0 {
		for _, obj := range list {
			if _, ok := obj.(map[string]interface{}); !ok {
				return nil, errors.Errorf("Only array of map allowed at root.")
			}
			mr, err := mapToNquads(obj.(map[string]interface{}), &idx, op, "")
			if err != nil {
				return mr.nquads, err
			}
			checkForDeletion(&mr, obj.(map[string]interface{}), op)
			nquads = append(nquads, mr.nquads...)
		}
		return nquads, nil
	}

	mr, err := mapToNquads(ms, &idx, op, "")
	checkForDeletion(&mr, ms, op)
	return mr.nquads, err
}
