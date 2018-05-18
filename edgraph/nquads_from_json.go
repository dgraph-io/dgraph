/*
 * Copyright 2018 Dgraph Labs, Inc.
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package edgraph

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
	geom "github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

func parseFacets(m map[string]interface{}, prefix string) ([]*api.Facet, error) {
	// This happens at root.
	if prefix == "" {
		return nil, nil
	}

	var facetsForPred []*api.Facet
	var fv interface{}
	for fname, facetVal := range m {
		if facetVal == nil {
			continue
		}
		if !strings.HasPrefix(fname, prefix) {
			continue
		}

		if len(fname) <= len(prefix) {
			return nil, x.Errorf("Facet key is invalid: %s", fname)
		}
		// Prefix includes colon, predicate:
		f := &api.Facet{Key: fname[len(prefix):]}
		switch v := facetVal.(type) {
		case string:
			if t, err := types.ParseTime(v); err == nil {
				f.ValType = api.Facet_DATETIME
				fv = t
			} else {
				f.ValType = api.Facet_STRING
				fv = v
			}
		case json.Number:
			valn := facetVal.(json.Number)
			if strings.Index(valn.String(), ".") >= 0 {
				if vf, err := valn.Float64(); err != nil {
					return nil, err
				} else {
					fv = vf
					f.ValType = api.Facet_FLOAT
				}
			} else {
				if vi, err := valn.Int64(); err != nil {
					return nil, err
				} else {
					fv = vi
					f.ValType = api.Facet_INT
				}
			}
		case bool:
			fv = v
			f.ValType = api.Facet_BOOL
		default:
			return nil, x.Errorf("Facet value for key: %s can only be string/float64/bool.",
				fname)
		}

		// convert facet val interface{} to binary
		tid := facets.TypeIDFor(&api.Facet{ValType: f.ValType})
		fVal := &types.Val{Tid: types.BinaryID}
		if err := types.Marshal(types.Val{Tid: tid, Value: fv}, fVal); err != nil {
			return nil, err
		}

		fval, ok := fVal.Value.([]byte)
		if !ok {
			return nil, x.Errorf("Error while marshalling types.Val into binary.")
		}
		f.Value = fval
		facetsForPred = append(facetsForPred, f)
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
	switch v.(type) {
	case json.Number:
		n := v.(json.Number)
		if strings.Index(n.String(), ".") >= 0 {
			f, err := n.Float64()
			if err != nil {
				return err
			}
			nq.ObjectValue = &api.Value{&api.Value_DoubleVal{f}}
			return nil
		}
		i, err := n.Int64()
		if err != nil {
			return err
		}
		nq.ObjectValue = &api.Value{&api.Value_IntVal{i}}
	case string:
		predWithLang := strings.SplitN(k, "@", 2)
		if len(predWithLang) == 2 && predWithLang[0] != "" {
			nq.Predicate = predWithLang[0]
			nq.Lang = predWithLang[1]
		}

		// Default value is considered as S P * deletion.
		if v == "" && op == delete {
			nq.ObjectValue = &api.Value{&api.Value_DefaultVal{x.Star}}
			return nil
		}

		nq.ObjectValue = &api.Value{&api.Value_StrVal{v.(string)}}
	case float64:
		if v == 0 && op == delete {
			nq.ObjectValue = &api.Value{&api.Value_DefaultVal{x.Star}}
			return nil
		}

		nq.ObjectValue = &api.Value{&api.Value_DoubleVal{v.(float64)}}
	case bool:
		if v == false && op == delete {
			nq.ObjectValue = &api.Value{&api.Value_DefaultVal{x.Star}}
			return nil
		}

		nq.ObjectValue = &api.Value{&api.Value_BoolVal{v.(bool)}}
	default:
		return x.Errorf("Unexpected type for val for attr: %s while converting to nquad", k)
	}
	return nil

}

func checkForDeletion(mr *mapResponse, m map[string]interface{}, op int) {
	// Since uid is the only key, this must be S * * deletion.
	if op == delete && len(mr.uid) > 0 && len(m) == 1 {
		mr.nquads = append(mr.nquads, &api.NQuad{
			Subject:     mr.uid,
			Predicate:   x.Star,
			ObjectValue: &api.Value{&api.Value_DefaultVal{x.Star}},
		})
	}
}

func tryParseAsGeo(b []byte, nq *api.NQuad) (bool, error) {
	var g geom.T
	err := geojson.Unmarshal(b, &g)
	if err == nil {
		geo, err := types.ObjectValue(types.GeoID, g)
		if err != nil {
			return false, x.Errorf("Couldn't convert value: %s to geo type", string(b))
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

		switch uidVal.(type) {
		case json.Number:
			uidn := uidVal.(json.Number)
			ui, err := uidn.Int64()
			if err != nil {
				return mr, err
			}
			uid = uint64(ui)

		case string:
			id := uidVal.(string)
			if len(id) == 0 {
				uid = 0
			} else if ok := strings.HasPrefix(id, "_:"); ok {
				mr.uid = id
			} else if u, err := strconv.ParseUint(id, 0, 64); err != nil {
				return mr, err
			} else {
				uid = u
			}
		}
		if uid > 0 {
			mr.uid = fmt.Sprintf("%d", uid)
		}
	}

	if len(mr.uid) == 0 {
		if op == delete {
			// Delete operations with a non-nil value must have a uid specified.
			return mr, x.Errorf("uid must be present and non-zero while deleting edges.")
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

		if op == delete {
			// This corresponds to edge deletion.
			if v == nil {
				mr.nquads = append(mr.nquads, &api.NQuad{
					Subject:     mr.uid,
					Predicate:   pred,
					ObjectValue: &api.Value{&api.Value_DefaultVal{x.Star}},
				})
				continue
			}
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

		if v == nil {
			if op == delete {
				nq.ObjectValue = &api.Value{&api.Value_DefaultVal{x.Star}}
				mr.nquads = append(mr.nquads, &nq)
			}
			continue
		}

		switch v.(type) {
		case string, json.Number, bool:
			if err := handleBasicType(pred, v, op, &nq); err != nil {
				return mr, err
			}
			mr.nquads = append(mr.nquads, &nq)
		case map[string]interface{}:
			val := v.(map[string]interface{})
			if len(val) == 0 {
				continue
			}

			// Geojson geometry should have type and coordinates.
			_, hasType := val["type"]
			_, hasCoordinates := val["coordinates"]
			if len(val) == 2 && hasType && hasCoordinates {
				b, err := json.Marshal(val)
				if err != nil {
					return mr, x.Errorf("Error while trying to parse "+
						"value: %+v as geo val", val)
				}
				ok, err := tryParseAsGeo(b, &nq)
				if err != nil {
					return mr, err
				}
				if ok {
					mr.nquads = append(mr.nquads, &nq)
					continue
				}
			}

			cr, err := mapToNquads(v.(map[string]interface{}), idx, op, pred)
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
			for _, item := range v.([]interface{}) {
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
						x.Errorf("Got unsupported type for list: %s", pred)
				}
			}
		default:
			return mr, x.Errorf("Unexpected type for val for attr: %s while converting to nquad", pred)
		}
	}

	fts, err := parseFacets(m, parentPred+query.FacetDelimeter)
	mr.fcts = fts
	return mr, err
}

const (
	set = iota
	delete
)

func nquadsFromJson(b []byte, op int) ([]*api.NQuad, error) {
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
		return nil, fmt.Errorf("Couldn't parse json as a map or an array.")
	}

	var idx int
	var nquads []*api.NQuad
	if len(list) > 0 {
		for _, obj := range list {
			if _, ok := obj.(map[string]interface{}); !ok {
				return nil, x.Errorf("Only array of map allowed at root.")
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
