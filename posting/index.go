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
	"encoding/binary"
	"math"
	"strings"
	"time"

	geom "github.com/twpayne/go-geom"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const maxBatchSize = 32 * (1 << 20)

var (
	indexLog   trace.EventLog
	reverseLog trace.EventLog
)

func init() {
	indexLog = trace.NewEventLog("index", "Logger")
	reverseLog = trace.NewEventLog("reverse", "Logger")
}

// IndexTokens return tokens, without the predicate prefix and index rune.
func IndexTokens(attr string, src types.Val) ([]string, error) {
	schemaType, err := schema.TypeOf(attr)
	if err != nil || !schemaType.IsScalar() {
		return nil, x.Errorf("Cannot index attribute %s of type object.", attr)
	}
	s := schemaType
	sv, err := types.Convert(src, s)
	if err != nil {
		return nil, err
	}
	switch sv.Tid {
	case types.GeoID:
		return types.IndexGeoTokens(sv.Value.(geom.T))
	case types.Int32ID:
		return IntIndex(sv.Value.(int32))
	case types.FloatID:
		return FloatIndex(sv.Value.(float64))
	case types.DateID:
		return DateIndex(sv.Value.(time.Time))
	case types.DateTimeID:
		return TimeIndex(sv.Value.(time.Time))
	case types.StringID:
		return DefaultIndexKeys(sv.Value.(string))
	default:
		return nil, x.Errorf("Invalid type. Cannot be indexed")
	}
	return nil, nil
}

// addIndexMutations adds mutation(s) for a single term, to maintain index.
// t represents the original uid -> value edge.
func addIndexMutations(ctx context.Context, t *task.DirectedEdge, p types.Val, op task.DirectedEdge_Op) {
	attr := t.Attr
	uid := t.Entity
	x.AssertTrue(uid != 0)
	tokens, err := IndexTokens(attr, p)
	if err != nil {
		// This data is not indexable
		return
	}

	// Create a value token -> uid edge.
	edge := &task.DirectedEdge{
		ValueId: uid,
		Attr:    attr,
		Label:   "idx",
		Op:      op,
	}

	for _, token := range tokens {
		addIndexMutation(ctx, edge, token)
	}
}

func addIndexMutation(ctx context.Context, edge *task.DirectedEdge, token string) {
	key := x.IndexKey(edge.Attr, token)

	var groupId uint32
	if rv, ok := ctx.Value("raft").(x.RaftValue); ok {
		groupId = rv.Group
	}

	plist, decr := GetOrCreate(key, groupId)
	defer decr()

	x.AssertTruef(plist != nil, "plist is nil [%s] %d %s",
		token, edge.ValueId, edge.Attr)
	_, err := plist.AddMutation(ctx, edge)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err,
			"Error adding/deleting %s for attr %s entity %d: %v",
			token, edge.Attr, edge.Entity))
	}
	indexLog.Printf("%s [%s] [%d] Term [%s]",
		edge.Op, edge.Attr, edge.Entity, token)
}

func addReverseMutation(ctx context.Context, t *task.DirectedEdge) {
	key := x.ReverseKey(t.Attr, t.ValueId)
	groupId := group.BelongsTo(t.Attr)

	plist, decr := GetOrCreate(key, groupId)
	defer decr()

	x.AssertTruef(plist != nil, "plist is nil [%s] %d %d",
		t.Attr, t.Entity, t.ValueId)
	edge := &task.DirectedEdge{
		Entity:  t.ValueId,
		ValueId: t.Entity,
		Attr:    t.Attr,
		Label:   "rev",
		Op:      t.Op,
	}

	_, err := plist.AddMutation(ctx, edge)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err,
			"Error adding/deleting reverse edge for attr %s src %d dst %d",
			t.Attr, t.Entity, t.ValueId))
	}
	reverseLog.Printf("%s [%s] [%d] [%d]", t.Op, t.Attr, t.Entity, t.ValueId)
}

// AddMutationWithIndex is AddMutation with support for indexing. It also
// supports reverse edges.
func (l *List) AddMutationWithIndex(ctx context.Context, t *task.DirectedEdge) error {
	x.AssertTruef(len(t.Attr) > 0,
		"[%s] [%d] [%v] %d %d\n", t.Attr, t.Entity, t.Value, t.ValueId, t.Op)

	var val types.Val
	var verr error

	l.index.Lock()
	defer l.index.Unlock()

	doUpdateIndex := pstore != nil && (t.Value != nil) && schema.IsIndexed(t.Attr)
	{
		l.Lock()
		if doUpdateIndex {
			// Check last posting for original value BEFORE any mutation actually happens.
			val, verr = l.value()
		}
		hasMutated, err := l.addMutation(ctx, t)
		l.Unlock()

		if err != nil {
			return err
		}
		if !hasMutated {
			return nil
		}
	}
	if doUpdateIndex {
		// Exact matches.
		if verr == nil && val.Value != nil {
			addIndexMutations(ctx, t, val, task.DirectedEdge_DEL)
		}
		if t.Op == task.DirectedEdge_SET {
			p := types.Val{
				Tid:   types.TypeID(t.ValueType),
				Value: t.Value,
			}
			addIndexMutations(ctx, t, p, task.DirectedEdge_SET)
		}
	}

	if (pstore != nil) && (t.ValueId != 0) && schema.IsReversed(t.Attr) {
		addReverseMutation(ctx, t)
	}
	return nil
}

// RebuildIndex rebuilds index for a given attribute.
func RebuildIndex(ctx context.Context, attr string) error {
	x.AssertTruef(schema.IsIndexed(attr), "Attr %s not indexed", attr)

	// Delete index entries from data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	idxIt := pstore.NewIterator()
	defer idxIt.Close()

	wb := pstore.NewWriteBatch()
	defer wb.Destroy()
	var batchSize int
	for idxIt.Seek(prefix); idxIt.ValidForPrefix(prefix); idxIt.Next() {
		key := idxIt.Key().Data()
		batchSize += len(key)
		wb.Delete(key)

		if batchSize >= maxBatchSize {
			if err := pstore.WriteBatch(wb); err != nil {
				return err
			}
			wb.Clear()
			batchSize = 0
		}
	}
	if wb.Count() > 0 {
		if err := pstore.WriteBatch(wb); err != nil {
			return err
		}
		wb.Clear()
	}

	// Add index entries to data store.
	edge := task.DirectedEdge{Attr: attr}
	prefix = pk.DataPrefix()
	it := pstore.NewIterator()
	defer it.Close()
	var pl types.PostingList
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		pki := x.Parse(it.Key().Data())
		edge.Entity = pki.Uid
		x.Check(pl.Unmarshal(it.Value().Data()))

		// Check last entry for value.
		if len(pl.Postings) == 0 {
			continue
		}
		p := pl.Postings[len(pl.Postings)-1]
		if p.Uid != math.MaxUint64 {
			continue
		}

		// Add index entries based on p.
		val := types.Val{
			Value: p.Value,
			Tid:   types.TypeID(p.ValType),
		}
		addIndexMutations(ctx, &edge, val, task.DirectedEdge_SET)
	}
	return nil
}

// DefaultIndexKeys tokenizes data as a string and return keys for indexing.
func DefaultIndexKeys(val string) ([]string, error) {
	words := strings.Fields(val)
	tokens := make([]string, 0, 5)
	for _, it := range words {
		if it == "_nil_" {
			tokens = append(tokens, it)
			continue
		}

		x.AssertTruef(!tok.ICUDisabled(), "Indexing requires ICU to be enabled.")
		tokenizer, err := tok.NewTokenizer([]byte(it))
		if err != nil {
			return nil, err
		}
		for {
			s := tokenizer.Next()
			if s == nil {
				break
			}
			tokens = append(tokens, string(s))
		}
		tokenizer.Destroy()
	}
	return tokens, nil
}

func encodeInt(val int32) ([]string, error) {
	buf := make([]byte, 5)
	binary.BigEndian.PutUint32(buf[1:], uint32(val))
	if val < 0 {
		buf[0] = 0
	} else {
		buf[0] = 1
	}
	return []string{string(buf)}, nil
}

// IntIndex indexs int type.
func IntIndex(val int32) ([]string, error) {
	return encodeInt(val)
}

// FloatIndex indexs float type.
func FloatIndex(val float64) ([]string, error) {
	in := int32(val)
	return encodeInt(in)
}

// DateIndex indexs time type.
func DateIndex(val time.Time) ([]string, error) {
	in := int32(val.Year())
	return encodeInt(in)
}

// TimeIndex indexs time type.
func TimeIndex(val time.Time) ([]string, error) {
	in := int32(val.Year())
	return encodeInt(in)
}
