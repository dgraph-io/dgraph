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

package worker

import (
	"bytes"
	"math/rand"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/trace"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

var (
	errUnservedTablet  = x.Errorf("Tablet isn't being served by this instance.")
	errPredicateMoving = x.Errorf("Predicate is being moved, please retry later")
	allocator          x.EmbeddedUidAllocator
)

// runMutation goes through all the edges and applies them. It returns the
// mutations which were not applied in left.
func runMutation(ctx context.Context, edge *protos.DirectedEdge, txn *posting.Txn) error {
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("In run mutations")
	}
	if !groups().ServesTablet(edge.Attr) {
		return errUnservedTablet
	}

	rv := ctx.Value("raft").(x.RaftValue)
	typ, err := schema.State().TypeOf(edge.Attr)
	x.Checkf(err, "Schema is not present for predicate %s", edge.Attr)

	if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
		n := groups().Node
		if err = n.syncAllMarks(ctx, rv.Index-1); err != nil {
			return err
		}
		if err = txn.DeletePredicate(ctx, edge.Attr); err != nil {
			return err
		}
		return nil
	}
	// Once mutation comes via raft we do best effort conversion
	// Type check is done before proposing mutation, in case schema is not
	// present, some invalid entries might be written initially
	err = ValidateAndConvert(edge, typ)

	key := x.DataKey(edge.Attr, edge.Entity)

	t := time.Now()
	plist := posting.Get(key)
	if dur := time.Since(t); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("GetLru took %v", dur)
		}
	}

	if err = plist.AddMutationWithIndex(ctx, edge, txn); err != nil {
		return err // abort applying the rest of them.
	}
	return nil
}

func abortMutations(ctx context.Context, tc *protos.TxnContext) error {
	for _, key := range tc.Keys {
		plist := posting.Get([]byte(key))
		if err := plist.AbortTransaction(ctx, tc.StartTs); err != nil {
			return err
		}
	}
	return posting.AbortMutations(tc.Keys, tc.StartTs)
}

func commitMutations(ctx context.Context, tc *protos.TxnContext) error {
	for _, key := range tc.Keys {
		plist := posting.Get([]byte(key))
		if err := plist.CommitMutation(ctx, tc.StartTs, tc.CommitTs); err != nil {
			return err
		}
	}
	return posting.CommitMutations(tc, groups().ServesTablet(tc.Primary))
}

// This is serialized with mutations, called after applied watermarks catch up
// and further mutations are blocked until this is done.
func runSchemaMutation(ctx context.Context, update *protos.SchemaUpdate, txn *posting.Txn) error {
	rv := ctx.Value("raft").(x.RaftValue)
	n := groups().Node
	// Wait for applied watermark to reach till previous index
	// All mutations before this should use old schema and after this
	// should use new schema
	if err := n.syncAllMarks(n.ctx, rv.Index-1); err != nil {
		return err
	}
	if !groups().ServesTablet(update.Predicate) {
		return errUnservedTablet
	}
	if err := checkSchema(update, txn.StartTs); err != nil {
		return err
	}
	old, ok := schema.State().Get(update.Predicate)
	current := *update
	updateSchema(update.Predicate, current, rv.Index, rv.Group)

	// Once we remove index or reverse edges from schema, even though the values
	// are present in db, they won't be used due to validation in work/task.go

	// We don't want to use sync watermarks for background removal, because it would block
	// linearizable read requests. Only downside would be on system crash, stale edges
	// might remain, which is ok.

	// Indexing can't be done in background as it can cause race conditons with new
	// index mutations (old set and new del)
	// We need watermark for index/reverse edge addition for linearizable reads.
	// (both applied and synced watermarks).
	defer x.Printf("Done schema update %+v\n", update)
	if !ok {
		if current.Directive == protos.SchemaUpdate_INDEX {
			if err := n.rebuildOrDelIndex(ctx, update.Predicate, true, txn); err != nil {
				return err
			}
		} else if current.Directive == protos.SchemaUpdate_REVERSE {
			if err := n.rebuildOrDelRevEdge(ctx, update.Predicate, true, txn); err != nil {
				return err
			}
		}

		if current.Count {
			if err := n.rebuildOrDelCountIndex(ctx, update.Predicate, true, txn); err != nil {
				return err
			}
		}
		return nil
	}
	// schema was present already
	if needReindexing(old, current) {
		// Reindex if update.Index is true or remove index
		if err := n.rebuildOrDelIndex(ctx, update.Predicate,
			current.Directive == protos.SchemaUpdate_INDEX, txn); err != nil {
			return err
		}
	} else if needsRebuildingReverses(old, current) {
		// Add or remove reverse edge based on update.Reverse
		if err := n.rebuildOrDelRevEdge(ctx, update.Predicate,
			current.Directive == protos.SchemaUpdate_REVERSE, txn); err != nil {
			return err
		}
	}

	if current.Count != old.Count {
		if err := n.rebuildOrDelCountIndex(ctx, update.Predicate, current.Count, txn); err != nil {
		}
	}
	return nil
}

func needsRebuildingReverses(old protos.SchemaUpdate, current protos.SchemaUpdate) bool {
	return (current.Directive == protos.SchemaUpdate_REVERSE) !=
		(old.Directive == protos.SchemaUpdate_REVERSE)
}

func needReindexing(old protos.SchemaUpdate, current protos.SchemaUpdate) bool {
	if (current.Directive == protos.SchemaUpdate_INDEX) != (old.Directive == protos.SchemaUpdate_INDEX) {
		return true
	}
	// if value types has changed
	if current.Directive == protos.SchemaUpdate_INDEX && current.ValueType != old.ValueType {
		return true
	}
	// if tokenizer has changed - if same tokenizer works differently
	// on different types
	if len(current.Tokenizer) != len(old.Tokenizer) {
		return true
	}
	for i, t := range old.Tokenizer {
		if current.Tokenizer[i] != t {
			return true
		}
	}

	return false
}

func updateSchema(attr string, s protos.SchemaUpdate, raftIndex uint64, group uint32) {
	ce := schema.SyncEntry{
		Attr:   attr,
		Schema: s,
		Index:  raftIndex,
		Water:  posting.SyncMarks(),
	}
	schema.State().Update(ce)
}

func updateSchemaType(attr string, typ types.TypeID, raftIndex uint64, group uint32) {
	// Don't overwrite schema blindly, acl's might have been set even though
	// type is not present
	s, ok := schema.State().Get(attr)
	if ok {
		s.ValueType = uint32(typ)
	} else {
		s = protos.SchemaUpdate{ValueType: uint32(typ)}
	}
	updateSchema(attr, s, raftIndex, group)
}

func hasEdges(attr string, startTs uint64) bool {
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.PrefetchValues = false
	txn := pstore.NewTransactionAt(startTs, false)
	defer txn.Discard()
	it := txn.NewIterator(iterOpt)
	defer it.Close()
	pk := x.ParsedKey{
		Attr: attr,
	}
	prefix := pk.DataPrefix()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		return true
	}
	return false
}

func checkSchema(s *protos.SchemaUpdate, startTs uint64) error {
	if len(s.Predicate) == 0 {
		return x.Errorf("No predicate specified in schema mutation")
	}

	if s.Directive == protos.SchemaUpdate_INDEX && len(s.Tokenizer) == 0 {
		return x.Errorf("Tokenizer must be specified while indexing a predicate: %+v", s)
	}

	if len(s.Tokenizer) > 0 && s.Directive != protos.SchemaUpdate_INDEX {
		return x.Errorf("Directive must be SchemaUpdate_INDEX when a tokenizer is specified")
	}

	typ := types.TypeID(s.ValueType)
	if typ == types.UidID && s.Directive == protos.SchemaUpdate_INDEX {
		// index on uid type
		return x.Errorf("Index not allowed on predicate of type uid on predicate %s",
			s.Predicate)
	} else if typ != types.UidID && s.Directive == protos.SchemaUpdate_REVERSE {
		// reverse on non-uid type
		return x.Errorf("Cannot reverse for non-uid type on predicate %s", s.Predicate)
	}
	if t, err := schema.State().TypeOf(s.Predicate); err == nil {
		// schema was defined already
		if t.IsScalar() == typ.IsScalar() {
			// If old type was list and new type is non-list, we don't allow it until user
			// has data.
			if schema.State().IsList(s.Predicate) && !s.List && hasEdges(s.Predicate, startTs) {
				return x.Errorf("Schema change not allowed from [%s] => %s without"+
					" deleting pred: %s", t.Name(), typ.Name(), s.Predicate)
			}
			// New and old type are both scalar or both are uid. Allow schema change.
			return nil
		}
		// uid => scalar or scalar => uid. Check that there shouldn't be any data.
		if hasEdges(s.Predicate, startTs) {
			return x.Errorf("Schema change not allowed from predicate to uid or vice versa"+
				" till you have data for pred: %s", s.Predicate)
		}
	}
	return nil
}

// If storage type is specified, then check compatibility or convert to schema type
// if no storage type is specified then convert to schema type.
func ValidateAndConvert(edge *protos.DirectedEdge, schemaType types.TypeID) error {
	if types.TypeID(edge.ValueType) == types.DefaultID && string(edge.Value) == x.Star {
		if edge.Op != protos.DirectedEdge_DEL {
			return x.Errorf("* allowed only with delete operation")
		}
		return nil
	}

	storageType := posting.TypeID(edge)
	if !schemaType.IsScalar() && !storageType.IsScalar() {
		return nil
	} else if !schemaType.IsScalar() && storageType.IsScalar() {
		return x.Errorf("Input for predicate %s of type uid is scalar", edge.Attr)
	} else if schemaType.IsScalar() && !storageType.IsScalar() {
		return x.Errorf("Input for predicate %s of type scalar is uid", edge.Attr)
	} else {
		// Both are scalars. Continue.
	}

	if storageType == schemaType {
		return nil
	}

	var src types.Val
	var dst types.Val
	var err error

	src = types.Val{types.TypeID(edge.ValueType), edge.Value}
	// check compatibility of schema type and storage type
	if dst, err = types.Convert(src, schemaType); err != nil {
		return err
	}

	// if storage type was specified skip
	if storageType != types.DefaultID {
		return nil
	}

	// convert to schema type
	b := types.ValueForType(types.BinaryID)
	if err = types.Marshal(dst, &b); err != nil {
		return err
	}
	edge.ValueType = uint32(schemaType)
	edge.Value = b.Value.([]byte)
	return nil
}

func AssignUidsOverNetwork(ctx context.Context, num *protos.Num) (*protos.AssignedIds, error) {
	if Config.InMemoryComm {
		return allocator.AssignUids(ctx, num)
	}
	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	conn := pl.Get()
	c := protos.NewZeroClient(conn)
	return c.AssignUids(ctx, num)
}

// proposeOrSend either proposes the mutation if the node serves the group gid or sends it to
// the leader of the group gid for proposing.
func proposeOrSend(ctx context.Context, gid uint32, m *protos.Mutations, chr chan res) {
	res := res{}
	if groups().ServesGroup(gid) {
		node := groups().Node
		// we don't timeout after proposing
		txn := &posting.Txn{
			StartTs:       m.StartTs,
			PrimaryAttr:   m.PrimaryAttr,
			ServesPrimary: groups().ServesTablet(m.PrimaryAttr),
		}
		res.err = node.ProposeAndWait(ctx, &protos.Proposal{Mutations: m}, txn)
		res.ctx = &protos.TxnContext{}
		txn.Fill(res.ctx)
		chr <- res
		return
	}

	pl := groups().Leader(gid)
	if pl == nil {
		res.err = conn.ErrNoConnection
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(res.err.Error())
		}
		chr <- res
		return
	}
	conn := pl.Get()

	var tc *protos.TxnContext
	c := protos.NewWorkerClient(conn)

	ch := make(chan error, 1)
	go func() {
		var err error
		tc, err = c.Mutate(ctx, m)
		ch <- err
	}()

	select {
	case <-ctx.Done():
		res.err = ctx.Err()
		res.ctx = nil
	case err := <-ch:
		res.err = err
		res.ctx = tc
	}
	chr <- res
}

func setPrimary(src *protos.Mutations, edge *protos.DirectedEdge) {
	if len(src.PrimaryAttr) == 0 {
		return
	}
	if groups().ServesTablet(edge.Attr) {
		src.PrimaryAttr = edge.Attr
	}
}

// addToMutationArray adds the edges to the appropriate index in the mutationArray,
// taking into account the op(operation) and the attribute.
func addToMutationMap(mutationMap map[uint32]*protos.Mutations, src *protos.Mutations) error {
	for _, edge := range src.Edges {
		gid := groups().BelongsTo(edge.Attr)
		mu := mutationMap[gid]
		if mu == nil {
			mu = &protos.Mutations{GroupId: gid}
			mutationMap[gid] = mu
		}
		mu.Edges = append(mu.Edges, edge)
		setPrimary(src, edge)
	}
	if len(src.Edges) > 0 && len(src.PrimaryAttr) == 0 {
		src.PrimaryAttr = src.Edges[0].Attr
	}
	for _, schema := range src.Schema {
		gid := groups().BelongsTo(schema.Predicate)
		mu := mutationMap[gid]
		if mu == nil {
			mu = &protos.Mutations{GroupId: gid}
			mutationMap[gid] = mu
		}
		mu.Schema = append(mu.Schema, schema)
	}

	if src.DropAll {
		for _, gid := range groups().KnownGroups() {
			mu := mutationMap[gid]
			if mu == nil {
				mu = &protos.Mutations{GroupId: gid}
				mutationMap[gid] = mu
			}
			mu.DropAll = true
		}
	}
	return nil
}

type res struct {
	err error
	ctx *protos.TxnContext
}

func CommitOverNetwork(ctx context.Context, txn *protos.TxnContext) (*protos.Payload, error) {
	// TODO: Propose and do this over network.
	// TODO: Do this first over primary, then others.
	if txn.CommitTs == 0 {
		return &protos.Payload{}, abortMutations(ctx, txn)
	} else {
		return &protos.Payload{}, commitMutations(ctx, txn)
	}
}

// MutateOverNetwork checks which group should be running the mutations
// according to the group config and sends it to that instance.
func MutateOverNetwork(ctx context.Context, m *protos.Mutations) (*protos.TxnContext, error) {
	tctx := &protos.TxnContext{StartTs: m.StartTs}
	mutationMap := make(map[uint32]*protos.Mutations)
	if err := addToMutationMap(mutationMap, m); err != nil {
		return tctx, err
	}
	tctx.Primary = m.PrimaryAttr

	resCh := make(chan res, len(mutationMap))
	for gid, mu := range mutationMap {
		if gid == 0 {
			return tctx, errUnservedTablet
		}
		mu.StartTs = m.StartTs
		mu.PrimaryAttr = m.PrimaryAttr
		go proposeOrSend(ctx, gid, mu, resCh)
	}

	// Wait for all the goroutines to reply back.
	// We return if an error was returned or the parent called ctx.Done()
	var e error
	for i := 0; i < len(mutationMap); i++ {
		res := <-resCh
		if res.err != nil {
			e = res.err
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while running all mutations: %+v", res.err)
			}
		}
		tctx.Keys = append(tctx.Keys, res.ctx.Keys...)
	}
	close(resCh)
	return tctx, e
}

func (w *grpcWorker) CommitOrAbort(ctx context.Context, tc *protos.TxnContext) (*protos.Payload, error) {
	if tc.CommitTs == 0 {
		err := abortMutations(ctx, tc)
		return &protos.Payload{}, err
	}
	err := commitMutations(ctx, tc)
	return &protos.Payload{}, err
}

// Mutate is used to apply mutations over the network on other instances.
func (w *grpcWorker) Mutate(ctx context.Context, m *protos.Mutations) (*protos.TxnContext, error) {
	txnCtx := &protos.TxnContext{}
	if ctx.Err() != nil {
		return txnCtx, ctx.Err()
	}
	if !groups().ServesGroup(m.GroupId) {
		return txnCtx, x.Errorf("This server doesn't serve group id: %v", m.GroupId)
	}

	node := groups().Node
	if rand.Float64() < Config.Tracing {
		var tr trace.Trace
		tr, ctx = x.NewTrace("GrpcMutate", ctx)
		defer tr.Finish()
	}

	txn := &posting.Txn{
		StartTs:       m.StartTs,
		PrimaryAttr:   m.PrimaryAttr,
		ServesPrimary: groups().ServesTablet(m.PrimaryAttr),
	}
	err := node.ProposeAndWait(ctx, &protos.Proposal{Mutations: m}, txn)
	txn.Fill(txnCtx)
	return txnCtx, err
}
