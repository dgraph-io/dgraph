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
	"sync"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

// TokensTable tracks the keys / tokens / buckets for an indexed attribute.
type TokensTable struct {
	sync.RWMutex
	key []string
}

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
	sv := types.ValueForType(s)
	err = types.Convert(src, &sv)
	if err != nil {
		return nil, err
	}
	return types.IndexTokens(sv)
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

	l.Lock()
	defer l.Unlock()

	doUpdateIndex := pstore != nil && (t.Value != nil) && schema.IsIndexed(t.Attr)
	if doUpdateIndex {
		// Check last posting for original value BEFORE any mutation actually happens.
		val, verr = l.value()
	}
	hasMutated, err := l.addMutation(ctx, t)
	if err != nil {
		return err
	}
	if !hasMutated {
		return nil
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

// TokensForTest returns keys for a table. This is just for testing / debugging.
func TokensForTest(attr string) []string {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	it := pstore.NewIterator()
	defer it.Close()

	var out []string
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		k := x.Parse(it.Key().Data())
		x.AssertTrue(k.IsIndex())
		out = append(out, k.Term)
	}
	return out
}
