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

	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const maxBatchSize = 32 * (1 << 20)

// IndexTokens return tokens, without the predicate prefix and index rune.
func indexTokens(attr, lang string, src types.Val) ([]string, error) {
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
	op protos.DirectedEdge_Op, txn *Txn) error {
	attr := t.Attr
	uid := t.Entity
	x.AssertTrue(uid != 0)
	tokens, err := indexTokens(attr, t.GetLang(), p)

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
		if err := addIndexMutation(ctx, edge, token, txn); err != nil {
			return err
		}
	}
	return nil
}

func addIndexMutation(ctx context.Context, edge *protos.DirectedEdge,
	token string, txn *Txn) error {
	key := x.IndexKey(edge.Attr, token)

	t := time.Now()
	plist := getOrMutate(key)
	if dur := time.Since(t); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("getOrMutate took %v", dur)
		}
	}

	x.AssertTrue(plist != nil)
	_, err := plist.AddMutation(ctx, txn, edge)
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

func addReverseMutation(ctx context.Context, t *protos.DirectedEdge, txn *Txn) error {
	key := x.ReverseKey(t.Attr, t.ValueId)
	plist := Get(key)

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
		countBefore = plist.length(txn.StartTs, 0)
	}
	_, err := plist.addMutation(ctx, txn, edge)
	if hasCountIndex {
		countAfter = plist.length(txn.StartTs, 0)
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
		}, txn); err != nil {
			return err
		}
	}
	return nil
}
func (l *List) handleDeleteAll(ctx context.Context, t *protos.DirectedEdge,
	txn *Txn) error {
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
	l.Iterate(0, 0, func(p *protos.Posting) bool {
		plen++
		if isReversed {
			// Delete reverse edge for each posting.
			delEdge.ValueId = p.Uid
			if err := addReverseMutation(ctx, delEdge, txn); err != nil {
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
			if err := addIndexMutations(ctx, t, p, protos.DirectedEdge_DEL, txn); err != nil {
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
		}, txn); err != nil {
			return err
		}
	}

	l.Lock()
	defer l.Unlock()
	_, err := l.addMutation(ctx, txn, t)
	return err
}

func addCountMutation(ctx context.Context, t *protos.DirectedEdge, count uint32,
	reverse bool, txn *Txn) error {
	key := x.CountKey(t.Attr, count, reverse)
	plist := getOrMutate(key)

	x.AssertTruef(plist != nil, "plist is nil [%s] %d",
		t.Attr, t.ValueId)
	_, err := plist.AddMutation(ctx, txn, t)
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

func updateCount(ctx context.Context, params countParams, txn *Txn) error {
	edge := protos.DirectedEdge{
		ValueId: params.entity,
		Attr:    params.attr,
		Op:      protos.DirectedEdge_DEL,
	}
	if err := addCountMutation(ctx, &edge, uint32(params.countBefore),
		params.reverse, txn); err != nil {
		return err
	}

	if params.countAfter > 0 {
		edge.Op = protos.DirectedEdge_SET
		if err := addCountMutation(ctx, &edge, uint32(params.countAfter),
			params.reverse, txn); err != nil {
			return err
		}
	}
	return nil
}

// AddMutationWithIndex is AddMutation with support for indexing. It also
// supports reverse edges.
func (l *List) AddMutationWithIndex(ctx context.Context, t *protos.DirectedEdge,
	txn *Txn) error {
	if len(t.Attr) == 0 {
		return x.Errorf("Predicate cannot be empty for edge with subject: [%v], object: [%v]"+
			" and value: [%v]", t.Entity, t.ValueId, t.Value)
	}

	var val types.Val
	var found bool

	t1 := time.Now()

	if t.Op == protos.DirectedEdge_DEL && string(t.Value) == x.Star {
		return l.handleDeleteAll(ctx, t, txn)
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
				val, found = l.findValue(0, farm.Fingerprint64([]byte(t.Lang)))
			} else {
				val, found = l.findValue(0, math.MaxUint64)
			}
		}
		countBefore, countAfter := 0, 0
		hasCountIndex := schema.State().HasCount(t.Attr)
		if hasCountIndex {
			countBefore = l.length(txn.StartTs, 0)
		}
		_, err := l.addMutation(ctx, txn, t)
		if hasCountIndex {
			countAfter = l.length(txn.StartTs, 0)
		}
		l.Unlock()

		if err != nil {
			return err
		}
		x.PredicateStats.Add(t.Attr, 1)
		if hasCountIndex && countAfter != countBefore {
			if err := updateCount(ctx, countParams{
				attr:        t.Attr,
				countBefore: countBefore,
				countAfter:  countAfter,
				entity:      t.Entity,
			}, txn); err != nil {
				return err
			}
		}
	}
	// We should always set index set and we can take care of stale indexes in
	// eventual index consistency
	if doUpdateIndex {
		// Exact matches.
		if found && val.Value != nil {
			if err := addIndexMutations(ctx, t, val, protos.DirectedEdge_DEL, txn); err != nil {
				return err
			}
		}
		if t.Op == protos.DirectedEdge_SET {
			p := types.Val{
				Tid:   types.TypeID(t.ValueType),
				Value: t.Value,
			}
			if err := addIndexMutations(ctx, t, p, protos.DirectedEdge_SET, txn); err != nil {
				return err
			}
		}
	}
	// Add reverse mutation irrespective of hasMutated, server crash can happen after
	// mutation is synced and before reverse edge is synced
	if (pstore != nil) && (t.ValueId != 0) && schema.State().IsReversed(t.Attr) {
		if err := addReverseMutation(ctx, t, txn); err != nil {
			return err
		}
	}
	return nil
}

// TODO: Write commit markers also
func deleteEntries(prefix []byte, readTs uint64) error {
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.PrefetchValues = false
	txn := pstore.NewTransactionAt(readTs, false)
	defer txn.Discard()
	delTxn := pstore.NewTransaction(true)
	idxIt := txn.NewIterator(iterOpt)
	defer idxIt.Close()

	var batchSize int
	for idxIt.Seek(prefix); idxIt.ValidForPrefix(prefix); idxIt.Next() {
		key := idxIt.Item().Key()
		data := make([]byte, len(key))
		copy(data, key)
		batchSize += len(key)
		err := delTxn.Set(data, nil, 0x00)
		x.Check(err)

		if batchSize >= maxBatchSize {
			if err := delTxn.Commit(nil); err != nil {
				return err
			}
			delTxn.Discard()
			delTxn = pstore.NewTransaction(true)
			batchSize = 0
		}
	}
	defer delTxn.Discard()
	return delTxn.Commit(nil)
}

func compareAttrAndType(key []byte, attr string, typ byte) bool {
	pk := x.Parse(key)
	if pk == nil {
		return true
	}
	if pk.Attr == attr && pk.IsType(typ) {
		return true
	}
	return false
}

func DeleteReverseEdges(ctx context.Context, attr string, startTs uint64) error {
	err := lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteReverse)
	})
	if err != nil {
		return err
	}
	// Delete index entries from data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.ReversePrefix()
	if err := deleteEntries(prefix, startTs); err != nil {
		return err
	}
	return nil
}

func deleteCountIndex(ctx context.Context, attr string, reverse bool, startTs uint64) error {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.CountPrefix(reverse)
	if err := deleteEntries(prefix, startTs); err != nil {
		return err
	}
	return nil
}

func DeleteCountIndex(ctx context.Context, attr string, startTs uint64) error {
	err := lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteCount)
	})
	if err != nil {
		return err
	}
	err = lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteCountRev)
	})
	if err != nil {
		return err
	}
	// Delete index entries from data store.
	if err := deleteCountIndex(ctx, attr, false, startTs); err != nil {
		return err
	}
	if err := deleteCountIndex(ctx, attr, true, startTs); err != nil { // delete reverse count indexes.
		return err
	}
	return nil
}

func rebuildCountIndex(ctx context.Context, attr string, reverse bool, errCh chan error, txn *Txn) {
	ch := make(chan item, 10000)
	che := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		go func() {
			var err error
			for it := range ch {
				l := it.list
				t := &protos.DirectedEdge{
					ValueId: it.uid,
					Attr:    attr,
					Op:      protos.DirectedEdge_SET,
				}
				if err = addCountMutation(ctx, t, uint32(l.Length(txn.StartTs, 0)), reverse, txn); err != nil {
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

	t := pstore.NewTransactionAt(txn.StartTs, false)
	defer t.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := t.NewIterator(iterOpts)
	defer it.Close()
	var prevKey []byte
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		iterItem := it.Item()
		key := iterItem.Key()
		if bytes.Equal(key, prevKey) {
			continue
		}
		pki := x.Parse(key)
		if pki == nil {
			continue
		}
		nk := make([]byte, len(key))
		copy(nk, key)
		// readPostingList advances the iterator until it finds complete pl
		l, err := readPostingList(nk, it)
		if err != nil {
			errCh <- err
			return
		}

		ch <- item{
			uid:  pki.Uid,
			list: l,
		}
		prevKey = nk
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

func RebuildCountIndex(ctx context.Context, attr string, txn *Txn) error {
	x.AssertTruef(schema.State().HasCount(attr), "Attr %s doesn't have count index", attr)
	errCh := make(chan error, 2)
	// Lets rebuild forward and reverse count indexes concurrently.
	go rebuildCountIndex(ctx, attr, false, errCh, txn)
	go rebuildCountIndex(ctx, attr, true, errCh, txn)

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
	list *List
}

// RebuildReverseEdges rebuilds the reverse edges for a given attribute.
func RebuildReverseEdges(ctx context.Context, attr string, txn *Txn) error {
	x.AssertTruef(schema.State().IsReversed(attr), "Attr %s doesn't have reverse", attr)
	// Add index entries to data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	t := pstore.NewTransactionAt(txn.StartTs, false)
	defer t.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := t.NewIterator(iterOpts)
	defer it.Close()

	// Helper function - Add reverse entries for values in posting list
	addReversePostings := func(uid uint64, pl *List) error {
		edge := protos.DirectedEdge{Attr: attr, Entity: uid}
		var err error
		pl.Iterate(txn.StartTs, 0, func(pp *protos.Posting) bool {
			puid := pp.Uid
			// Add reverse entries based on p.
			edge.ValueId = puid
			edge.Op = protos.DirectedEdge_SET
			edge.Facets = pp.Facets
			edge.Label = pp.Label
			err = addReverseMutation(ctx, &edge, txn)
			// We retry once in case we do GetLru and stop the world happens
			// before we do addmutation
			if err == ErrRetry {
				err = addReverseMutation(ctx, &edge, txn)
			}
			if err != nil {
				return false
			}
			return true
		})
		return err
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

	var prevKey []byte
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		iterItem := it.Item()
		key := iterItem.Key()
		if !bytes.HasPrefix(key, prevKey) {
			continue
		}
		pki := x.Parse(key)
		if pki == nil {
			continue
		}
		nk := make([]byte, len(key))
		copy(nk, key)
		l, err := readPostingList(nk, it)
		if err != nil {
			return err
		}

		ch <- item{
			uid:  pki.Uid,
			list: l,
		}
		prevKey = nk
	}
	close(ch)
	for i := 0; i < 1000; i++ {
		if err := <-che; err != nil {
			return err
		}
	}
	return nil
}

func DeleteIndex(ctx context.Context, attr string, startTs uint64) error {
	err := lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteIndex)
	})
	if err != nil {
		return err
	}
	// Delete index entries from data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	if err := deleteEntries(prefix, startTs); err != nil {
		return err
	}
	return nil
}

// RebuildIndex rebuilds index for a given attribute.
// Ignore for now
// TODO: Commit the mutations immediately
func RebuildIndex(ctx context.Context, attr string, txn *Txn) error {
	x.AssertTruef(schema.State().IsIndexed(attr), "Attr %s not indexed", attr)
	// Add index entries to data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	t := pstore.NewTransactionAt(txn.StartTs, false)
	defer t.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := t.NewIterator(iterOpts)
	defer it.Close()

	// Helper function - Add index entries for values in posting list
	addPostingsToIndex := func(uid uint64, pl *List) error {
		edge := protos.DirectedEdge{Attr: attr, Entity: uid}
		var err error
		pl.Iterate(txn.StartTs, 0, func(p *protos.Posting) bool {
			// Add index entries based on p.
			val := types.Val{
				Value: p.Value,
				Tid:   types.TypeID(p.ValType),
			}
			err = addIndexMutations(ctx, &edge, val, protos.DirectedEdge_SET, txn)
			// We retry once in case we do GetLru and stop the world happens
			// before we do addmutation
			if err == ErrRetry {
				err = addIndexMutations(ctx, &edge, val, protos.DirectedEdge_SET, txn)
			}
			if err != nil {
				return false
			}
			return true
		})
		return err
	}

	type item struct {
		uid  uint64
		list *List
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

	var prevKey []byte
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		iterItem := it.Item()
		key := iterItem.Key()
		if !bytes.Equal(key, prevKey) {
			continue
		}
		pki := x.Parse(key)
		if pki == nil {
			continue
		}
		nk := make([]byte, len(key))
		copy(nk, key)
		l, err := readPostingList(nk, it)
		if err != nil {
			return err
		}

		ch <- item{
			uid:  pki.Uid,
			list: l,
		}
		prevKey = nk
	}
	close(ch)
	for i := 0; i < 1000; i++ {
		if err := <-che; err != nil {
			return err
		}
	}
	return nil
}

func DeleteAll(startTs uint64) error {
	if err := lcache.clear(func([]byte) bool { return true }); err != nil {
		return err
	}
	return deleteEntries(nil, startTs)
}

func DeletePredicate(ctx context.Context, attr string, startTs uint64) error {
	err := lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteData)
	})
	if err != nil {
		return err
	}
	pk := x.ParsedKey{
		Attr: attr,
	}
	prefix := pk.DataPrefix()
	// Delete all data postings for the given predicate.
	if err := deleteEntries(prefix, startTs); err != nil {
		return err
	}

	// TODO - We will still have the predicate present in <uid, _predicate_> posting lists.
	indexed := schema.State().IsIndexed(attr)
	reversed := schema.State().IsReversed(attr)
	if indexed {
		if err := DeleteIndex(ctx, attr, startTs); err != nil {
			return err
		}
	} else if reversed {
		if err := DeleteReverseEdges(ctx, attr, startTs); err != nil {
			return err
		}
	}

	hasCountIndex := schema.State().HasCount(attr)
	if hasCountIndex {
		if err := DeleteCountIndex(ctx, attr, startTs); err != nil {
			return err
		}
	}

	s, ok := schema.State().Get(attr)
	if !ok {
		return nil
	}

	var index uint64
	if rv, ok := ctx.Value("raft").(x.RaftValue); ok {
		index = rv.Index
	}
	if index == 0 {
		// This function is called by cleaning thread(after predicate move)
		return schema.State().Remove(attr)
	}
	if !s.Explicit {
		// Delete predicate from schema.
		se := schema.SyncEntry{
			Attr:  attr,
			Index: index,
			Water: SyncMarks(),
			Schema: protos.SchemaUpdate{
				Predicate: attr,
				Directive: protos.SchemaUpdate_DELETE,
			},
		}
		schema.State().Delete(se)
	}
	return nil
}
