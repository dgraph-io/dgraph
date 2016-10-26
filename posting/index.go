/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package posting

import (
	"context"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/geo"
	"github.com/dgraph-io/dgraph/schema"
	stype "github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	indexLog trace.EventLog
)

func init() {
	indexLog = trace.NewEventLog("index", "Logger")
}

func tokenizedIndexKeys(attr string, p stype.Value) ([][]byte, error) {
	schemaType := schema.TypeOf(attr)
	if !schemaType.IsScalar() {
		return nil, x.Errorf("Cannot index attribute %s of type object.", attr)
	}
	s := schemaType.(stype.Scalar)
	schemaVal, err := s.Convert(p)
	if err != nil {
		return nil, err
	}
	switch v := schemaVal.(type) {
	case *stype.Geo:
		return geo.IndexKeys(v)
	case *stype.Int32:
		return stype.IntIndex(attr, v)
	case *stype.Float:
		return stype.FloatIndex(attr, v)
	case *stype.Date:
		return stype.DateIndex(attr, v)
	case *stype.Time:
		return stype.TimeIndex(attr, v)
	case *stype.String:
		return stype.DefaultIndexKeys(attr, v), nil
	}
	return nil, nil
}

// processIndexTerm adds mutation(s) for a single term, to maintain index.
func processIndexTerm(ctx context.Context, attr string, uid uint64, p stype.Value, del bool) {
	x.Assert(uid != 0)
	keys, err := tokenizedIndexKeys(attr, p)
	if err != nil {
		// This data is not indexable
		return
	}
	edge := x.DirectedEdge{
		Timestamp: time.Now(),
		ValueId:   uid,
		Attribute: attr,
		Source:    "idx",
	}

	for _, key := range keys {
		plist, decr := GetOrCreate(key)
		defer decr()
		x.Assertf(plist != nil, "plist is nil [%s] %d %s", key, edge.ValueId, edge.Attribute)

		if del {
			_, err := plist.AddMutation(ctx, edge, Del)
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error deleting %s for attr %s entity %d: %v",
					string(key), edge.Attribute, edge.Entity))
			}
			indexLog.Printf("DEL [%s] [%d] OldTerm [%s]", edge.Attribute, edge.Entity, string(key))
		} else {
			_, err := plist.AddMutation(ctx, edge, Set)
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error adding %s for attr %s entity %d: %v",
					string(key), edge.Attribute, edge.Entity))
			}
			indexLog.Printf("SET [%s] [%d] NewTerm [%s]", edge.Attribute, edge.Entity, string(key))
		}
	}
}

// AddMutationWithIndex is AddMutation with support for indexing.
func (l *List) AddMutationWithIndex(ctx context.Context, t x.DirectedEdge, op byte) error {
	x.Assertf(len(t.Attribute) > 0 && t.Attribute[0] != ':',
		"[%s] [%d] [%s] %d %d\n", t.Attribute, t.Entity, string(t.Value), t.ValueId, op)

	var vbytes []byte
	var vtype byte
	var verr error

	doUpdateIndex := pstore != nil && (t.Value != nil) &&
		schema.IsIndexed(t.Attribute)
	if doUpdateIndex {
		// Check last posting for original value BEFORE any mutation actually happens.
		vbytes, vtype, verr = l.Value()
	}
	hasMutated, err := l.AddMutation(ctx, t, op)
	if err != nil {
		return err
	}
	if !hasMutated || !doUpdateIndex {
		return nil
	}

	// Exact matches.
	if verr == nil && len(vbytes) > 0 {
		delTerm := vbytes
		delType := vtype
		p := stype.ValueForType(stype.TypeID(delType))
		err = p.UnmarshalBinary(delTerm)
		if err != nil {
			return err
		}
		processIndexTerm(ctx, t.Attribute, t.Entity, p, true)
	}
	if op == Set {
		p := stype.ValueForType(stype.TypeID(t.ValueType))
		err := p.UnmarshalBinary(t.Value)
		if err != nil {
			return err
		}
		processIndexTerm(ctx, t.Attribute, t.Entity, p, false)
	}
	return nil
}
