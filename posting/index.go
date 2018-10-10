/*
 * Copyright 2016-2018 Dgraph Labs, Inc.
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/y"
	"github.com/golang/glog"

	"github.com/dgraph-io/dgraph/protos/pb"
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
				return nil, status.Errorf(codes.Internal, "Tokenizer not available for language: %s", lang)
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
func (txn *Txn) addIndexMutations(ctx context.Context, t *pb.DirectedEdge, p types.Val,
	op pb.DirectedEdge_Op) error {
	attr := t.Attr
	uid := t.Entity
	x.AssertTrue(uid != 0)
	tokens, err := indexTokens(attr, t.GetLang(), p)

	if err != nil {
		// This data is not indexable
		return err
	}

	// Create a value token -> uid edge.
	edge := &pb.DirectedEdge{
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

func (txn *Txn) addIndexMutation(ctx context.Context, edge *pb.DirectedEdge,
	token string) error {
	key := x.IndexKey(edge.Attr, token)

	plist, err := txn.Get(key)
	if err != nil {
		return err
	}

	x.AssertTrue(plist != nil)
	if err = plist.AddMutation(ctx, txn, edge); err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("Error adding/deleting %s for attr %s entity %d: %v",
				token, edge.Attr, edge.Entity, err)
		}
		return err
	}
	x.PredicateStats.Add("i."+edge.Attr, 1)
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
	hasCountIndex bool, edge *pb.DirectedEdge) (countParams, error) {
	countBefore, countAfter := 0, 0
	plist.Lock()
	defer plist.Unlock()
	if hasCountIndex {
		countBefore = plist.length(txn.StartTs, 0)
		if countBefore == -1 {
			return emptyCountParams, ErrTsTooOld
		}
	}
	if err := plist.addMutation(ctx, txn, edge); err != nil {
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

func (txn *Txn) addReverseMutation(ctx context.Context, t *pb.DirectedEdge) error {
	key := x.ReverseKey(t.Attr, t.ValueId)
	plist, err := txn.Get(key)
	if err != nil {
		return err
	}

	x.AssertTrue(plist != nil)
	// We must create a copy here.
	edge := &pb.DirectedEdge{
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

func (l *List) handleDeleteAll(ctx context.Context, t *pb.DirectedEdge,
	txn *Txn) error {
	isReversed := schema.State().IsReversed(t.Attr)
	isIndexed := schema.State().IsIndexed(t.Attr)
	hasCount := schema.State().HasCount(t.Attr)
	delEdge := &pb.DirectedEdge{
		Attr:   t.Attr,
		Op:     t.Op,
		Entity: t.Entity,
	}
	// To calculate length of posting list. Used for deletion of count index.
	var plen int
	if err := l.Iterate(txn.StartTs, 0, func(p *pb.Posting) error {
		plen++
		switch {
		case isReversed:
			// Delete reverse edge for each posting.
			delEdge.ValueId = p.Uid
			return txn.addReverseMutation(ctx, delEdge)
		case isIndexed:
			// Delete index edge of each posting.
			p := types.Val{
				Tid:   types.TypeID(p.ValType),
				Value: p.Value,
			}
			return txn.addIndexMutations(ctx, t, p, pb.DirectedEdge_DEL)
		default:
			return nil
		}
	}); err != nil {
		return err
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
	return l.addMutation(ctx, txn, t)
}

func (txn *Txn) addCountMutation(ctx context.Context, t *pb.DirectedEdge, count uint32,
	reverse bool) error {
	key := x.CountKey(t.Attr, count, reverse)
	plist, err := txn.Get(key)
	if err != nil {
		return err
	}

	x.AssertTruef(plist != nil, "plist is nil [%s] %d",
		t.Attr, t.ValueId)
	if err = plist.AddMutation(ctx, txn, t); err != nil {
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
	edge := pb.DirectedEdge{
		ValueId: params.entity,
		Attr:    params.attr,
		Op:      pb.DirectedEdge_DEL,
	}
	if err := txn.addCountMutation(ctx, &edge, uint32(params.countBefore),
		params.reverse); err != nil {
		return err
	}

	if params.countAfter > 0 {
		edge.Op = pb.DirectedEdge_SET
		if err := txn.addCountMutation(ctx, &edge, uint32(params.countAfter),
			params.reverse); err != nil {
			return err
		}
	}
	return nil
}

func (txn *Txn) addMutationHelper(ctx context.Context, l *List, doUpdateIndex bool,
	hasCountIndex bool, t *pb.DirectedEdge) (types.Val, bool, countParams, error) {
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
	if err = l.addMutation(ctx, txn, t); err != nil {
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
func (l *List) AddMutationWithIndex(ctx context.Context, t *pb.DirectedEdge,
	txn *Txn) error {
	if len(t.Attr) == 0 {
		return x.Errorf("Predicate cannot be empty for edge with subject: [%v], object: [%v]"+
			" and value: [%v]", t.Entity, t.ValueId, t.Value)
	}

	if t.Op == pb.DirectedEdge_DEL && string(t.Value) == x.Star {
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
			if err := txn.addIndexMutations(ctx, t, val, pb.DirectedEdge_DEL); err != nil {
				return err
			}
		}
		if t.Op == pb.DirectedEdge_SET {
			p := types.Val{
				Tid:   types.TypeID(t.ValueType),
				Value: t.Value,
			}
			if err := txn.addIndexMutations(ctx, t, p, pb.DirectedEdge_SET); err != nil {
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
		nkey := item.KeyCopy(nil)
		version := item.Version()

		txn := pstore.NewTransactionAt(version, true)
		txn.Delete(nkey)
		wg.Add(1)
		err := txn.CommitAt(version, func(e error) {
			defer wg.Done()
			if e != nil {
				setError(e)
				return
			}
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

func DeleteReverseEdges(attr string) error {
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

func deleteCountIndex(attr string, reverse bool) error {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.CountPrefix(reverse)
	return deleteEntries(prefix, func(key []byte) bool {
		return true
	})
}

func DeleteCountIndex(attr string) error {
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteCount)
	})
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteCountRev)
	})
	// Delete index entries from data store.
	if err := deleteCountIndex(attr, false); err != nil {
		return err
	}
	if err := deleteCountIndex(attr, true); err != nil { // delete reverse count indexes.
		return err
	}
	return nil
}

// Index rebuilding logic here.
type rebuild struct {
	prefix  []byte
	startTs uint64
	fn      func(uid uint64, pl *List, txn *Txn) error
}

func (r *rebuild) Run(ctx context.Context) error {
	t := pstore.NewTransactionAt(r.startTs, false)
	defer t.Discard()

	opts := badger.DefaultIteratorOptions
	opts.AllVersions = true
	it := t.NewIterator(opts)
	defer it.Close()

	// We create one txn for all the mutations to be housed in. We also create a
	// localized posting list cache, to avoid stressing or mixing up with the
	// global lcache (the LRU cache).
	cache := make(map[string]*List)
	txn := &Txn{StartTs: r.startTs}
	var numGets uint64
	txn.getList = func(key []byte) (*List, error) {
		numGets++
		if glog.V(2) && numGets%1000 == 0 {
			glog.Infof("During rebuild, getList hit %d times\n", numGets)
		}
		if pl, ok := cache[string(key)]; ok {
			return pl, nil
		}
		pl, err := getNew(key, pstore)
		if err != nil {
			return nil, err
		}
		cache[string(key)] = pl
		return pl, nil
	}

	var prevKey []byte
	for it.Seek(r.prefix); it.ValidForPrefix(r.prefix); {
		item := it.Item()
		if bytes.Equal(item.Key(), prevKey) {
			it.Next()
			continue
		}
		key := item.KeyCopy(nil)
		prevKey = key

		pk := x.Parse(key)
		if pk == nil {
			it.Next()
			continue
		}

		// We should return quickly if the context is no longer valid.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		l, err := ReadPostingList(key, it)
		if err != nil {
			return err
		}
		if err := r.fn(pk.Uid, l, txn); err != nil {
			return err
		}
	}

	// We must commit all the posting lists to memory, so they'd be picked up
	// during posting list rollup below.
	if err := txn.CommitToMemory(r.startTs); err != nil {
		return err
	}

	// Now we write all the created posting lists to disk.
	writer := x.TxnWriter{DB: pstore}
	for key := range txn.deltas {
		pl, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		pl.Lock() // We shouldn't need to release this lock.
		le := pl.length(r.startTs, 0)
		y.AssertTruef(le > 0, "Unexpected list of size zero: %q", key)
		if err := pl.rollup(); err != nil {
			return err
		}
		data, meta := marshalPostingList(pl.plist)
		if err = writer.SetAt([]byte(key), data, meta, r.startTs); err != nil {
			return err
		}
	}
	return writer.Flush()
}

// RebuildIndex rebuilds index for a given attribute.
// We commit mutations with startTs and ignore the errors.
func RebuildIndex(ctx context.Context, attr string, startTs uint64) error {
	x.AssertTruef(schema.State().IsIndexed(attr), "Attr %s not indexed", attr)

	pk := x.ParsedKey{Attr: attr}
	builder := rebuild{prefix: pk.DataPrefix(), startTs: startTs}
	builder.fn = func(uid uint64, pl *List, txn *Txn) error {
		edge := pb.DirectedEdge{Attr: attr, Entity: uid}
		return pl.Iterate(txn.StartTs, 0, func(p *pb.Posting) error {
			// Add index entries based on p.
			val := types.Val{
				Value: p.Value,
				Tid:   types.TypeID(p.ValType),
			}

			for {
				err := txn.addIndexMutations(ctx, &edge, val, pb.DirectedEdge_SET)
				switch err {
				case ErrRetry:
					time.Sleep(10 * time.Millisecond)
				default:
					return err
				}
			}
		})
	}
	return builder.Run(ctx)
}

func RebuildCountIndex(ctx context.Context, attr string, startTs uint64) error {
	x.AssertTruef(schema.State().HasCount(attr), "Attr %s doesn't have count index", attr)

	var reverse bool
	fn := func(uid uint64, pl *List, txn *Txn) error {
		t := &pb.DirectedEdge{
			ValueId: uid,
			Attr:    attr,
			Op:      pb.DirectedEdge_SET,
		}
		sz := pl.Length(startTs, 0)
		if sz == -1 {
			return nil
		}
		for {
			err := txn.addCountMutation(ctx, t, uint32(sz), reverse)
			switch err {
			case ErrRetry:
				time.Sleep(10 * time.Millisecond)
			default:
				return err
			}
		}
	}

	// Create the forward index.
	pk := x.ParsedKey{Attr: attr}
	builder := rebuild{prefix: pk.DataPrefix(), startTs: startTs}
	builder.fn = fn
	if err := builder.Run(ctx); err != nil {
		return err
	}

	// Create the reverse index.
	reverse = true
	builder = rebuild{prefix: pk.ReversePrefix(), startTs: startTs}
	builder.fn = fn
	return builder.Run(ctx)
}

type item struct {
	uid  uint64
	list *List
}

// RebuildReverseEdges rebuilds the reverse edges for a given attribute.
func RebuildReverseEdges(ctx context.Context, attr string, startTs uint64) error {
	x.AssertTruef(schema.State().IsReversed(attr), "Attr %s doesn't have reverse", attr)

	pk := x.ParsedKey{Attr: attr}
	builder := rebuild{prefix: pk.DataPrefix(), startTs: startTs}
	builder.fn = func(uid uint64, pl *List, txn *Txn) error {
		edge := pb.DirectedEdge{Attr: attr, Entity: uid}
		return pl.Iterate(txn.StartTs, 0, func(pp *pb.Posting) error {
			puid := pp.Uid
			// Add reverse entries based on p.
			edge.ValueId = puid
			edge.Op = pb.DirectedEdge_SET
			edge.Facets = pp.Facets
			edge.Label = pp.Label

			for {
				err := txn.addReverseMutation(ctx, &edge)
				switch err {
				case ErrRetry:
					time.Sleep(10 * time.Millisecond)
				default:
					return err
				}
			}
		})
	}
	return builder.Run(ctx)
}

func DeleteIndex(attr string) error {
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

	// Let's clear out the cache for anything which belongs to this attribute,
	// so once we're done, any reads would see the new list type. Note that we
	// don't use lcache during the rebuild process.
	lcache.clear(func(key []byte) bool {
		return compareAttrAndType(key, attr, x.ByteData)
	})

	pk := x.ParsedKey{Attr: attr}
	builder := rebuild{prefix: pk.DataPrefix(), startTs: startTs}
	builder.fn = func(uid uint64, pl *List, txn *Txn) error {
		var mpost *pb.Posting
		if err := pl.Iterate(txn.StartTs, 0, func(p *pb.Posting) error {
			// We only want to modify the untagged value. There could be other values with a
			// lang tag.
			if p.Uid == math.MaxUint64 {
				mpost = p
			}
			return nil
		}); err != nil {
			return err
		}
		if mpost == nil {
			return nil
		}
		// Delete the old edge corresponding to ValueId math.MaxUint64
		t := &pb.DirectedEdge{
			ValueId: mpost.Uid,
			Attr:    attr,
			Op:      pb.DirectedEdge_DEL,
		}
		if err := pl.AddMutation(ctx, txn, t); err != nil {
			return err
		}
		// Add the new edge with the fingerprinted value id.
		newEdge := &pb.DirectedEdge{
			Attr:      attr,
			Value:     mpost.Value,
			ValueType: mpost.ValType,
			Op:        pb.DirectedEdge_SET,
			Label:     mpost.Label,
			Facets:    mpost.Facets,
		}
		return pl.AddMutation(ctx, txn, newEdge)
	}
	return builder.Run(ctx)
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
		if err := DeleteIndex(attr); err != nil {
			return err
		}
	} else if reversed {
		if err := DeleteReverseEdges(attr); err != nil {
			return err
		}
	}

	hasCountIndex := schema.State().HasCount(attr)
	if hasCountIndex {
		if err := DeleteCountIndex(attr); err != nil {
			return err
		}
	}

	return schema.State().Delete(attr)
}
