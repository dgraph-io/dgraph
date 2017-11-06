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

package gql

import (
	"errors"
	"strconv"

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	ErrInvalidUID = errors.New("UID has to be greater than one.")
)

// Mutation stores the strings corresponding to set and delete operations.
type Mutation struct {
	Set     []*protos.NQuad
	Del     []*protos.NQuad
	DropAll bool
	Schema  string
}

// HasOps returns true iff the mutation has at least one non-empty
// part.
func (m Mutation) HasOps() bool {
	return len(m.Set) > 0 || len(m.Del) > 0 || len(m.Schema) > 0 || m.DropAll
}

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
		if val.GetStrVal() == "" {
			return types.Val{types.StringID, "_nil_"}
		}
		return types.Val{types.StringID, val.GetStrVal()}
	case *protos.Value_BoolVal:
		return types.Val{types.BoolID, val.GetBoolVal()}
	case *protos.Value_DoubleVal:
		return types.Val{types.FloatID, val.GetDoubleVal()}
	case *protos.Value_GeoVal:
		return types.Val{types.GeoID, val.GetGeoVal()}
	case *protos.Value_DatetimeVal:
		return types.Val{types.DateTimeID, val.GetDatetimeVal()}
	case *protos.Value_PasswordVal:
		return types.Val{types.PasswordID, val.GetPasswordVal()}
	case *protos.Value_DefaultVal:
		if val.GetDefaultVal() == "" {
			return types.Val{types.DefaultID, "_nil_"}
		}
		return types.Val{types.DefaultID, val.GetDefaultVal()}
	}

	return types.Val{types.StringID, ""}
}

func byteVal(nq NQuad) ([]byte, types.TypeID, error) {
	// We infer object type from type of value. We set appropriate type in parse
	// function or the Go client has already set.
	p := typeValFrom(nq.ObjectValue)
	// These three would have already been marshalled to bytes by the client or
	// in parse function.
	if p.Tid == types.GeoID || p.Tid == types.DateTimeID {
		return p.Value.([]byte), p.Tid, nil
	}

	p1 := types.ValueForType(types.BinaryID)
	if err := types.Marshal(p, &p1); err != nil {
		return []byte{}, p.Tid, err
	}
	return []byte(p1.Value.([]byte)), p.Tid, nil
}

func toUid(subject string, newToUid map[string]uint64) (uid uint64, err error) {
	x.AssertTrue(len(subject) > 0)
	if id, err := ParseUid(subject); err == nil || err == ErrInvalidUID {
		return id, err
	}
	// It's an xid
	if id, present := newToUid[subject]; present {
		return id, err
	}
	return 0, x.Errorf("uid not found/generated for xid %s\n", subject)
}

var emptyEdge protos.DirectedEdge

func (nq NQuad) createEdge(subjectUid uint64, newToUid map[string]uint64) (*protos.DirectedEdge, error) {
	var err error
	var objectUid uint64

	out := &protos.DirectedEdge{
		Entity: subjectUid,
		Attr:   nq.Predicate,
		Label:  nq.Label,
		Lang:   nq.Lang,
		Facets: nq.Facets,
	}

	switch nq.valueType() {
	case x.ValueUid:
		objectUid, err = toUid(nq.ObjectId, newToUid)
		if err != nil {
			return out, err
		}
		x.AssertTrue(objectUid > 0)
		out.ValueId = objectUid
	case x.ValuePlain, x.ValueMulti:
		if err = copyValue(out, nq); err != nil {
			return &emptyEdge, err
		}
	default:
		return &emptyEdge, errors.New("unknow value type")
	}
	return out, nil
}

func (nq NQuad) createEdgePrototype(subjectUid uint64) *protos.DirectedEdge {
	return &protos.DirectedEdge{
		Entity: subjectUid,
		Attr:   nq.Predicate,
		Label:  nq.Label,
		Lang:   nq.Lang,
		Facets: nq.Facets,
	}
}

func (nq NQuad) CreateUidEdge(subjectUid uint64, objectUid uint64) *protos.DirectedEdge {
	out := nq.createEdgePrototype(subjectUid)
	out.ValueId = objectUid
	return out
}

func (nq NQuad) CreateValueEdge(subjectUid uint64) (*protos.DirectedEdge, error) {
	var err error

	out := nq.createEdgePrototype(subjectUid)
	if err = copyValue(out, nq); err != nil {
		return &emptyEdge, err
	}
	return out, nil
}

func (nq NQuad) ToDeletePredEdge() (*protos.DirectedEdge, error) {
	if nq.Subject != x.Star && nq.ObjectValue.String() != x.Star {
		return &emptyEdge, x.Errorf("Subject and object both should be *. Got: %+v", nq)
	}

	out := &protos.DirectedEdge{
		// This along with edge.ObjectValue == x.Star would indicate
		// that we want to delete the predicate.
		Entity: 0,
		Attr:   nq.Predicate,
		Label:  nq.Label,
		Lang:   nq.Lang,
		Facets: nq.Facets,
		Op:     protos.DirectedEdge_DEL,
	}

	if err := copyValue(out, nq); err != nil {
		return &emptyEdge, err
	}
	return out, nil
}

// ToEdgeUsing determines the UIDs for the provided XIDs and populates the
// xidToUid map.
func (nq NQuad) ToEdgeUsing(newToUid map[string]uint64) (*protos.DirectedEdge, error) {
	var edge *protos.DirectedEdge
	sUid, err := toUid(nq.Subject, newToUid)
	if err != nil {
		return nil, err
	}

	switch nq.valueType() {
	case x.ValueUid:
		oUid, err := toUid(nq.ObjectId, newToUid)
		if err != nil {
			return nil, err
		}
		edge = nq.CreateUidEdge(sUid, oUid)
	case x.ValuePlain, x.ValueMulti:
		edge, err = nq.CreateValueEdge(sUid)
	default:
		return &emptyEdge, x.Errorf("unknown value type for nquad: %+v", nq)
	}
	if err != nil {
		return nil, err
	}
	return edge, nil
}

func copyValue(out *protos.DirectedEdge, nq NQuad) error {
	var err error
	var t types.TypeID
	if out.Value, t, err = byteVal(nq); err != nil {
		return err
	}
	out.ValueType = t.Enum()
	return nil
}

func (nq NQuad) valueType() x.ValueTypeInfo {
	hasValue := nq.ObjectValue != nil
	hasLang := len(nq.Lang) > 0
	hasSpecialId := len(nq.ObjectId) == 0
	return x.ValueType(hasValue, hasLang, hasSpecialId)
}
