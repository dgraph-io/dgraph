/*
 * Copyright 2016-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package posting

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/golang/glog"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

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
	sv, err := types.Convert(src, schemaType)
	if err != nil {
		return nil, err
	}
	// Schema will know the mapping from attr to tokenizer.
	var tokens []string
	for _, it := range schema.State().Tokenizer(attr) {
		if it.Name() == "exact" && schemaType == types.StringID && len(sv.Value.(string)) > 100 {
			// Exact index can only be applied for strings so we can safely try to convert Value to
			// string.
			glog.Infof("Long term for exact index on predicate: [%s]. "+
				"Consider switching to hash for better performance.\n", attr)
		}
		toks, err := tok.BuildTokens(sv.Value, tok.GetLangTokenizer(it, lang))
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
	err := l.Iterate(txn.StartTs, 0, func(p *pb.Posting) error {
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
	})
	if err != nil {
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
		span := otrace.FromContext(ctx)
		span.Annotatef([]otrace.Attribute{otrace.BoolAttribute("slow-lock", true)},
			"Acquired lock %v %v %v", dur, t.Attr, t.Entity)
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
	return pstore.View(func(txn *badger.Txn) error {
		opt := badger.DefaultIteratorOptions
		opt.Prefix = prefix
		opt.PrefetchValues = false

		itr := txn.NewIterator(opt)
		defer itr.Close()

		writer := x.NewTxnWriter(pstore)
		for itr.Rewind(); itr.Valid(); itr.Next() {
			item := itr.Item()
			if !remove(item.Key()) {
				continue
			}
			if err := writer.Delete(item.KeyCopy(nil), item.Version()); err != nil {
				return err
			}
		}
		return writer.Flush()
	})
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
	cache   map[string]*List

	// The posting list passed here is the on disk version. It is not coming
	// from the LRU cache.
	fn func(uid uint64, pl *List, txn *Txn) error
}

// storeList would store the list in the cache.
func (r *rebuild) storeList(list *List) bool {
	key := string(list.key)
	if _, ok := r.cache[key]; ok {
		return false
	}
	r.cache[key] = list
	return true
}

func (r *rebuild) Run(ctx context.Context) error {
	t := pstore.NewTransactionAt(r.startTs, false)
	defer t.Discard()

	glog.V(1).Infof("Rebuild: Starting process. StartTs=%d. Prefix=\n%s\n",
		r.startTs, hex.Dump(r.prefix))
	opts := badger.DefaultIteratorOptions
	opts.AllVersions = true
	opts.Prefix = r.prefix
	it := t.NewIterator(opts)
	defer it.Close()

	// We create one txn for all the mutations to be housed in. We also create a
	// localized posting list cache, to avoid stressing or mixing up with the
	// global lcache (the LRU cache).
	txn := &Txn{StartTs: r.startTs}
	r.cache = make(map[string]*List)
	var numGets uint64
	txn.getList = func(key []byte) (*List, error) {
		numGets++
		if glog.V(2) && numGets%1000 == 0 {
			glog.Infof("During rebuild, getList hit %d times\n", numGets)
		}
		if pl, ok := r.cache[string(key)]; ok {
			return pl, nil
		}
		pl, err := getNew(key, pstore)
		if err != nil {
			return nil, err
		}
		r.cache[string(key)] = pl
		return pl, nil
	}

	var prevKey []byte
	for it.Rewind(); it.Valid(); {
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
	glog.V(1).Infof("Rebuild: Iteration done. Now commiting at ts=%d\n", r.startTs)

	// We must commit all the posting lists to memory, so they'd be picked up
	// during posting list rollup below.
	if err := txn.CommitToMemory(r.startTs); err != nil {
		return err
	}

	// Now we write all the created posting lists to disk.
	writer := x.NewTxnWriter(pstore)
	for key := range txn.deltas {
		pl, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		le := pl.Length(r.startTs, 0)
		if le == 0 {
			continue
		}
		kv, err := pl.MarshalToKv()
		if err != nil {
			return err
		}
		// We choose to write the PL at r.startTs, so it won't be read by txns,
		// which occurred before this schema mutation. Typically, we use
		// kv.Version as the timestamp.
		if err = writer.SetAt(kv.Key, kv.Val, kv.UserMeta[0], r.startTs); err != nil {
			return err
		}
		// This locking is just to catch any future issues.  We shouldn't need
		// to release this lock, because each posting list must only be accessed
		// once and never again.
		pl.Lock()
	}
	glog.V(1).Infoln("Rebuild: Flushing all writes.")
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
		err := pl.Iterate(txn.StartTs, 0, func(p *pb.Posting) error {
			// We only want to modify the untagged value. There could be other values with a
			// lang tag.
			if p.Uid == math.MaxUint64 {
				mpost = p
			}
			return nil
		})
		if err != nil {
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

		// Ensure that list is in the cache run by txn. Otherwise, nothing would
		// get updated.
		x.AssertTrue(builder.storeList(pl))
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
	glog.Infof("Dropping predicate: [%s]", attr)
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
