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
	"io/ioutil"
	"math"
	"os"
	"time"

	"github.com/golang/glog"
	ostats "go.opencensus.io/stats"
	otrace "go.opencensus.io/trace"

	"github.com/dgraph-io/badger/v2"
	bpb "github.com/dgraph-io/badger/v2/pb"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/tok"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
	"github.com/pkg/errors"
)

var emptyCountParams countParams

type indexMutationInfo struct {
	tokenizers []tok.Tokenizer
	edge       *pb.DirectedEdge // Represents the original uid -> value edge.
	val        types.Val
	op         pb.DirectedEdge_Op
}

// indexTokensforTokenizers return tokens, without the predicate prefix and
// index rune, for specific tokenizers.
func indexTokens(info *indexMutationInfo) ([]string, error) {
	attr := info.edge.Attr
	lang := info.edge.GetLang()

	schemaType, err := schema.State().TypeOf(attr)
	if err != nil || !schemaType.IsScalar() {
		return nil, errors.Errorf("Cannot index attribute %s of type object.", attr)
	}

	if !schema.State().IsIndexed(attr) {
		return nil, errors.Errorf("Attribute %s is not indexed.", attr)
	}
	sv, err := types.Convert(info.val, schemaType)
	if err != nil {
		return nil, err
	}

	var tokens []string
	for _, it := range info.tokenizers {
		toks, err := tok.BuildTokens(sv.Value, tok.GetLangTokenizer(it, lang))
		if err != nil {
			return tokens, err
		}
		tokens = append(tokens, toks...)
	}
	return tokens, nil
}

// addIndexMutations adds mutation(s) for a single term, to maintain the index,
// but only for the given tokenizers.
// TODO - See if we need to pass op as argument as t should already have Op.
func (txn *Txn) addIndexMutations(ctx context.Context, info *indexMutationInfo) error {
	if info.tokenizers == nil {
		info.tokenizers = schema.State().Tokenizer(info.edge.Attr)
	}

	attr := info.edge.Attr
	uid := info.edge.Entity
	x.AssertTrue(uid != 0)
	tokens, err := indexTokens(info)

	if err != nil {
		// This data is not indexable
		return err
	}

	// Create a value token -> uid edge.
	edge := &pb.DirectedEdge{
		ValueId: uid,
		Attr:    attr,
		Op:      info.op,
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

	plist, err := txn.cache.GetFromDelta(key)
	if err != nil {
		return err
	}

	x.AssertTrue(plist != nil)
	if err = plist.addMutation(ctx, txn, edge); err != nil {
		return err
	}
	ostats.Record(ctx, x.NumEdges.M(1))
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

	if hasCountIndex {
		countBefore = plist.Length(txn.StartTs, 0)
		if countBefore == -1 {
			return emptyCountParams, ErrTsTooOld
		}
	}
	if err := plist.addMutation(ctx, txn, edge); err != nil {
		return emptyCountParams, err
	}
	if hasCountIndex {
		countAfter = plist.Length(txn.StartTs, 0)
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
	hasCountIndex := schema.State().HasCount(t.Attr)

	var getFn func(key []byte) (*List, error)
	if hasCountIndex {
		// We need to retrieve the full posting list from disk, to allow us to get the length of the
		// posting list for the counts.
		getFn = txn.Get
	} else {
		// We are just adding a reverse edge. No need to read the list from disk.
		getFn = txn.GetFromDelta
	}
	plist, err := getFn(key)
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

	cp, err := txn.addReverseMutationHelper(ctx, plist, hasCountIndex, edge)
	if err != nil {
		return err
	}
	ostats.Record(ctx, x.NumEdges.M(1))

	if hasCountIndex && cp.countAfter != cp.countBefore {
		if err := txn.updateCount(ctx, cp); err != nil {
			return err
		}
	}
	return nil
}

func (l *List) handleDeleteAll(ctx context.Context, edge *pb.DirectedEdge,
	txn *Txn) error {
	isReversed := schema.State().IsReversed(edge.Attr)
	isIndexed := schema.State().IsIndexed(edge.Attr)
	hasCount := schema.State().HasCount(edge.Attr)
	delEdge := &pb.DirectedEdge{
		Attr:   edge.Attr,
		Op:     edge.Op,
		Entity: edge.Entity,
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
			val := types.Val{
				Tid:   types.TypeID(p.ValType),
				Value: p.Value,
			}
			return txn.addIndexMutations(ctx, &indexMutationInfo{
				tokenizers: schema.State().Tokenizer(edge.Attr),
				edge:       edge,
				val:        val,
				op:         pb.DirectedEdge_DEL,
			})
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
			attr:        edge.Attr,
			countBefore: plen,
			countAfter:  0,
			entity:      edge.Entity,
		}); err != nil {
			return err
		}
	}

	return l.addMutation(ctx, txn, edge)
}

func (txn *Txn) addCountMutation(ctx context.Context, t *pb.DirectedEdge, count uint32,
	reverse bool) error {
	key := x.CountKey(t.Attr, count, reverse)
	plist, err := txn.cache.GetFromDelta(key)
	if err != nil {
		return err
	}

	x.AssertTruef(plist != nil, "plist is nil [%s] %d",
		t.Attr, t.ValueId)
	if err = plist.addMutation(ctx, txn, t); err != nil {
		return err
	}
	ostats.Record(ctx, x.NumEdges.M(1))
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

	if err := l.canMutateUid(txn, t); err != nil {
		return val, found, emptyCountParams, err
	}

	if doUpdateIndex {
		// Check original value BEFORE any mutation actually happens.
		val, found, err = l.findValue(txn.StartTs, fingerprintEdge(t))
		if err != nil {
			return val, found, emptyCountParams, err
		}
	}

	// If the predicate schema is not a list, ignore delete triples whose object is not a star or
	// a value that does not match the existing value.
	if !schema.State().IsList(t.Attr) && t.Op == pb.DirectedEdge_DEL && string(t.Value) != x.Star {
		newPost := NewPosting(t)
		pFound, currPost, err := l.findPosting(txn.StartTs, fingerprintEdge(t))
		if err != nil {
			return val, found, emptyCountParams, err
		}

		// This is a scalar value of non-list type and a delete edge mutation, so if the value
		// given by the user doesn't match the value we have, we return found to be false, to avoid
		// deleting the uid from index posting list.
		// This second check is required because we fingerprint the scalar values as math.MaxUint64,
		// so even though they might be different the check in the doUpdateIndex block above would
		// return found to be true.
		if pFound && !(bytes.Equal(currPost.Value, newPost.Value) &&
			types.TypeID(currPost.ValType) == types.TypeID(newPost.ValType)) {
			return val, false, emptyCountParams, nil
		}
	}

	countBefore, countAfter := 0, 0
	if hasCountIndex {
		countBefore = l.length(txn.StartTs, 0)
		if countBefore == -1 {
			return val, found, emptyCountParams, ErrTsTooOld
		}
	}
	if err = l.addMutationInternal(ctx, txn, t); err != nil {
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

// AddMutationWithIndex is addMutation with support for indexing. It also
// supports reverse edges.
func (l *List) AddMutationWithIndex(ctx context.Context, edge *pb.DirectedEdge,
	txn *Txn) error {
	if len(edge.Attr) == 0 {
		return errors.Errorf("Predicate cannot be empty for edge with subject: [%v], object: [%v]"+
			" and value: [%v]", edge.Entity, edge.ValueId, edge.Value)
	}

	if edge.Op == pb.DirectedEdge_DEL && string(edge.Value) == x.Star {
		return l.handleDeleteAll(ctx, edge, txn)
	}

	doUpdateIndex := pstore != nil && schema.State().IsIndexed(edge.Attr)
	hasCountIndex := schema.State().HasCount(edge.Attr)
	val, found, cp, err := txn.addMutationHelper(ctx, l, doUpdateIndex, hasCountIndex, edge)
	if err != nil {
		return err
	}
	ostats.Record(ctx, x.NumEdges.M(1))
	if hasCountIndex && cp.countAfter != cp.countBefore {
		if err := txn.updateCount(ctx, cp); err != nil {
			return err
		}
	}
	if doUpdateIndex {
		// Exact matches.
		if found && val.Value != nil {
			if err := txn.addIndexMutations(ctx, &indexMutationInfo{
				tokenizers: schema.State().Tokenizer(edge.Attr),
				edge:       edge,
				val:        val,
				op:         pb.DirectedEdge_DEL,
			}); err != nil {
				return err
			}
		}
		if edge.Op == pb.DirectedEdge_SET {
			val = types.Val{
				Tid:   types.TypeID(edge.ValueType),
				Value: edge.Value,
			}
			if err := txn.addIndexMutations(ctx, &indexMutationInfo{
				tokenizers: schema.State().Tokenizer(edge.Attr),
				edge:       edge,
				val:        val,
				op:         pb.DirectedEdge_SET,
			}); err != nil {
				return err
			}
		}
	}
	// Add reverse mutation irrespective of hasMutated, server crash can happen after
	// mutation is synced and before reverse edge is synced
	if (pstore != nil) && (edge.ValueId != 0) && schema.State().IsReversed(edge.Attr) {
		if err := txn.addReverseMutation(ctx, edge); err != nil {
			return err
		}
	}
	return nil
}

// deleteTokensFor deletes the index for the given attribute and token.
func deleteTokensFor(attr, tokenizerName string) error {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.IndexPrefix()
	tokenizer, ok := tok.GetTokenizer(tokenizerName)
	if !ok {
		return errors.Errorf("Could not find valid tokenizer for %s", tokenizerName)
	}
	prefix = append(prefix, tokenizer.Identifier())
	if err := pstore.DropPrefix(prefix); err != nil {
		return err
	}

	// Also delete all the parts of any list that has been split into multiple parts.
	// Such keys have a different prefix (the last byte is set to 1).
	prefix = pk.IndexPrefix()
	prefix[len(prefix)-1] = x.ByteSplit
	prefix = append(prefix, tokenizer.Identifier())
	return pstore.DropPrefix(prefix)
}

func deleteReverseEdges(attr string) error {
	pk := x.ParsedKey{Attr: attr}
	prefix := pk.ReversePrefix()
	if err := pstore.DropPrefix(prefix); err != nil {
		return err
	}

	// Also delete all the parts of any list that has been split into multiple parts.
	// Such keys have a different prefix (the last byte is set to 1).
	prefix = pk.ReversePrefix()
	prefix[len(prefix)-1] = x.ByteSplit

	return pstore.DropPrefix(prefix)
}

func deleteCountIndex(attr string) error {
	pk := x.ParsedKey{Attr: attr}
	if err := pstore.DropPrefix(pk.CountPrefix(false)); err != nil {
		return err
	}
	if err := pstore.DropPrefix(pk.CountPrefix(true)); err != nil {
		return err
	}

	// Also delete all the parts of any list that has been split into multiple parts.
	// Such keys have a different prefix (the last byte is set to 1).
	prefix := pk.CountPrefix(false)
	prefix[len(prefix)-1] = x.ByteSplit
	if err := pstore.DropPrefix(prefix); err != nil {
		return err
	}

	prefix = pk.CountPrefix(true)
	prefix[len(prefix)-1] = x.ByteSplit
	return pstore.DropPrefix(prefix)
}

// rebuilder handles the process of rebuilding an index.
type rebuilder struct {
	attr    string
	prefix  []byte
	startTs uint64

	// The posting list passed here is the on disk version. It is not coming
	// from the LRU cache.
	fn func(uid uint64, pl *List, txn *Txn) error
}

func (r *rebuilder) Run(ctx context.Context) error {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	glog.V(1).Infof("Rebuilding indexes using the tmp folder %s\n", tmpDir)

	opts := badger.DefaultOptions(tmpDir).WithSyncWrites(false)
	tmpDB, err := badger.Open(opts)
	if err != nil {
		return err
	}
	defer tmpDB.Close()

	glog.V(1).Infof(
		"Rebuilding index for predicate %s: Starting process. StartTs=%d. Prefix=\n%s\n",
		r.attr, r.startTs, hex.Dump(r.prefix))

	// We create one txn for all the mutations to be housed in. We also create a
	// localized posting list cache, to avoid stressing or mixing up with the
	// global lcache (the LRU cache).
	txn := NewTxn(r.startTs)

	stream := pstore.NewStreamAt(r.startTs)
	stream.LogPrefix = fmt.Sprintf("Rebuilding index for predicate %s:", r.attr)
	stream.Prefix = r.prefix
	stream.KeyToList = func(key []byte, itr *badger.Iterator) (*bpb.KVList, error) {
		// We should return quickly if the context is no longer valid.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		pk, err := x.Parse(key)
		if err != nil {
			return nil, errors.Wrapf(err, "could not parse key %s", hex.Dump(key))
		}

		item := itr.Item()
		keyCopy := item.KeyCopy(nil)
		l, err := ReadPostingList(keyCopy, itr)
		if err != nil {
			return nil, err
		}
		if err := r.fn(pk.Uid, l, txn); err != nil {
			return nil, err
		}

		// Convert data into deltas.
		txn.Update()

		// Write deltas into tmp badger
		tmpTxn := tmpDB.NewTransaction(true)
		txn.cache.Lock()
		for key, data := range txn.cache.deltas {
			tmpTxn.SetEntry(&badger.Entry{
				Key:      []byte(key),
				Value:    data,
				UserMeta: BitDeltaPosting,
			})
		}
		// Reset deltas
		for k := range txn.cache.deltas {
			delete(txn.cache.deltas, k)
		}
		txn.cache.Unlock()
		tmpTxn.Commit()

		return nil, nil
	}
	stream.Send = func(*bpb.KVList) error {
		// The work of adding the index edges to the transaction is done by r.fn
		// so this function doesn't have any work to do.
		return nil
	}

	if err := stream.Orchestrate(ctx); err != nil {
		return err
	}
	glog.V(1).Infof("Rebuilding index for predicate %s: Iteration done. Now committing at ts=%d\n",
		r.attr, r.startTs)

	// Now we write all the created posting lists to disk.
	tmpTxn := tmpDB.NewTransaction(true)
	defer tmpTxn.Discard()
	iterOpts := badger.DefaultIteratorOptions
	iterOpts.AllVersions = true
	it := tmpTxn.NewIterator(iterOpts)
	defer it.Close()

	writer := NewTxnWriter(pstore)
	for it.Rewind(); it.Valid(); {
		key := it.Item().Key()

		l, err := ReadPostingList(key, it)
		if err != nil {
			return err
		}

		kvs, err := l.Rollup()
		if err != nil {
			return err
		}
		for _, kv := range kvs {
			if len(kv.Value) == 0 {
				continue
			}

			// We choose to write the PL at r.startTs, so it won't be read by txns,
			// which occurred before this schema mutation. Typically, we use
			// kv.Version as the timestamp.
			err := writer.SetAt([]byte(kv.Key), kv.Value, BitCompletePosting, r.startTs)
			if err != nil {
				return err
			}
		}
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
	if err := rebuildListType(ctx, rb); err != nil {
		return err
	}
	if err := rebuildIndex(ctx, rb); err != nil {
		return err
	}
	if err := rebuildReverseEdges(ctx, rb); err != nil {
		return err
	}
	return rebuildCountIndex(ctx, rb)
}

type indexRebuildInfo struct {
	op                  indexOp
	tokenizersToDelete  []string
	tokenizersToRebuild []string
}

func (rb *IndexRebuild) needsIndexRebuild() indexRebuildInfo {
	x.AssertTruef(rb.CurrentSchema != nil, "Current schema cannot be nil.")

	// If the old schema is nil, we can treat it as an empty schema. Copy it
	// first to avoid overwriting it in rb.
	old := rb.OldSchema
	if old == nil {
		old = &pb.SchemaUpdate{}
	}

	currIndex := rb.CurrentSchema.Directive == pb.SchemaUpdate_INDEX
	prevIndex := old.Directive == pb.SchemaUpdate_INDEX

	// Index does not need to be rebuilt or deleted if the scheme directive
	// did not require an index before and now.
	if !currIndex && !prevIndex {
		return indexRebuildInfo{
			op: indexNoop,
		}
	}

	// Index only needs to be deleted if the schema directive changed and the
	// new directive does not require an index. Predicate is not checking
	// prevIndex since the previous if statement guarantees both values are
	// different.
	if !currIndex {
		return indexRebuildInfo{
			op:                 indexDelete,
			tokenizersToDelete: old.Tokenizer,
		}
	}

	// All tokenizers in the index need to be deleted and rebuilt if the value
	// types have changed.
	if currIndex && rb.CurrentSchema.ValueType != old.ValueType {
		return indexRebuildInfo{
			op:                  indexRebuild,
			tokenizersToDelete:  old.Tokenizer,
			tokenizersToRebuild: rb.CurrentSchema.Tokenizer,
		}
	}

	// Index needs to be rebuilt if the tokenizers have changed
	prevTokens := make(map[string]struct{})
	for _, t := range old.Tokenizer {
		prevTokens[t] = struct{}{}
	}
	currTokens := make(map[string]struct{})
	for _, t := range rb.CurrentSchema.Tokenizer {
		currTokens[t] = struct{}{}
	}

	newTokenizers, deletedTokenizers := x.Diff(currTokens, prevTokens)

	// If the tokenizers are the same, nothing needs to be done.
	if len(newTokenizers) == 0 && len(deletedTokenizers) == 0 {
		return indexRebuildInfo{
			op: indexNoop,
		}
	}

	return indexRebuildInfo{
		op:                  indexRebuild,
		tokenizersToDelete:  deletedTokenizers,
		tokenizersToRebuild: newTokenizers,
	}
}

// rebuildIndex rebuilds index for a given attribute.
// We commit mutations with startTs and ignore the errors.
func rebuildIndex(ctx context.Context, rb *IndexRebuild) error {
	// Exit early if indices do not need to be rebuilt.
	rebuildInfo := rb.needsIndexRebuild()

	if rebuildInfo.op == indexNoop {
		return nil
	}

	glog.Infof("Deleting index for attr %s and tokenizers %s", rb.Attr,
		rebuildInfo.tokenizersToDelete)
	for _, tokenizer := range rebuildInfo.tokenizersToDelete {
		if err := deleteTokensFor(rb.Attr, tokenizer); err != nil {
			return err
		}
	}

	// Exit early if the index only need to be deleted and not rebuilt.
	if rebuildInfo.op == indexDelete {
		return nil
	}

	// Exit early if there are no tokenizers to rebuild.
	if len(rebuildInfo.tokenizersToRebuild) == 0 {
		return nil
	}

	glog.Infof("Rebuilding index for attr %s and tokenizers %s", rb.Attr,
		rebuildInfo.tokenizersToRebuild)
	// Before rebuilding, the existing index needs to be deleted.
	for _, tokenizer := range rebuildInfo.tokenizersToRebuild {
		if err := deleteTokensFor(rb.Attr, tokenizer); err != nil {
			return err
		}
	}

	tokenizers, err := tok.GetTokenizers(rebuildInfo.tokenizersToRebuild)
	if err != nil {
		return err
	}

	pk := x.ParsedKey{Attr: rb.Attr}
	builder := rebuilder{attr: rb.Attr, prefix: pk.DataPrefix(), startTs: rb.StartTs}
	builder.fn = func(uid uint64, pl *List, txn *Txn) error {
		edge := pb.DirectedEdge{Attr: rb.Attr, Entity: uid}
		return pl.Iterate(txn.StartTs, 0, func(p *pb.Posting) error {
			// Add index entries based on p.
			val := types.Val{
				Value: p.Value,
				Tid:   types.TypeID(p.ValType),
			}

			for {
				err := txn.addIndexMutations(ctx, &indexMutationInfo{
					tokenizers: tokenizers,
					edge:       &edge,
					val:        val,
					op:         pb.DirectedEdge_SET,
				})
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

func (rb *IndexRebuild) needsCountIndexRebuild() indexOp {
	x.AssertTruef(rb.CurrentSchema != nil, "Current schema cannot be nil.")

	// If the old schema is nil, treat it as an empty schema. Copy it to avoid
	// overwriting it in rb.
	old := rb.OldSchema
	if old == nil {
		old = &pb.SchemaUpdate{}
	}

	// Do nothing if the schema directive did not change.
	if rb.CurrentSchema.Count == old.Count {
		return indexNoop

	}

	// If the new schema does not require an index, delete the current index.
	if !rb.CurrentSchema.Count {
		return indexDelete
	}

	// Otherwise, the index needs to be rebuilt.
	return indexRebuild
}

// rebuildCountIndex rebuilds the count index for a given attribute.
func rebuildCountIndex(ctx context.Context, rb *IndexRebuild) error {
	op := rb.needsCountIndexRebuild()
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
	builder := rebuilder{attr: rb.Attr, prefix: pk.DataPrefix(), startTs: rb.StartTs}
	builder.fn = fn
	if err := builder.Run(ctx); err != nil {
		return err
	}

	// Create the reverse index. The count reverse index is created if this
	// predicate has both a count and reverse directive in the schema. It's safe
	// to call builder.Run even if that's not the case as the reverse prefix
	// will be empty.
	reverse = true
	builder = rebuilder{attr: rb.Attr, prefix: pk.ReversePrefix(), startTs: rb.StartTs}
	builder.fn = fn
	return builder.Run(ctx)
}

func (rb *IndexRebuild) needsReverseEdgesRebuild() indexOp {
	x.AssertTruef(rb.CurrentSchema != nil, "Current schema cannot be nil.")

	// If old schema is nil, treat it as an empty schema. Copy it to avoid
	// overwriting it in rb.
	old := rb.OldSchema
	if old == nil {
		old = &pb.SchemaUpdate{}
	}

	currIndex := rb.CurrentSchema.Directive == pb.SchemaUpdate_REVERSE
	prevIndex := old.Directive == pb.SchemaUpdate_REVERSE

	// If the schema directive did not change, return indexNoop.
	if currIndex == prevIndex {
		return indexNoop
	}

	// If the current schema requires an index, index should be rebuilt.
	if currIndex {
		return indexRebuild
	}
	// Otherwise, index should only be deleted.
	return indexDelete
}

// rebuildReverseEdges rebuilds the reverse edges for a given attribute.
func rebuildReverseEdges(ctx context.Context, rb *IndexRebuild) error {
	op := rb.needsReverseEdgesRebuild()
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
	builder := rebuilder{attr: rb.Attr, prefix: pk.DataPrefix(), startTs: rb.StartTs}
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
func (rb *IndexRebuild) needsListTypeRebuild() (bool, error) {
	x.AssertTruef(rb.CurrentSchema != nil, "Current schema cannot be nil.")

	if rb.OldSchema == nil {
		return false, nil
	}
	if rb.CurrentSchema.List && !rb.OldSchema.List {
		return true, nil
	}
	if rb.OldSchema.List && !rb.CurrentSchema.List {
		return false, errors.Errorf("Type can't be changed from list to scalar for attr: [%s]"+
			" without dropping it first.", rb.CurrentSchema.Predicate)
	}

	return false, nil
}

// rebuildListType rebuilds the index when the schema is changed from scalar to list type.
// We need to fingerprint the values to get the new ValueId.
func rebuildListType(ctx context.Context, rb *IndexRebuild) error {
	if needsRebuild, err := rb.needsListTypeRebuild(); !needsRebuild || err != nil {
		return err
	}

	pk := x.ParsedKey{Attr: rb.Attr}
	builder := rebuilder{attr: rb.Attr, prefix: pk.DataPrefix(), startTs: rb.StartTs}
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
		pl = txn.cache.SetIfAbsent(string(pl.key), pl)
		if err := pl.addMutation(ctx, txn, t); err != nil {
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
		return pl.addMutation(ctx, txn, newEdge)
	}
	return builder.Run(ctx)
}

// DeleteAll deletes all entries in the posting list.
func DeleteAll() error {
	return pstore.DropAll()
}

// DeleteData deletes all data but leaves types and schema intact.
func DeleteData() error {
	return pstore.DropPrefix([]byte{x.DefaultPrefix})
}

// DeletePredicate deletes all entries and indices for a given predicate.
func DeletePredicate(ctx context.Context, attr string) error {
	glog.Infof("Dropping predicate: [%s]", attr)
	prefix := x.PredicatePrefix(attr)
	if err := pstore.DropPrefix(prefix); err != nil {
		return err
	}

	return schema.State().Delete(attr)
}
