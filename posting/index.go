/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * This file is available under the Apache License, Version 2.0,
 * with the Commons Clause restriction.
 */

package posting

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"

	"github.com/dgraph-io/dgraph/protos/intern"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const maxBatchSize = 32 * (1 << 20)

var emptyCountParams countParams

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
		if schemaType == types.StringID {
			exactTok, ok := tok.GetTokenizer("exact")
			x.AssertTruef(ok, "Couldn't find exact tokenizer.")
			// Exact index can only be applied for strings so we can safely try to convert Value to
			// string.
			if (it.Identifier() == exactTok.Identifier()) && len(sv.Value.(string)) > 100 {
				x.Printf("Long term for exact index on predicate: [%s]. "+
					"Consider switching to hash for better performance.\n", attr)
			}
		}
		toks, err := tok.BuildTokens(sv.Value, it)
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
func (txn *Txn) addIndexMutations(ctx context.Context, t *intern.DirectedEdge, p types.Val,
	op intern.DirectedEdge_Op) error {
	attr := t.Attr
	uid := t.Entity
	x.AssertTrue(uid != 0)
	tokens, err := indexTokens(attr, t.GetLang(), p)

	if err != nil {
		// This data is not indexable
		return err
	}

	// Create a value token -> uid edge.
	edge := &intern.DirectedEdge{
		ValueId: uid,
		Attr:    attr,
		Op:      op,
	}

	for _, token := range tokens {
		if err := txn.addIndexMutation(ctx, edge, token); err != nil {
			return err
		}
	}
	return nil
}

func (txn *Txn) addIndexMutation(ctx context.Context, edge *intern.DirectedEdge,
	token string) error {
	key := x.IndexKey(edge.Attr, token)

	t := time.Now()
	plist, err := Get(key)
	if dur := time.Since(t); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("getOrMutate took %v", dur)
		}
	}
	if err != nil {
		return err
	}

	x.AssertTrue(plist != nil)
	_, err = plist.AddMutation(ctx, txn, edge)
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

func (txn *Txn) addReverseMutationHelper(ctx context.Context, plist *List,
	hasCountIndex bool, edge *intern.DirectedEdge) (countParams, error) {
	countBefore, countAfter := 0, 0
	plist.Lock()
	defer plist.Unlock()
	if hasCountIndex {
		countBefore = plist.length(txn.StartTs, 0)
		if countBefore == -1 {
			return emptyCountParams, ErrTsTooOld
		}
	}
	_, err := plist.addMutation(ctx, txn, edge)
	if err != nil {
		return emptyCountParams, err
	}
	if hasCountIndex {
		countAfter = plist.length(txn.StartTs, 0)
		if countAfter == -1 {
			return emptyCountParams, ErrTsTooOld
		}
		return countParams{
			attr:        edge.Attr,
			countBefore: countBefore,
			countAfter:  countAfter,
			entity:      edge.Entity,
			reverse:     true,
		}, nil
	}
	return emptyCountParams, nil
}

func (txn *Txn) addReverseMutation(ctx context.Context, t *intern.DirectedEdge) error {
	key := x.ReverseKey(t.Attr, t.ValueId)
	plist, err := Get(key)
	if err != nil {
		return err
	}

	x.AssertTrue(plist != nil)
	edge := &intern.DirectedEdge{
		Entity:  t.ValueId,
		ValueId: t.Entity,
		Attr:    t.Attr,
		Op:      t.Op,
		Facets:  t.Facets,
	}

	hasCountIndex := schema.State().HasCount(t.Attr)
	cp, err := txn.addReverseMutationHelper(ctx, plist, hasCountIndex, edge)
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error adding/deleting reverse edge for attr %s entity %d: %v",
				t.Attr, t.Entity, err)
		}
		return err
	}
	x.PredicateStats.Add(fmt.Sprintf("r.%s", edge.Attr), 1)

	if hasCountIndex && cp.countAfter != cp.countBefore {
		if err := txn.updateCount(ctx, cp); err != nil {
			return err
		}
	}
	return nil
}

func (l *List) handleDeleteAll(ctx context.Context, t *intern.DirectedEdge,
	txn *Txn) error {
	isReversed := schema.State().IsReversed(t.Attr)
	isIndexed := schema.State().IsIndexed(t.Attr)
	hasCount := schema.State().HasCount(t.Attr)
	delEdge := &intern.DirectedEdge{
		Attr:   t.Attr,
		Op:     t.Op,
		Entity: t.Entity,
	}
	// To calculate length of posting list. Used for deletion of count index.
	var plen int
	var iterErr error
	l.Iterate(txn.StartTs, 0, func(p *intern.Posting) bool {
		plen++
		if isReversed {
			// Delete reverse edge for each posting.
			delEdge.ValueId = p.Uid
			if err := txn.addReverseMutation(ctx, delEdge); err != nil {
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
			if err := txn.addIndexMutations(ctx, t, p, intern.DirectedEdge_DEL); err != nil {
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
		if err := txn.updateCount(ctx, countParams{
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
	_, err := l.addMutation(ctx, txn, t)
	return err
}

func (txn *Txn) addCountMutation(ctx context.Context, t *intern.DirectedEdge, count uint32,
	reverse bool) error {
	key := x.CountKey(t.Attr, count, reverse)
	plist, err := Get(key)
	if err != nil {
		return err
	}

	x.AssertTruef(plist != nil, "plist is nil [%s] %d",
		t.Attr, t.ValueId)
	_, err = plist.AddMutation(ctx, txn, t)
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

func (txn *Txn) updateCount(ctx context.Context, params countParams) error {
	edge := intern.DirectedEdge{
		ValueId: params.entity,
		Attr:    params.attr,
		Op:      intern.DirectedEdge_DEL,
	}
	if err := txn.addCountMutation(ctx, &edge, uint32(params.countBefore),
		params.reverse); err != nil {
		return err
	}

	if params.countAfter > 0 {
		edge.Op = intern.DirectedEdge_SET
		if err := txn.addCountMutation(ctx, &edge, uint32(params.countAfter),
			params.reverse); err != nil {
			return err
		}
	}
	return nil
}

func (txn *Txn) addMutationHelper(ctx context.Context, l *List, doUpdateIndex bool,
	hasCountIndex bool, t *intern.DirectedEdge) (types.Val, bool, countParams, error) {
	var val types.Val
	var found bool
	var err error

	t1 := time.Now()
	l.Lock()
	defer l.Unlock()
	if dur := time.Since(t1); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("acquired lock %v %v %v", dur, t.Attr, t.Entity)
		}
	}

	if doUpdateIndex {
		// Check original value BEFORE any mutation actually happens.
		val, found, err = l.findValue(txn.StartTs, fingerprintEdge(t))
		if err != nil {
			return val, found, emptyCountParams, err
		}
	}
	countBefore, countAfter := 0, 0
	if hasCountIndex {
		countBefore = l.length(txn.StartTs, 0)
		if countBefore == -1 {
			return val, found, emptyCountParams, ErrTsTooOld
		}
	}
	_, err = l.addMutation(ctx, txn, t)
	if err != nil {
		return val, found, emptyCountParams, err
	}
	if hasCountIndex {
		countAfter = l.length(txn.StartTs, 0)
		if countAfter == -1 {
			return val, found, emptyCountParams, ErrTsTooOld
		}
		return val, found, countParams{
			attr:        t.Attr,
			countBefore: countBefore,
			countAfter:  countAfter,
			entity:      t.Entity,
		}, nil
	}
	return val, found, emptyCountParams, nil
}

// AddMutationWithIndex is AddMutation with support for indexing. It also
// supports reverse edges.
func (l *List) AddMutationWithIndex(ctx context.Context, t *intern.DirectedEdge,
	txn *Txn) error {
	if len(t.Attr) == 0 {
		return x.Errorf("Predicate cannot be empty for edge with subject: [%v], object: [%v]"+
			" and value: [%v]", t.Entity, t.ValueId, t.Value)
	}

	if t.Op == intern.DirectedEdge_DEL && string(t.Value) == x.Star {
		return l.handleDeleteAll(ctx, t, txn)
	}

	doUpdateIndex := pstore != nil && schema.State().IsIndexed(t.Attr)
	hasCountIndex := schema.State().HasCount(t.Attr)
	val, found, cp, err := txn.addMutationHelper(ctx, l, doUpdateIndex, hasCountIndex, t)
	if err != nil {
		return err
	}
	x.PredicateStats.Add(t.Attr, 1)
	if hasCountIndex && cp.countAfter != cp.countBefore {
		if err := txn.updateCount(ctx, cp); err != nil {
			return err
		}
	}
	if doUpdateIndex {
		// Exact matches.
		if found && val.Value != nil {
			if err := txn.addIndexMutations(ctx, t, val, intern.DirectedEdge_DEL); err != nil {
				return err
			}
		}
		if t.Op == intern.DirectedEdge_SET {
			p := types.Val{
				Tid:   types.TypeID(t.ValueType),
				Value: t.Value,
			}
			if err := txn.addIndexMutations(ctx, t, p, intern.DirectedEdge_SET); err != nil {
				return err
			}
		}
	}
	// Add reverse mutation irrespective of hasMutated, server crash can happen after
	// mutation is synced and before reverse edge is synced
	if (pstore != nil) && (t.ValueId != 0) && schema.State().IsReversed(t.Attr) {
		if err := txn.addReverseMutation(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

func deleteEntries(prefix []byte, remove func(key []byte) bool) error {
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.PrefetchValues = false
	txn := pstore.NewTransactionAt(math.MaxUint64, false)
	defer txn.Discard()
	idxIt := txn.NewIterator(iterOpt)
	defer idxIt.Close()

	var m sync.Mutex
	var err error
	setError := func(e error) {
		m.Lock()
		err = e
		m.Unlock()
	}
	var wg sync.WaitGroup
	for idxIt.Seek(prefix); idxIt.ValidForPrefix(prefix); idxIt.Next() {
		item := idxIt.Item()
		if !remove(item.Key()) {
			continue
		}
		nkey := make([]byte, len(item.Key()))
		copy(nkey, item.Key())
		version := item.Version()

		txn := pstore.NewTransactionAt(version, true)
		// Purge doesn't delete anything, so write an empty pl
		txn.SetWithMeta(nkey, nil, BitEmptyPosting)
		wg.Add(1)
		err := txn.CommitAt(version, func(e error) {
			defer wg.Done()
			if e != nil {
				setError(e)
				return
			}
			pstore.PurgeVersionsBelow(nkey, version)
		})
		txn.Discard()
		if err != nil {
			break
		}
	}
	wg.Wait()
	return err
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

func DeleteReverseEdges(ctx context.Context, attr string) error {
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteReverse)
	})
	// Delete index entries from data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.ReversePrefix()
	return deleteEntries(prefix, func(key []byte) bool {
		return true
	})
}

func deleteCountIndex(ctx context.Context, attr string, reverse bool) error {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.CountPrefix(reverse)
	return deleteEntries(prefix, func(key []byte) bool {
		return true
	})
}

func DeleteCountIndex(ctx context.Context, attr string) error {
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteCount)
	})
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteCountRev)
	})
	// Delete index entries from data store.
	if err := deleteCountIndex(ctx, attr, false); err != nil {
		return err
	}
	if err := deleteCountIndex(ctx, attr, true); err != nil { // delete reverse count indexes.
		return err
	}
	return nil
}

func rebuildCountIndex(ctx context.Context, attr string, reverse bool, errCh chan error,
	startTs uint64) {
	ch := make(chan item, 10000)
	che := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		go func() {
			var err error
			txn := &Txn{StartTs: startTs}
			for it := range ch {
				l := it.list
				t := &intern.DirectedEdge{
					ValueId: it.uid,
					Attr:    attr,
					Op:      intern.DirectedEdge_SET,
				}
				len := l.Length(txn.StartTs, 0)
				if len == -1 {
					continue
				}
				err = txn.addCountMutation(ctx, t, uint32(len), reverse)
				for err == ErrRetry {
					time.Sleep(10 * time.Millisecond)
					err = txn.addCountMutation(ctx, t, uint32(len), reverse)
				}
				if err == nil {
					err = txn.CommitMutationsMemory(ctx, txn.StartTs)
				}
				if err != nil {
					txn.AbortMutations(ctx)
				}
				txn.deltas = nil
			}
			che <- err
		}()
	}

	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	if reverse {
		prefix = pk.ReversePrefix()
	}

	t := pstore.NewTransactionAt(startTs, false)
	defer t.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := t.NewIterator(iterOpts)
	defer it.Close()
	var prevKey []byte
	it.Seek(prefix)
	for it.ValidForPrefix(prefix) {
		iterItem := it.Item()
		key := iterItem.Key()
		if bytes.Equal(key, prevKey) {
			it.Next()
			continue
		}
		nk := make([]byte, len(key))
		copy(nk, key)
		prevKey = nk
		pki := x.Parse(key)
		if pki == nil {
			it.Next()
			continue
		}
		// readPostingList advances the iterator until it finds complete pl
		l, err := ReadPostingList(nk, it)
		if err != nil {
			continue
		}

		ch <- item{
			uid:  pki.Uid,
			list: l,
		}
	}
	close(ch)

	for i := 0; i < 1000; i++ {
		if err := <-che; err != nil {
			errCh <- x.Errorf("While rebuilding count index for attr: [%v], error: [%v]", attr, err)
			return
		}
	}

	errCh <- nil
}

func RebuildCountIndex(ctx context.Context, attr string, startTs uint64) error {
	x.AssertTruef(schema.State().HasCount(attr), "Attr %s doesn't have count index", attr)
	che := make(chan error, 2)
	// Lets rebuild forward and reverse count indexes concurrently.
	go rebuildCountIndex(ctx, attr, false, che, startTs)
	go rebuildCountIndex(ctx, attr, true, che, startTs)

	var err error
	for i := 0; i < 2; i++ {
		if e := <-che; e != nil {
			err = e
		}
	}

	return err
}

type item struct {
	uid  uint64
	list *List
}

// RebuildReverseEdges rebuilds the reverse edges for a given attribute.
func RebuildReverseEdges(ctx context.Context, attr string, startTs uint64) error {
	x.AssertTruef(schema.State().IsReversed(attr), "Attr %s doesn't have reverse", attr)
	// Add index entries to data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	t := pstore.NewTransactionAt(startTs, false)
	defer t.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := t.NewIterator(iterOpts)
	defer it.Close()

	// Helper function - Add reverse entries for values in posting list
	addReversePostings := func(uid uint64, pl *List, txn *Txn) {
		edge := intern.DirectedEdge{Attr: attr, Entity: uid}
		var err error
		pl.Iterate(txn.StartTs, 0, func(pp *intern.Posting) bool {
			puid := pp.Uid
			// Add reverse entries based on p.
			edge.ValueId = puid
			edge.Op = intern.DirectedEdge_SET
			edge.Facets = pp.Facets
			edge.Label = pp.Label
			err = txn.addReverseMutation(ctx, &edge)
			for err == ErrRetry {
				time.Sleep(10 * time.Millisecond)
				err = txn.addReverseMutation(ctx, &edge)
			}
			if err != nil {
				x.Printf("Error while adding reverse mutation: %v\n", err)
			}
			return true
		})
	}

	ch := make(chan item, 10000)
	che := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		go func() {
			var err error
			txn := &Txn{StartTs: startTs}
			for it := range ch {
				addReversePostings(it.uid, it.list, txn)
				err = txn.CommitMutationsMemory(ctx, txn.StartTs)
				if err != nil {
					txn.AbortMutations(ctx)
				}
				txn.deltas = nil
			}
			che <- err
		}()
	}

	var prevKey []byte
	it.Seek(prefix)
	for it.ValidForPrefix(prefix) {
		iterItem := it.Item()
		key := iterItem.Key()
		if bytes.Equal(key, prevKey) {
			it.Next()
			continue
		}
		nk := make([]byte, len(key))
		copy(nk, key)
		prevKey = nk
		pki := x.Parse(key)
		if pki == nil {
			it.Next()
			continue
		}
		l, err := ReadPostingList(nk, it)
		if err != nil {
			continue
		}

		ch <- item{
			uid:  pki.Uid,
			list: l,
		}
	}
	close(ch)

	for i := 0; i < 1000; i++ {
		if err := <-che; err != nil {
			return x.Errorf("While rebuilding reverse edges for attr: [%v], error: [%v]", attr, err)
		}
	}
	return nil
}

func DeleteIndex(ctx context.Context, attr string) error {
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteIndex)
	})
	// Delete index entries from data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	return deleteEntries(prefix, func(key []byte) bool {
		return true
	})
}

// This function is called when the schema is changed from scalar to list type.
// We need to fingerprint the values to get the new ValueId.
func RebuildListType(ctx context.Context, attr string, startTs uint64) error {
	x.AssertTruef(schema.State().IsList(attr), "Attr %s is not of list type", attr)
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteData)
	})

	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	t := pstore.NewTransactionAt(startTs, false)
	defer t.Discard()
	iterOpts := badger.DefaultIteratorOptions
	it := t.NewIterator(iterOpts)
	defer it.Close()

	rewriteValuePostings := func(pl *List, txn *Txn) error {
		var mpost *intern.Posting
		pl.Iterate(txn.StartTs, 0, func(p *intern.Posting) bool {
			// We only want to modify the untagged value. There could be other values with a
			// lang tag.
			if p.Uid == math.MaxUint64 {
				mpost = p
				return false
			}
			return true
		})
		if mpost != nil {
			// Delete the old edge corresponding to ValueId math.MaxUint64
			t := &intern.DirectedEdge{
				ValueId: mpost.Uid,
				Attr:    attr,
				Op:      intern.DirectedEdge_DEL,
			}

			_, err := pl.AddMutation(ctx, txn, t)
			if err != nil {
				return err
			}

			// Add the new edge with the fingerprinted value id.
			newEdge := &intern.DirectedEdge{
				Attr:      attr,
				Value:     mpost.Value,
				ValueType: mpost.ValType,
				Op:        intern.DirectedEdge_SET,
				Label:     mpost.Label,
				Facets:    mpost.Facets,
			}
			_, err = pl.AddMutation(ctx, txn, newEdge)
			if err != nil {
				return err
			}
		}
		return nil
	}

	ch := make(chan *List, 10000)
	che := make(chan error, 1000)
	for i := 0; i < 1000; i++ {
		go func() {
			var err error
			txn := &Txn{StartTs: startTs}
			for list := range ch {
				if err := rewriteValuePostings(list, txn); err != nil {
					che <- err
					return
				}

				err = txn.CommitMutationsMemory(ctx, txn.StartTs)
				if err != nil {
					txn.AbortMutations(ctx)
				}
				txn.deltas = nil
			}
			che <- err
		}()
	}

	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		iterItem := it.Item()
		key := iterItem.Key()
		nk := make([]byte, len(key))
		copy(nk, key)

		// Get is important because we are modifying the mutation layer of the posting lists and
		// hence want to put the PL in LRU cache.
		pl, err := Get(nk)
		if err != nil {
			return err
		}
		ch <- pl
	}
	close(ch)

	for i := 0; i < 1000; i++ {
		if err := <-che; err != nil {
			return x.Errorf("While rebuilding list type for attr: [%v], error: [%v]", attr, err)
		}
	}
	return nil
}

// RebuildIndex rebuilds index for a given attribute.
// We commit mutations with startTs and ignore the errors.
func RebuildIndex(ctx context.Context, attr string, startTs uint64) error {
	x.AssertTruef(schema.State().IsIndexed(attr), "Attr %s not indexed", attr)
	// Add index entries to data store.
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.DataPrefix()
	t := pstore.NewTransactionAt(startTs, false)
	defer t.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := t.NewIterator(iterOpts)
	defer it.Close()

	// Helper function - Add index entries for values in posting list
	addPostingsToIndex := func(uid uint64, pl *List, txn *Txn) {
		edge := intern.DirectedEdge{Attr: attr, Entity: uid}
		var err error
		pl.Iterate(txn.StartTs, 0, func(p *intern.Posting) bool {
			// Add index entries based on p.
			val := types.Val{
				Value: p.Value,
				Tid:   types.TypeID(p.ValType),
			}
			err = txn.addIndexMutations(ctx, &edge, val, intern.DirectedEdge_SET)
			for err == ErrRetry {
				time.Sleep(10 * time.Millisecond)
				err = txn.addIndexMutations(ctx, &edge, val, intern.DirectedEdge_SET)
			}
			if err != nil {
				x.Printf("Error while adding index mutation: %v\n", err)
			}
			return true
		})
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
			txn := &Txn{StartTs: startTs}
			for it := range ch {
				addPostingsToIndex(it.uid, it.list, txn)
				err = txn.CommitMutationsMemory(ctx, txn.StartTs)
				if err != nil {
					txn.AbortMutations(ctx)
				}
				txn.deltas = nil
			}
			che <- err
		}()
	}

	var prevKey []byte
	it.Seek(prefix)
	for it.ValidForPrefix(prefix) {
		iterItem := it.Item()
		key := iterItem.Key()
		if bytes.Equal(key, prevKey) {
			it.Next()
			continue
		}
		nk := make([]byte, len(key))
		copy(nk, key)
		prevKey = nk
		pki := x.Parse(key)
		if pki == nil {
			it.Next()
			continue
		}
		l, err := ReadPostingList(nk, it)
		if err != nil {
			continue
		}

		ch <- item{
			uid:  pki.Uid,
			list: l,
		}
	}
	close(ch)
	for i := 0; i < 1000; i++ {
		if err := <-che; err != nil {
			return x.Errorf("While rebuilding index for attr: [%v], error: [%v]", attr, err)
		}
	}
	return nil
}

func DeleteAll() error {
	btree.DeleteAll()
	lcache.clear(func([]byte) bool { return true })
	return deleteEntries(nil, func(key []byte) bool {
		pk := x.Parse(key)
		if pk == nil {
			return true
		} else if pk.IsSchema() && pk.Attr == x.PredicateListAttr {
			// Don't delete schema for _predicate_
			return false
		}
		return true
	})
}

func DeletePredicate(ctx context.Context, attr string) error {
	x.Printf("Dropping predicate: [%s]", attr)
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteData)
	})
	pk := x.ParsedKey{
		Attr: attr,
	}
	prefix := pk.DataPrefix()
	// Delete all data postings for the given predicate.
	err := deleteEntries(prefix, func(key []byte) bool {
		return true
	})
	if err != nil {
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

	hasCountIndex := schema.State().HasCount(attr)
	if hasCountIndex {
		if err := DeleteCountIndex(ctx, attr); err != nil {
			return err
		}
	}

	return schema.State().Delete(attr)
}
