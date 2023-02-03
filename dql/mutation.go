/*
 * Copyright 2017-2022 Dgraph Labs, Inc. and Contributors
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

package dql

import (
	"strconv"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgo/v210/protos/api"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	errInvalidUID = errors.New("UID must to be greater than 0")
)

// Mutation stores the strings corresponding to set and delete operations.
type Mutation struct {
	Cond         string
	Set          []*api.NQuad
	Del          []*api.NQuad
	AllowedPreds []string

	Metadata *pb.Metadata
}

// ParseUid parses the given string into an UID. This method returns with an error
// if the string cannot be parsed or the parsed UID is zero.
func ParseUid(xid string) (uint64, error) {
	// If string represents a UID, convert to uint64 and return.
	uid, err := strconv.ParseUint(xid, 0, 64)
	if err != nil {
		return 0, err
	}
	if uid == 0 {
		return 0, errInvalidUID
	}
	return uid, nil
}

// NQuad is an alias for the NQuad type in the API protobuf library.
type NQuad struct {
	*api.NQuad
}

func TypeValFrom(val *api.Value) types.Val {
	switch val.Val.(type) {
	case *api.Value_BytesVal:
		return types.Val{Tid: types.BinaryID, Value: val.GetBytesVal()}
	case *api.Value_IntVal:
		return types.Val{Tid: types.IntID, Value: val.GetIntVal()}
	case *api.Value_StrVal:
		return types.Val{Tid: types.StringID, Value: val.GetStrVal()}
	case *api.Value_BoolVal:
		return types.Val{Tid: types.BoolID, Value: val.GetBoolVal()}
	case *api.Value_DoubleVal:
		return types.Val{Tid: types.FloatID, Value: val.GetDoubleVal()}
	case *api.Value_GeoVal:
		return types.Val{Tid: types.GeoID, Value: val.GetGeoVal()}
	case *api.Value_DatetimeVal:
		return types.Val{Tid: types.DateTimeID, Value: val.GetDatetimeVal()}
	case *api.Value_PasswordVal:
		return types.Val{Tid: types.PasswordID, Value: val.GetPasswordVal()}
	case *api.Value_DefaultVal:
		return types.Val{Tid: types.DefaultID, Value: val.GetDefaultVal()}
	}

	return types.Val{Tid: types.StringID, Value: ""}
}

func byteVal(nq NQuad) ([]byte, types.TypeID, error) {
	// We infer object type from type of value. We set appropriate type in parse
	// function or the Go client has already set.
	p := TypeValFrom(nq.ObjectValue)
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
	if id, err := ParseUid(subject); err == nil || err == errInvalidUID {
		return id, err
	}
	// It's an xid
	if id, present := newToUid[subject]; present {
		return id, err
	}
	return 0, errors.Errorf("UID not found/generated for xid %s\n", subject)
}

var emptyEdge pb.DirectedEdge

func (nq NQuad) createEdgePrototype(subjectUid uint64) *pb.DirectedEdge {
	return &pb.DirectedEdge{
		Entity:    subjectUid,
		Attr:      nq.Predicate,
		Namespace: nq.Namespace,
		Lang:      nq.Lang,
		Facets:    nq.Facets,
	}
}

// CreateUidEdge returns a Directed edge connecting the given subject and object UIDs.
func (nq NQuad) CreateUidEdge(subjectUid uint64, objectUid uint64) *pb.DirectedEdge {
	out := nq.createEdgePrototype(subjectUid)
	out.ValueId = objectUid
	out.ValueType = pb.Posting_UID
	return out
}

// CreateValueEdge returns a DirectedEdge with the given subject. The predicate,
// language, and facet values are derived from the NQuad.
func (nq NQuad) CreateValueEdge(subjectUid uint64) (*pb.DirectedEdge, error) {
	var err error

	out := nq.createEdgePrototype(subjectUid)
	if err = copyValue(out, nq); err != nil {
		return &emptyEdge, err
	}
	return out, nil
}

// ToDeletePredEdge takes an NQuad of the form '* p *' and returns the equivalent
// directed edge. Returns an error if the NQuad does not have the expected form.
func (nq NQuad) ToDeletePredEdge() (*pb.DirectedEdge, error) {
	if nq.Subject != x.Star && nq.ObjectValue.String() != x.Star {
		return &emptyEdge, errors.Errorf("Subject and object both should be *. Got: %+v", nq)
	}

	out := &pb.DirectedEdge{
		// This along with edge.ObjectValue == x.Star would indicate
		// that we want to delete the predicate.
		Entity:    0,
		Attr:      nq.Predicate,
		Namespace: nq.Namespace,
		Lang:      nq.Lang,
		Facets:    nq.Facets,
		Op:        pb.DirectedEdge_DEL,
	}

	if err := copyValue(out, nq); err != nil {
		return &emptyEdge, err
	}
	return out, nil
}

// ToEdgeUsing determines the UIDs for the provided XIDs and populates the
// xidToUid map.
func (nq NQuad) ToEdgeUsing(newToUid map[string]uint64) (*pb.DirectedEdge, error) {
	var edge *pb.DirectedEdge
	sUid, err := toUid(nq.Subject, newToUid)
	if err != nil {
		return nil, err
	}

	if sUid == 0 {
		return nil, errors.Errorf("Subject should be > 0 for nquad: %+v", nq)
	}

	switch nq.valueType() {
	case x.ValueUid:
		oUid, err := toUid(nq.ObjectId, newToUid)
		if err != nil {
			return nil, err
		}
		if oUid == 0 {
			return nil, errors.Errorf("ObjectId should be > 0 for nquad: %+v", nq)
		}
		edge = nq.CreateUidEdge(sUid, oUid)
	case x.ValuePlain, x.ValueMulti:
		edge, err = nq.CreateValueEdge(sUid)
	default:
		return &emptyEdge, errors.Errorf("Unknown value type for nquad: %+v", nq)
	}
	if err != nil {
		return nil, err
	}
	return edge, nil
}

func copyValue(out *pb.DirectedEdge, nq NQuad) error {
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
