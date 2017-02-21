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

	"github.com/dgraph-io/dgraph/algo"
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	maxBatchSize        = 32 * (1 << 20)
	updateIndexInterval = time.Second * 10
)

var (
	indexLog        trace.EventLog
	reverseLog      trace.EventLog
	updateIndexChan chan struct{} // Push here to force an updateIndex.
)

func init() {
	indexLog = trace.NewEventLog("index", "Logger")
	reverseLog = trace.NewEventLog("reverse", "Logger")
	updateIndexChan = make(chan struct{}, 5)
}

// ForceUpdateIndex forces updateIndex.
func ForceUpdateIndex() {
	updateIndexChan <- struct{}{}
}

type indexUpdater struct {
	indexPl *List
	attr    string
	groupID uint32
	term    string
}

func (s *indexUpdater) processUID(ctx context.Context, uid uint64) error {
	pl, decr := GetOrCreate(x.DataKey(s.attr, uid), s.groupID)
	defer decr()
	v, err := pl.Value()
	if err == nil {
		// Tokenize v and check if it matches term.
		tokens, err := IndexTokens(s.attr, v)
		if err != nil {
			return err
		}
		for _, tok := range tokens {
			if tok == s.term {
				return nil
			}
		}
	}

	// Delete uid from index.
	edge := task.DirectedEdge{
		ValueId: uid,
		Attr:    s.attr,
		Label:   "idx",
		Op:      task.DirectedEdge_DEL,
	}
	_, err = s.indexPl.AddMutation(ctx, &edge)
	return err
}

func (s *indexUpdater) processIndexKey(ctx context.Context, indexKey []byte) error {
	var decr func()
	s.indexPl, decr = GetOrCreate(indexKey, s.groupID) // Get index data.
	defer decr()

	var opts ListOptions
	uids := s.indexPl.Uids(opts)
	pki := x.Parse(indexKey)
	x.AssertTrue(pki.IsIndex())
	s.term = pki.Term
	x.AssertTrue(len(s.term) > 0)

	it := algo.NewListIterator(uids)
	for ; it.Valid(); it.Next() {
		uid := it.Val()
		if err := s.processUID(ctx, uid); err != nil {
			return err
		}
	}
	return nil
}

func (s *indexUpdater) run() {
	indexLog.Printf("indexUpdater run")
	it := pstore.NewIterator()
	defer it.Close()
	var pk x.ParsedKey
	ctx := context.Background()
	for _, attr := range schema.State().IndexedFields() {
		pk.Attr = attr
		prefix := pk.IndexPrefix()
		s.attr = attr
		s.groupID = group.BelongsTo(attr)
		// Iterate over data store just to get the index keys.
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if err := s.processIndexKey(ctx, it.Key().Data()); err != nil {
				indexLog.Errorf("Error updating index for attr %s: %v", attr, err)
			}
		}
	}
}

// updateIndex iterates over index entries and delete invalid entries.
func updateIndex() {
	// Wait for group config to be ready. We do it by simple polling. It is a one-off.
	for !group.Ready() {
		time.Sleep(100 * time.Millisecond)
	}
	tickC := time.Tick(updateIndexInterval)
	var updater indexUpdater
	for {
		select {
		case <-tickC:
			updater.run() // select has no fallthrough.
		case <-updateIndexChan:
			updater.run()
		}
	}
}

// IndexTokens return tokens, without the predicate prefix and index rune.
func IndexTokens(attr string, src types.Val) ([]string, error) {
	schemaType, err := schema.State().TypeOf(attr)
	if err != nil || !schemaType.IsScalar() {
		return nil, x.Errorf("Cannot index attribute %s of type object.", attr)
	}
	s := schemaType
	sv, err := types.Convert(src, s)
	if err != nil {
		return nil, err
	}
	// Schema will know the mapping from attr to tokenizer.
	return schema.State().Tokenizer(attr).Tokens(sv)
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

	l.index.Lock()
	defer l.index.Unlock()

	l.Lock()
	hasMutated, err := l.addMutation(ctx, t)
	l.Unlock()
	if err != nil || !hasMutated {
		return err
	}

	// We only add index mutations, but not delete here.
	if pstore != nil && (t.Value != nil) && t.Op == task.DirectedEdge_SET &&
		schema.State().IsIndexed(t.Attr) {
		p := types.Val{
			Tid:   types.TypeID(t.ValueType),
			Value: t.Value,
		}
		addIndexMutations(ctx, t, p, task.DirectedEdge_SET)
	}

	if (pstore != nil) && (t.ValueId != 0) && schema.State().IsReversed(t.Attr) {
		addReverseMutation(ctx, t)
	}
	return nil
}

// RebuildIndex rebuilds index for a given attribute.
func RebuildIndex(ctx context.Context, attr string) error {
	x.AssertTruef(schema.State().IsIndexed(attr), "Attr %s not indexed", attr)

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
		// TODO(tzdybal) - what about the values with Lang?
		p := pl.Postings[len(pl.Postings)-1]
		pt := postingType(p)
		if pt != valueTagged && pt != valueUntagged {
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
