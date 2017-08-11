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
	"bytes"
	"context"
	"fmt"
	"math"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const maxBatchSize = 32 * (1 << 20)

// IndexTokens return tokens, without the predicate prefix and index rune.
func IndexTokens(attr, lang string, src types.Val) ([]string, error) {
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
		if tok.FtsTokenizerName("") == it.Name() && len(lang) > 0 {
			newTokenizer, ok := tok.GetTokenizer(tok.FtsTokenizerName(lang))
			if ok {
				it = newTokenizer
			} else {
				return nil, x.Errorf("Tokenizer not available for language: %s", lang)
			}
		}
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
// TODO - See if we need to pass op as argument as t should already have Op.
func addIndexMutations(ctx context.Context, t *protos.DirectedEdge, p types.Val,
	op protos.DirectedEdge_Op) error {
	attr := t.Attr
	uid := t.Entity
	x.AssertTrue(uid != 0)
	tokens, err := IndexTokens(attr, t.GetLang(), p)

	if err != nil {
		// This data is not indexable
		return err
	}

	// Create a value token -> uid edge.
	edge := &protos.DirectedEdge{
		ValueId: uid,
		Attr:    attr,
		Op:      op,
	}

	for _, token := range tokens {
		if err := addIndexMutation(ctx, edge, token); err != nil {
			return err
		}
	}
	return nil
}

func addIndexMutation(ctx context.Context, edge *protos.DirectedEdge,
	token string) error {
	key := x.IndexKey(edge.Attr, token)
	var groupId uint32
	if rv, ok := ctx.Value("raft").(x.RaftValue); ok {
		groupId = rv.Group
	}
	if groupId == 0 {
		groupId = group.BelongsTo(edge.Attr)
	}

	t := time.Now()
	plist := GetOrCreate(key, groupId)
	if dur := time.Since(t); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("GetOrCreate took %v", dur)
		}
	}

	x.AssertTrue(plist != nil)
	_, err := plist.AddMutation(ctx, edge)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error adding/deleting %s for attr %s entity %d: %v",
				token, edge.Attr, edge.Entity, err)
		}
		return err
	}
	x.PredicateStats.Add(fmt.Sprintf("i.%s", edge.Attr), 1)
	return nil
}

// countParams is sent to updateCount function. It is used to update the count index.
// It deletes the uid from the key corresponding to <attr, countBefore> and adds it
// to <attr, countAfter>.
type countParams struct {
	attr        string
	countBefore int
	countAfter  int
	entity      uint64
	reverse     bool
}

func addReverseMutation(ctx context.Context, t *protos.DirectedEdge) error {
	key := x.ReverseKey(t.Attr, t.ValueId)
	groupId := group.BelongsTo(t.Attr)

	plist := GetOrCreate(key, groupId)

	x.AssertTrue(plist != nil)
	edge := &protos.DirectedEdge{
		Entity:  t.ValueId,
		ValueId: t.Entity,
		Attr:    t.Attr,
		Op:      t.Op,
		Facets:  t.Facets,
	}

	countBefore, countAfter := 0, 0
	hasCountIndex := schema.State().HasCount(t.Attr)
	plist.Lock()
	if hasCountIndex {
		countBefore = plist.length(0)
	}
	_, err := plist.addMutation(ctx, edge)
	if hasCountIndex {
		countAfter = plist.length(0)
	}
	plist.Unlock()
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error adding/deleting reverse edge for attr %s entity %d: %v",
				t.Attr, t.Entity, err)
		}
		return err
	}
	x.PredicateStats.Add(fmt.Sprintf("r.%s", edge.Attr), 1)

	if hasCountIndex && countAfter != countBefore {
		if err := updateCount(ctx, countParams{
			attr:        t.Attr,
			countBefore: countBefore,
			countAfter:  countAfter,
			entity:      edge.Entity,
			reverse:     true,
		}); err != nil {
			return err
		}
	}
	return nil
}
func (l *List) handleDeleteAll(ctx context.Context, t *protos.DirectedEdge) error {
	isReversed := schema.State().IsReversed(t.Attr)
	isIndexed := schema.State().IsIndexed(t.Attr)
	hasCount := schema.State().HasCount(t.Attr)
	delEdge := &protos.DirectedEdge{
		Attr:   t.Attr,
		Op:     t.Op,
		Entity: t.Entity,
	}
	// To calculate length of posting list. Used for deletion of count index.
	var plen int
	var iterErr error
	l.Iterate(0, func(p *protos.Posting) bool {
		plen++
		if isReversed {
			// Delete reverse edge for each posting.
			delEdge.ValueId = p.Uid
			if err := addReverseMutation(ctx, delEdge); err != nil {
				iterErr = err
				return false
			}
			return true
		} else if isIndexed {
			// Delete index edge of each posting.
			p := types.Val{
				Tid:   types.TypeID(p.ValType),
				Value: p.Value,
			}
			if err := addIndexMutations(ctx, t, p, protos.DirectedEdge_DEL); err != nil {
				iterErr = err
				return false
			}
		}
		return true
	})
	if iterErr != nil {
		return iterErr
	}
	if hasCount {
		// Delete uid from count index. Deletion of reverses is taken care by addReverseMutation
		// above.
		if err := updateCount(ctx, countParams{
			attr:        t.Attr,
			countBefore: plen,
			countAfter:  0,
			entity:      t.Entity,
		}); err != nil {
			return err
		}
	}

	l.Lock()
	defer l.Unlock()
	return l.delete(ctx, t.Attr)
}

func addCountMutation(ctx context.Context, t *protos.DirectedEdge, count uint32,
	reverse bool) error {
	key := x.CountKey(t.Attr, count, reverse)
	groupId := group.BelongsTo(t.Attr)

	plist := GetOrCreate(key, groupId)

	x.AssertTruef(plist != nil, "plist is nil [%s] %d",
		t.Attr, t.ValueId)
	_, err := plist.AddMutation(ctx, t)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error adding/deleting count edge for attr %s count %d dst %d: %v",
				t.Attr, count, t.ValueId, err)
		}
		return err
	}
	x.PredicateStats.Add(fmt.Sprintf("c.%s", t.Attr), 1)
	return nil

}

func updateCount(ctx context.Context, params countParams) error {
	edge := protos.DirectedEdge{
		ValueId: params.entity,
		Attr:    params.attr,
		Op:      protos.DirectedEdge_DEL,
	}
	if err := addCountMutation(ctx, &edge, uint32(params.countBefore),
		params.reverse); err != nil {
		return err
	}

	edge.Op = protos.DirectedEdge_SET
	if err := addCountMutation(ctx, &edge, uint32(params.countAfter),
		params.reverse); err != nil {
		return err
	}
	return nil
}

// AddMutationWithIndex is AddMutation with support for indexing. It also
// supports reverse edges.
func (l *List) AddMutationWithIndex(ctx context.Context, t *protos.DirectedEdge) error {
	x.AssertTruef(len(t.Attr) > 0,
		"[%s] [%d] [%v] %d %d\n", t.Attr, t.Entity, t.Value, t.ValueId, t.Op)

	var val types.Val
	var found bool

	t1 := time.Now()
	l.index.Lock()
	if dur := time.Since(t1); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("acquired index lock %v %v %v", dur, t.Attr, t.Entity)
		}
	}
	defer l.index.Unlock()

	if t.Op == protos.DirectedEdge_DEL && string(t.Value) == x.Star {
		return l.handleDeleteAll(ctx, t)
	}

	doUpdateIndex := pstore != nil && (t.Value != nil) && schema.State().IsIndexed(t.Attr)
	{
		t1 = time.Now()
		l.Lock()
		if dur := time.Since(t1); dur > time.Millisecond {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("acquired lock %v %v %v", dur, t.Attr, t.Entity)
			}
		}

		if doUpdateIndex {
			// Check original value BEFORE any mutation actually happens.
			if len(t.Lang) > 0 {
				val, found = l.findValue(farm.Fingerprint64([]byte(t.Lang)))
			} else {
				val, found = l.findValue(math.MaxUint64)
			}
		}
		countBefore := l.length(0)
		_, err := l.addMutation(ctx, t)
		countAfter := l.length(0)
		l.Unlock()

		if err != nil {
			return err
		}
		x.PredicateStats.Add(t.Attr, 1)
		if countAfter != countBefore && schema.State().HasCount(t.Attr) {
			if err := updateCount(ctx, countParams{
				attr:        t.Attr,
				countBefore: countBefore,
				countAfter:  countAfter,
				entity:      t.Entity,
			}); err != nil {
				return err
			}
		}
	}
	// We should always set index set and we can take care of stale indexes in
	// eventual index consistency
	if doUpdateIndex {
		// Exact matches.
		if found && val.Value != nil {
			addIndexMutations(ctx, t, val, protos.DirectedEdge_DEL)
		}
		if t.Op == protos.DirectedEdge_SET {
			p := types.Val{
				Tid:   types.TypeID(t.ValueType),
				Value: t.Value,
			}
			addIndexMutations(ctx, t, p, protos.DirectedEdge_SET)
		}
	}
	// Add reverse mutation irrespective of hasMutated, server crash can happen after
	// mutation is synced and before reverse edge is synced
	if (pstore != nil) && (t.ValueId != 0) && schema.State().IsReversed(t.Attr) {
		addReverseMutation(ctx, t)
	}
	return nil
}

func deleteEntries(prefix []byte) error {
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.FetchValues = false
	idxIt := pstore.NewIterator(iterOpt)
	defer idxIt.Close()

	wb := make([]*badger.Entry, 0, 100)
	var batchSize int
	for idxIt.Seek(prefix); idxIt.ValidForPrefix(prefix); idxIt.Next() {
		key := idxIt.Item().Key()
		data := make([]byte, len(key))
		copy(data, key)
		batchSize += len(key)
		wb = badger.EntriesDelete(wb, data)

		if batchSize >= maxBatchSize {
			if err := pstore.BatchSet(wb); err != nil {
				return err
			}
			wb = wb[:0]
			batchSize = 0
		}
	}
	if len(wb) > 0 {
		if err := pstore.BatchSet(wb); err != nil {
			return err
		}
	}
	return nil
}

func DeleteReverseEdges(ctx context.Context, attr string) error {
	if err := lcache.clear(attr, x.ByteReverse); err != nil {
		return err
	}
	// Delete index entries from data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.ReversePrefix()
	if err := deleteEntries(prefix); err != nil {
		return err
	}
	return nil
}

func deleteCountIndex(ctx context.Context, attr string, reverse bool) error {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.CountPrefix(reverse)
	if err := deleteEntries(prefix); err != nil {
		return err
	}
	return nil
}

func DeleteCountIndex(ctx context.Context, attr string) error {
	if err := lcache.clear(attr, x.ByteCount); err != nil {
		return err
	}
	if err := lcache.clear(attr, x.ByteCountRev); err != nil {
		return err
	}
	// Delete index entries from data store.
	if err := deleteCountIndex(ctx, attr, false); err != nil {
		return err
	}
	if err := deleteCountIndex(ctx, attr, true); err != nil { // delete reverse count indexes.
		return err
	}
	return nil
}

func rebuildCountIndex(ctx context.Context, attr string, reverse bool, errCh chan error) {
	ch := make(chan item, 10000)
	che := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		go func() {
			var err error
			for it := range ch {
				pl := it.list
				t := &protos.DirectedEdge{
					ValueId: it.uid,
					Attr:    attr,
					Op:      protos.DirectedEdge_SET,
				}
				if err = addCountMutation(ctx, t, uint32(len(pl.Uids)/8), reverse); err != nil {
					break
				}
			}
			che <- err
		}()
	}

	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	if reverse {
		prefix = pk.ReversePrefix()
	}

	it := pstore.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		iterItem := it.Item()
		key := iterItem.Key()
		pki := x.Parse(key)
		var pl protos.PostingList
		UnmarshalWithCopy(iterItem.Value(), iterItem.UserMeta(), &pl)

		ch <- item{
			uid:  pki.Uid,
			list: &pl,
		}
	}
	close(ch)
	var finalErr error
	for i := 0; i < 1000; i++ {
		if err := <-che; err != nil {
			finalErr = err
		}
	}
	errCh <- finalErr
}

func RebuildCountIndex(ctx context.Context, attr string) error {
	x.AssertTruef(schema.State().HasCount(attr), "Attr %s doesn't have count index", attr)
	errCh := make(chan error, 2)
	// Lets rebuild forward and reverse count indexes concurrently.
	go rebuildCountIndex(ctx, attr, false, errCh)
	go rebuildCountIndex(ctx, attr, true, errCh)

	var rebuildErr error
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				rebuildErr = err
			}
		case <-ctx.Done():
			rebuildErr = ctx.Err()
		}
	}
	return rebuildErr
}

type item struct {
	uid  uint64
	list *protos.PostingList
}

// RebuildReverseEdges rebuilds the reverse edges for a given attribute.
func RebuildReverseEdges(ctx context.Context, attr string) error {
	x.AssertTruef(schema.State().IsReversed(attr), "Attr %s doesn't have reverse", attr)
	// Add index entries to data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	it := pstore.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	// Helper function - Add reverse entries for values in posting list
	addReversePostings := func(uid uint64, pl *protos.PostingList) error {
		var pitr PIterator
		pitr.Init(pl, 0)
		edge := protos.DirectedEdge{Attr: attr, Entity: uid}
		for ; pitr.Valid(); pitr.Next() {
			pp := pitr.Posting()
			puid := pp.Uid
			// Add reverse entries based on p.
			edge.ValueId = puid
			edge.Op = protos.DirectedEdge_SET
			edge.Facets = pp.Facets
			edge.Label = pp.Label
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

	for it.Seek(prefix); it.Valid(); it.Next() {
		iterItem := it.Item()
		key := iterItem.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		pki := x.Parse(key)
		var pl protos.PostingList
		UnmarshalWithCopy(iterItem.Value(), iterItem.UserMeta(), &pl)

		// Posting list contains only values or only UIDs.
		if (len(pl.Postings) == 0 && len(pl.Uids) != 0) ||
			postingType(pl.Postings[0]) == x.ValueUid {
			ch <- item{
				uid:  pki.Uid,
				list: &pl,
			}
		}
	}
	close(ch)
	for i := 0; i < 1000; i++ {
		if err := <-che; err != nil {
			return err
		}
	}
	return nil
}

func DeleteIndex(ctx context.Context, attr string) error {
	if err := lcache.clear(attr, x.ByteIndex); err != nil {
		return err
	}
	// Delete index entries from data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	if err := deleteEntries(prefix); err != nil {
		return err
	}
	return nil
}

// RebuildIndex rebuilds index for a given attribute.
func RebuildIndex(ctx context.Context, attr string) error {
	x.AssertTruef(schema.State().IsIndexed(attr), "Attr %s not indexed", attr)
	// Add index entries to data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	it := pstore.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()

	// Helper function - Add index entries for values in posting list
	addPostingsToIndex := func(uid uint64, pl *protos.PostingList) error {
		postingsLen := len(pl.Postings)
		edge := protos.DirectedEdge{Attr: attr, Entity: uid}
		for idx := 0; idx < postingsLen; idx++ {
			p := pl.Postings[idx]
			// Add index entries based on p.
			val := types.Val{
				Value: p.Value,
				Tid:   types.TypeID(p.ValType),
			}
			err := addIndexMutations(ctx, &edge, val, protos.DirectedEdge_SET)
			// We retry once in case we do GetOrCreate and stop the world happens
			// before we do addmutation
			if err == ErrRetry {
				err = addIndexMutations(ctx, &edge, val, protos.DirectedEdge_SET)
			}
			if err != nil {
				return err
			}
		}
		return nil
	}

	type item struct {
		uid  uint64
		list *protos.PostingList
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

	for it.Seek(prefix); it.Valid(); it.Next() {
		iterItem := it.Item()
		key := iterItem.Key()
		if !bytes.HasPrefix(key, prefix) {
			break
		}
		pki := x.Parse(key)
		var pl protos.PostingList
		UnmarshalWithCopy(iterItem.Value(), iterItem.UserMeta(), &pl)

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
		if err := <-che; err != nil {
			return err
		}
	}
	return nil
}

func DeletePredicate(ctx context.Context, attr string) error {
	if err := lcache.clear(attr, x.ByteData); err != nil {
		return err
	}
	pk := x.ParsedKey{
		Attr: attr,
	}
	prefix := pk.DataPrefix()
	// Delete all data postings for the given predicate.
	if err := deleteEntries(prefix); err != nil {
		return err
	}

	// TODO - We will still have the predicate present in <uid, _predicate_> posting lists.
	indexed := schema.State().IsIndexed(attr)
	reversed := schema.State().IsReversed(attr)
	if indexed {
		if err := DeleteIndex(ctx, attr); err != nil {
			return err
		}
	} else if reversed {
		if err := DeleteReverseEdges(ctx, attr); err != nil {
			return err
		}
	}

	if ok := schema.State().HasCount(attr); ok {
		if err := DeleteCountIndex(ctx, attr); err != nil {
			return err
		}
	}
	return nil
}
