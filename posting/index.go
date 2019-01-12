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
	"reflect"
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

	if err := l.isUidMutationAllowedBySchema(ctx, txn, t); err != nil {
		return val, found, emptyCountParams, err
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
	glog.Infof("HACK. We're in AddMutationWithIndex. t: +%v\n", t)

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

		writer := NewTxnWriter(pstore)
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

func deleteAllEntries(prefix []byte) error {
	return deleteEntries(prefix, func(key []byte) bool {
		return true
	})
}

func deleteIndex(attr string) error {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	return deleteAllEntries(prefix)
}

func deleteReverseEdges(attr string) error {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.ReversePrefix()
	return deleteAllEntries(prefix)
}

func deleteCountIndex(attr string) error {
	pk := x.ParsedKey{Attr: attr}
	if err := deleteAllEntries(pk.CountPrefix(false)); err != nil {
		return err
	}

	return deleteAllEntries(pk.CountPrefix(true))
}

// Index rebuilding logic here.
type rebuild struct {
	prefix  []byte
	startTs uint64

	// The posting list passed here is the on disk version. It is not coming
	// from the LRU cache.
	fn func(uid uint64, pl *List, txn *Txn) error
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
	txn := NewTxn(r.startTs)

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
	writer := NewTxnWriter(pstore)
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
		if err = writer.SetAt(kv.Key, kv.Value, kv.UserMeta[0], r.startTs); err != nil {
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

// IndexRebuild holds the info needed to initiate a rebuilt of the indices.
type IndexRebuild struct {
	Attr          string
	StartTs       uint64
	OldSchema     *pb.SchemaUpdate
	CurrentSchema *pb.SchemaUpdate
}

type indexOp int

const (
	indexNoop    indexOp = iota // Index should be left alone.
	indexDelete          = iota // Index should be deleted.
	indexRebuild         = iota // Index should be deleted and rebuilt.
)

// Run rebuilds all indices that need it.
func (rb *IndexRebuild) Run(ctx context.Context) error {
	if err := RebuildListType(ctx, rb); err != nil {
		return err
	}
	if err := RebuildIndex(ctx, rb); err != nil {
		return err
	}
	if err := RebuildCountIndex(ctx, rb); err != nil {
		return err
	}
	return RebuildReverseEdges(ctx, rb)
}

func needsIndexRebuild(old *pb.SchemaUpdate, current *pb.SchemaUpdate) indexOp {
	x.AssertTruef(current != nil, "Current schema cannot be nil.")

	if old == nil {
		old = &pb.SchemaUpdate{}
	}

	currIndex := current.Directive == pb.SchemaUpdate_INDEX
	prevIndex := old.Directive == pb.SchemaUpdate_INDEX

	// Index does not need to be rebuilt or deleted if the scheme directive
	// did not require an index before and now.
	if !currIndex && !prevIndex {
		return indexNoop
	}

	// Index only needs to be deleted if the schema directive changed and the
	// new directive does not require an index. Predicate is not checking
	// prevIndex since the previous if statement guarantees both values are
	// different.
	if !currIndex {
		return indexDelete
	}

	// Index needs to be rebuilt if the value types have changed.
	if currIndex && current.ValueType != old.ValueType {
		return indexRebuild
	}

	// Index needs to be rebuilt if the tokenizers have changed
	prevTokens := make(map[string]bool)
	for _, t := range old.Tokenizer {
		prevTokens[t] = true
	}
	currTokens := make(map[string]bool)
	for _, t := range current.Tokenizer {
		currTokens[t] = true
	}

	if equal := reflect.DeepEqual(prevTokens, currTokens); equal {
		return indexNoop
	}
	return indexRebuild
}

// RebuildIndex rebuilds index for a given attribute.
// We commit mutations with startTs and ignore the errors.
func RebuildIndex(ctx context.Context, rb *IndexRebuild) error {
	// Exit early if indices do not need to be rebuilt.
	op := needsIndexRebuild(rb.OldSchema, rb.CurrentSchema)

	if op == indexNoop {
		return nil
	}

	glog.Infof("Deleting index for %s", rb.Attr)
	if err := deleteIndex(rb.Attr); err != nil {
		return err
	}

	// Exit early if the index only neeed to be deleted and not rebuild.
	if op == indexDelete {
		return nil
	}

	glog.Infof("Rebuilding index for %s", rb.Attr)
	pk := x.ParsedKey{Attr: rb.Attr}
	builder := rebuild{prefix: pk.DataPrefix(), startTs: rb.StartTs}
	builder.fn = func(uid uint64, pl *List, txn *Txn) error {
		edge := pb.DirectedEdge{Attr: rb.Attr, Entity: uid}
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

func needsCountIndexRebuild(old *pb.SchemaUpdate, current *pb.SchemaUpdate) indexOp {
	x.AssertTruef(current != nil, "Current schema cannot be nil.")

	if old == nil {
		old = &pb.SchemaUpdate{}
	}

	// Do nothing if the schema directive did not change.
	if current.Count == old.Count {
		return indexNoop

	}

	// If the new schema does not require an index, delete the current index.
	if !current.Count {
		return indexDelete
	}

	// Otherwise, the index needs to be rebuilt.
	return indexRebuild
}

// RebuildCountIndex rebuilds the count index for a given attribute.
func RebuildCountIndex(ctx context.Context, rb *IndexRebuild) error {
	op := needsCountIndexRebuild(rb.OldSchema, rb.CurrentSchema)
	if op == indexNoop {
		return nil
	}

	glog.Infof("Deleting count index for %s", rb.Attr)
	if err := deleteCountIndex(rb.Attr); err != nil {
		return err
	}

	// Exit early if attribute is index only needed to be deleted.
	if op == indexDelete {
		return nil
	}

	glog.Infof("Rebuilding count index for %s", rb.Attr)
	var reverse bool
	fn := func(uid uint64, pl *List, txn *Txn) error {
		t := &pb.DirectedEdge{
			ValueId: uid,
			Attr:    rb.Attr,
			Op:      pb.DirectedEdge_SET,
		}
		sz := pl.Length(rb.StartTs, 0)
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
	pk := x.ParsedKey{Attr: rb.Attr}
	builder := rebuild{prefix: pk.DataPrefix(), startTs: rb.StartTs}
	builder.fn = fn
	if err := builder.Run(ctx); err != nil {
		return err
	}

	// Create the reverse index.
	reverse = true
	builder = rebuild{prefix: pk.ReversePrefix(), startTs: rb.StartTs}
	builder.fn = fn
	return builder.Run(ctx)
}

func needsReverseEdgesRebuild(old *pb.SchemaUpdate, current *pb.SchemaUpdate) indexOp {
	x.AssertTruef(current != nil, "Current schema cannot be nil.")

	if old == nil {
		old = &pb.SchemaUpdate{}
	}

	currIndex := current.Directive == pb.SchemaUpdate_REVERSE
	prevIndex := old.Directive == pb.SchemaUpdate_REVERSE

	// If the schema directive did not change, return indexNoop.
	if currIndex == prevIndex {
		return indexNoop
	}

	// If the current schema requires an index, index should be rebuild.
	if currIndex {
		return indexRebuild
	}
	// Otherwise, index should only be deleted.
	return indexDelete
}

// RebuildReverseEdges rebuilds the reverse edges for a given attribute.
func RebuildReverseEdges(ctx context.Context, rb *IndexRebuild) error {
	op := needsReverseEdgesRebuild(rb.OldSchema, rb.CurrentSchema)
	if op == indexNoop {
		return nil
	}

	glog.Infof("Deleting reverse index for %s", rb.Attr)
	if err := deleteReverseEdges(rb.Attr); err != nil {
		return err
	}

	// Exit early if index only needed to be deleted.
	if op == indexDelete {
		return nil
	}

	glog.Infof("Rebuilding reverse index for %s", rb.Attr)
	pk := x.ParsedKey{Attr: rb.Attr}
	builder := rebuild{prefix: pk.DataPrefix(), startTs: rb.StartTs}
	builder.fn = func(uid uint64, pl *List, txn *Txn) error {
		edge := pb.DirectedEdge{Attr: rb.Attr, Entity: uid}
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

// needsListTypeRebuild returns true if the schema changed from a scalar to a
// list. It returns true if the index can be left as is.
func needsListTypeRebuild(old *pb.SchemaUpdate, current *pb.SchemaUpdate) (bool, error) {
	x.AssertTruef(current != nil, "Current schema cannot be nil.")

	if old == nil {
		return false, nil
	}
	if current.List && !old.List {
		return true, nil
	}
	if old.List && !current.List {
		return false, fmt.Errorf("Type can't be changed from list to scalar for attr: [%s]"+
			" without dropping it first.", current.Predicate)
	}

	return false, nil
}

// RebuildListType rebuilds the index when the schema is changed from scalar to list type.
// We need to fingerprint the values to get the new ValueId.
func RebuildListType(ctx context.Context, rb *IndexRebuild) error {
	if needsRebuild, err := needsListTypeRebuild(rb.OldSchema, rb.CurrentSchema); !needsRebuild ||
		err != nil {
		return err
	}

	pk := x.ParsedKey{Attr: rb.Attr}
	builder := rebuild{prefix: pk.DataPrefix(), startTs: rb.StartTs}
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
			Attr:    rb.Attr,
			Op:      pb.DirectedEdge_DEL,
		}

		// Ensure that list is in the cache run by txn. Otherwise, nothing would
		// get updated.
		txn.cache.Set(string(pl.key), pl)
		if err := pl.AddMutation(ctx, txn, t); err != nil {
			return err
		}
		// Add the new edge with the fingerprinted value id.
		newEdge := &pb.DirectedEdge{
			Attr:      rb.Attr,
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

// DeleteAll deletes all entries in the posting list.
func DeleteAll() error {
	return deleteEntries(nil, func(key []byte) bool {
		pk := x.Parse(key)
		if pk == nil {
			return true
		} else if pk.IsSchema() {
			// Don't delete schema for _predicate_
			_, isInitialPred := x.InitialPreds[pk.Attr]
			return !isInitialPred
		}
		return true
	})
}

// DeletePredicate deletes all entries and indices for a given predicate.
func DeletePredicate(ctx context.Context, attr string) error {
	glog.Infof("Dropping predicate: [%s]", attr)
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
		if err := deleteIndex(attr); err != nil {
			return err
		}
	} else if reversed {
		if err := deleteReverseEdges(attr); err != nil {
			return err
		}
	}

	hasCountIndex := schema.State().HasCount(attr)
	if hasCountIndex {
		if err := deleteCountIndex(attr); err != nil {
			return err
		}
	}

	return schema.State().Delete(attr)
}
