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
	"math"
	"sort"

	"golang.org/x/net/trace"

	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
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
	// Schema will know the mapping from attr to tokenizer.
	return schema.Tokenizer(attr).Tokens(sv)
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
	var found bool

	l.index.Lock()
	defer l.index.Unlock()

	doUpdateIndex := pstore != nil && (t.Value != nil) && schema.IsIndexed(t.Attr)
	{
		l.Lock()
		if doUpdateIndex {
			// Check original value BEFORE any mutation actually happens.
			if len(t.Lang) > 0 {
				found, val = l.findValue(farm.Fingerprint64([]byte(t.Lang)))
			} else {
				found, val = l.findValue(math.MaxUint64)
			}
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
		if found && val.Value != nil {
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

	addPostingToIndex := func(idx int) {
		p := pl.Postings[idx]
		pt := postingType(p)
		if pt == valueTagged || pt == valueUntagged {
			// Add index entries based on p.
			val := types.Val{
				Value: p.Value,
				Tid:   types.TypeID(p.ValType),
			}
			edge.Lang = p.Lang
			addIndexMutations(ctx, &edge, val, task.DirectedEdge_SET)
		}
	}

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		pki := x.Parse(it.Key().Data())
		edge.Entity = pki.Uid
		x.Check(pl.Unmarshal(it.Value().Data()))

		postingLen := len(pl.Postings)
		if postingLen == 0 {
			continue
		}
		for _, lang := range pl.Langs {
			langUid := farm.Fingerprint64([]byte(lang))
			idx := sort.Search(postingLen, func(i int) bool {
				p := pl.Postings[i]
				return langUid < p.Uid
			})

			if pl.Postings[idx].Lang == lang {
				addPostingToIndex(idx)
			}
		}
		addPostingToIndex(len(pl.Postings) - 1) // value without language
	}
	return nil
}
