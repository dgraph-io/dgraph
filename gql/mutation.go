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
	"fmt"
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
	Set    []*protos.NQuad
	Del    []*protos.NQuad
	Schema string
}

func (m Mutation) HasVariables() bool {
	for _, n := range m.Set {
		if HasVariables(n) {
			return true
		}
	}
	for _, n := range m.Del {
		if HasVariables(n) {
			return true
		}
	}
	return false
}

// HasOps returns true iff the mutation has at least one non-empty
// part.
func (m Mutation) HasOps() bool {
	return len(m.Set) > 0 || len(m.Del) > 0 || len(m.Schema) > 0
}

// NeededVars retrieves NQuads and variable names of NQuads that refer a variable.
func (m Mutation) NeededVars() (res map[*protos.NQuad]string) {
	res = make(map[*protos.NQuad]string)
	addIfVar := func(nq *protos.NQuad) {
		if len(nq.SubjectVar) > 0 {
			res[nq] = nq.SubjectVar
		}
		if len(nq.ObjectVar) > 0 {
			res[nq] = nq.ObjectVar
		}
	}
	for _, s := range m.Set {
		addIfVar(s)
	}
	for _, d := range m.Del {
		addIfVar(d)
	}
	return
}

type NQuads struct {
	NQuads []*protos.NQuad
	Types  []protos.DirectedEdge_Op
}

func WrapNQ(s []*protos.NQuad, typ protos.DirectedEdge_Op) NQuads {
	t := make([]protos.DirectedEdge_Op, len(s))
	for i := range t {
		t[i] = typ
	}
	return NQuads{NQuads: s, Types: t}
}

func (n *NQuads) SetTypes(t protos.DirectedEdge_Op) {
	n.Types = make([]protos.DirectedEdge_Op, len(n.NQuads))
	for i := range n.Types {
		n.Types[i] = t
	}
}

func (n NQuads) IsEmpty() bool {
	return len(n.NQuads) == 0
}

func (n NQuads) Add(m NQuads) (res NQuads) {
	res.NQuads = n.NQuads
	res.Types = n.Types
	res.NQuads = append(res.NQuads, m.NQuads...)
	res.Types = append(res.Types, m.Types...)
	x.AssertTrue(len(res.NQuads) == len(n.NQuads)+len(m.NQuads))
	x.AssertTrue(len(res.Types) == len(n.Types)+len(m.Types))
	return
}

// Partitions NQuads using given predicate.
func (n NQuads) Partition(by func(*protos.NQuad) bool) (t NQuads, f NQuads) {
	t.NQuads = make([]*protos.NQuad, 0, 10)
	f.NQuads = make([]*protos.NQuad, 0, 10)
	t.Types = make([]protos.DirectedEdge_Op, 0, 10)
	f.Types = make([]protos.DirectedEdge_Op, 0, 10)
	p := func(nq *protos.NQuad, typ protos.DirectedEdge_Op) {
		if by(nq) {
			t.NQuads = append(t.NQuads, nq)
			t.Types = append(t.Types, typ)
		} else {
			f.NQuads = append(f.NQuads, nq)
			f.Types = append(f.Types, typ)
		}
	}
	for i, s := range n.NQuads {
		p(s, n.Types[i])
	}
	x.AssertTrue(len(t.NQuads)+len(f.NQuads) == len(n.NQuads))
	x.AssertTrue(len(t.Types)+len(f.Types) == len(n.Types))
	return
}

// HasVariables returns true iff given NQuad refers some variable.
func HasVariables(n *protos.NQuad) bool {
	return len(n.SubjectVar) > 0 || len(n.ObjectVar) > 0
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
	if p.Tid == types.GeoID || p.Tid == types.DateTimeID {
		return p.Value.([]byte), nil
	}

	p1 := types.ValueForType(types.BinaryID)
	if err := types.Marshal(p, &p1); err != nil {
		return []byte{}, err
	}
	return []byte(p1.Value.([]byte)), nil
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

func (nq NQuad) createUidEdge(subjectUid uint64, objectUid uint64) *protos.DirectedEdge {
	out := nq.createEdgePrototype(subjectUid)
	out.ValueId = objectUid
	return out
}

func (nq NQuad) createValueEdge(subjectUid uint64) (*protos.DirectedEdge, error) {
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
		edge = nq.createUidEdge(sUid, oUid)
	case x.ValuePlain, x.ValueMulti:
		edge, err = nq.createValueEdge(sUid)
	default:
		return &emptyEdge, errors.New("unknow value type")
	}
	if err != nil {
		return nil, err
	}
	return edge, nil
}

func (nq NQuad) ExpandVariables(newToUid map[string]uint64,
	subjectUids []uint64,
	objectUids []uint64) (edges []*protos.DirectedEdge, err error) {
	var edge *protos.DirectedEdge
	edges = make([]*protos.DirectedEdge, 0, len(subjectUids)*len(objectUids))

	if len(subjectUids) == 0 {
		if len(nq.Subject) == 0 {
			// Empty variable.
			return
		}
		sUid, err := toUid(nq.Subject, newToUid)
		if err != nil {
			return nil, err
		}
		subjectUids = []uint64{sUid}[:]
	}

	switch nq.valueType() {
	case x.ValueUid:
		for _, sUid := range subjectUids {
			if len(objectUids) > 0 {
				for _, oUid := range objectUids {
					edge = nq.createUidEdge(sUid, oUid)
					edges = append(edges, edge)
				}
			} else {
				x.AssertTruef(len(nq.ObjectId) > 0, "Empty objectId %s", nq.String())
				oUid, err := toUid(nq.ObjectId, newToUid)
				if err != nil {
					return edges, err
				}
				edge = nq.createUidEdge(sUid, oUid)
				edges = append(edges, edge)
			}

		}
	case x.ValuePlain, x.ValueMulti:
		for _, sUid := range subjectUids {
			edge, err = nq.createValueEdge(sUid)
			if err != nil {
				return edges, err
			}
			edges = append(edges, edge)
		}
	default:
		return edges, fmt.Errorf("unknow value type: %s", nq.String())
	}
	return edges, nil

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
	hasSpecialId := len(nq.ObjectId) == 0 && len(nq.ObjectVar) == 0
	return x.ValueType(hasValue, hasLang, hasSpecialId)
}
