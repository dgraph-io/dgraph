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

package posting

import (
	"context"
	"math"

	"golang.org/x/net/trace"

	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/schema"
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
	schemaType, err := schema.State().TypeOf(attr)
	if err != nil || !schemaType.IsScalar() {
		return nil, x.Errorf("Cannot index attribute %s of type object.", attr)
	}

	if !schema.State().IsIndexed(attr) {
		return nil, x.Errorf("Attribute %s is not indexed.", attr)
	}
	s := schemaType
	sv, err := types.Convert(src, s)
	if err != nil {
		return nil, err
	}
	// Schema will know the mapping from attr to tokenizer.
	var tokens []string
	tokenizers := schema.State().Tokenizer(attr)
	for _, it := range tokenizers {
		toks, err := it.Tokens(sv)
		if err != nil {
			return tokens, err
		}
		tokens = append(tokens, toks...)
	}
	return tokens, nil
}

// addIndexMutations adds mutation(s) for a single term, to maintain index.
// t represents the original uid -> value edge.
func addIndexMutations(ctx context.Context, t *taskp.DirectedEdge, p types.Val,
	op taskp.DirectedEdge_Op) error {
	attr := t.Attr
	uid := t.Entity
	x.AssertTrue(uid != 0)
	tokens, err := IndexTokens(attr, p)
	if err != nil {
		// This data is not indexable
		return err
	}

	// Create a value token -> uid edge.
	edge := &taskp.DirectedEdge{
		ValueId: uid,
		Attr:    attr,
		Label:   "idx",
		Op:      op,
	}

	for _, token := range tokens {
		if err := addIndexMutation(ctx, edge, token); err != nil {
			return err
		}
	}
	return nil
}

func addIndexMutation(ctx context.Context, edge *taskp.DirectedEdge,
	token string) error {
	key := x.IndexKey(edge.Attr, token)

	var groupId uint32
	if rv, ok := ctx.Value("raft").(x.RaftValue); ok {
		groupId = rv.Group
	}
	if groupId == 0 {
		groupId = group.BelongsTo(edge.Attr)
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
		return err
	}
	indexLog.Printf("%s [%s] [%d] Term [%s]",
		edge.Op, edge.Attr, edge.Entity, token)
	return nil
}

func addReverseMutation(ctx context.Context, t *taskp.DirectedEdge) error {
	key := x.ReverseKey(t.Attr, t.ValueId)
	groupId := group.BelongsTo(t.Attr)

	plist, decr := GetOrCreate(key, groupId)
	defer decr()

	x.AssertTruef(plist != nil, "plist is nil [%s] %d %d",
		t.Attr, t.Entity, t.ValueId)
	edge := &taskp.DirectedEdge{
		Entity:  t.ValueId,
		ValueId: t.Entity,
		Attr:    t.Attr,
		Label:   "rev",
		Op:      t.Op,
		Facets:  t.Facets,
	}

	_, err := plist.AddMutation(ctx, edge)
	if err != nil {
		x.TraceError(ctx, x.Wrapf(err,
			"Error adding/deleting reverse edge for attr %s src %d dst %d",
			t.Attr, t.Entity, t.ValueId))
		return err
	}
	reverseLog.Printf("%s [%s] [%d] [%d]", t.Op, t.Attr, t.Entity, t.ValueId)
	return nil
}

// AddMutationWithIndex is AddMutation with support for indexing. It also
// supports reverse edges.
func (l *List) AddMutationWithIndex(ctx context.Context, t *taskp.DirectedEdge) error {
	x.AssertTruef(len(t.Attr) > 0,
		"[%s] [%d] [%v] %d %d\n", t.Attr, t.Entity, t.Value, t.ValueId, t.Op)

	var val types.Val
	var found bool

	l.index.Lock()
	defer l.index.Unlock()

	doUpdateIndex := pstore != nil && (t.Value != nil) && schema.State().IsIndexed(t.Attr)
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
		_, err := l.addMutation(ctx, t)
		l.Unlock()

		if err != nil {
			return err
		}
	}
	// We should always set index set and we can take care of stale indexes in
	// eventual index consistency
	if doUpdateIndex {
		// Exact matches.
		if found && val.Value != nil {
			addIndexMutations(ctx, t, val, taskp.DirectedEdge_DEL)
		}
		if t.Op == taskp.DirectedEdge_SET {
			p := types.Val{
				Tid:   types.TypeID(t.ValueType),
				Value: t.Value,
			}
			addIndexMutations(ctx, t, p, taskp.DirectedEdge_SET)
		}
	}
	// Add reverse mutation irrespective of hashMutated, server crash can happen after
	// mutation is synced and before reverse edge is synced
	if (pstore != nil) && (t.ValueId != 0) && schema.State().IsReversed(t.Attr) {
		addReverseMutation(ctx, t)
	}
	return nil
}

func DeleteReverseEdges(ctx context.Context, attr string) error {
	// Delete index entries from data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.ReversePrefix()
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
	return nil
}

// RebuildReverseEdges rebuilds the reverse edges for a given attribute.
func RebuildReverseEdges(ctx context.Context, attr string) error {
	x.AssertTruef(schema.State().IsReversed(attr), "Attr %s doesn't have reverse", attr)
	if err := DeleteReverseEdges(ctx, attr); err != nil {
		return err
	}

	// Add index entries to data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	it := pstore.NewIterator()
	defer it.Close()

	EvictGroup(group.BelongsTo(attr))
	// Helper function - Add reverse entries for values in posting list
	addReversePostings := func(uid uint64, pl *typesp.PostingList) error {
		postingsLen := len(pl.Postings)
		edge := taskp.DirectedEdge{Attr: attr, Entity: uid}
		for idx := 0; idx < postingsLen; idx++ {
			p := pl.Postings[idx]
			// Add reverse entries based on p.
			edge.ValueId = p.Uid
			edge.Op = taskp.DirectedEdge_SET
			edge.Facets = p.Facets
			edge.Label = p.Label
			err := addReverseMutation(ctx, &edge)
			// We retry once in case we do GetOrCreate and stop the world happens
			// before we do addmutation
			if err == ErrRetry {
				err = addReverseMutation(ctx, &edge)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}

	type item struct {
		uid  uint64
		list *typesp.PostingList
	}
	ch := make(chan item, 10000)
	che := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		go func() {
			var err error
			for it := range ch {
				err = addReversePostings(it.uid, it.list)
				if err != nil {
					break
				}
			}
			che <- err
		}()
	}

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		pki := x.Parse(it.Key().Data())
		var pl typesp.PostingList
		x.Check(pl.Unmarshal(it.Value().Data()))

		// Posting list contains only values or only UIDs.
		if len(pl.Postings) != 0 && postingType(pl.Postings[0]) == x.ValueUid {
			ch <- item{
				uid:  pki.Uid,
				list: &pl,
			}
		}
	}
	close(ch)
	for i := 0; i < 1000; i++ {
		select {
		case err := <-che:
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func DeleteIndex(ctx context.Context, attr string) error {
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
	return nil
}

// RebuildIndex rebuilds index for a given attribute.
func RebuildIndex(ctx context.Context, attr string) error {
	x.AssertTruef(schema.State().IsIndexed(attr), "Attr %s not indexed", attr)
	if err := DeleteIndex(ctx, attr); err != nil {
		return err
	}

	// Add index entries to data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	it := pstore.NewIterator()
	defer it.Close()

	EvictGroup(group.BelongsTo(attr))
	// Helper function - Add index entries for values in posting list
	addPostingsToIndex := func(uid uint64, pl *typesp.PostingList) error {
		postingsLen := len(pl.Postings)
		edge := taskp.DirectedEdge{Attr: attr, Entity: uid}
		for idx := 0; idx < postingsLen; idx++ {
			p := pl.Postings[idx]
			// Add index entries based on p.
			val := types.Val{
				Value: p.Value,
				Tid:   types.TypeID(p.ValType),
			}
			err := addIndexMutations(ctx, &edge, val, taskp.DirectedEdge_SET)
			// We retry once in case we do GetOrCreate and stop the world happens
			// before we do addmutation
			if err == ErrRetry {
				err = addIndexMutations(ctx, &edge, val, taskp.DirectedEdge_SET)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}

	type item struct {
		uid  uint64
		list *typesp.PostingList
	}
	ch := make(chan item, 10000)
	che := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		go func() {
			var err error
			for it := range ch {
				err = addPostingsToIndex(it.uid, it.list)
				if err != nil {
					break
				}
			}
			che <- err
		}()
	}

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		pki := x.Parse(it.Key().Data())
		var pl typesp.PostingList
		x.Check(pl.Unmarshal(it.Value().Data()))

		// Posting list contains only values or only UIDs.
		if len(pl.Postings) != 0 && postingType(pl.Postings[0]) != x.ValueUid {
			ch <- item{
				uid:  pki.Uid,
				list: &pl,
			}
		}
	}
	close(ch)
	for i := 0; i < 1000; i++ {
		select {
		case err := <-che:
			if err != nil {
				return err
			}
		}
	}
	return nil
}
