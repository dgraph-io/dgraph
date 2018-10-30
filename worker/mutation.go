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

package worker

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/dgo/protos/api"
	"github.com/dgraph-io/dgo/y"
	"github.com/dgraph-io/dgraph/conn"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/pb"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"golang.org/x/net/trace"
)

var (
	errUnservedTablet  = x.Errorf("Tablet isn't being served by this instance.")
	errPredicateMoving = x.Errorf("Predicate is being moved, please retry later")
)

func isStarAll(v []byte) bool {
	return bytes.Equal(v, []byte(x.Star))
}

func isDeletePredicateEdge(edge *pb.DirectedEdge) bool {
	return edge.Entity == 0 && isStarAll(edge.Value)
}

// runMutation goes through all the edges and applies them.
func runMutation(ctx context.Context, edge *pb.DirectedEdge, txn *posting.Txn) error {
	if !groups().ServesTabletRW(edge.Attr) {
		// Don't assert, can happen during replay of raft logs if server crashes immediately
		// after predicate move and before snapshot.
		return errUnservedTablet
	}

	su, ok := schema.State().Get(edge.Attr)
	if edge.Op == pb.DirectedEdge_SET {
		x.AssertTruef(ok, "Schema is not present for predicate %s", edge.Attr)
	}

	if isDeletePredicateEdge(edge) {
		return errors.New("We should never reach here")
	}
	// Once mutation comes via raft we do best effort conversion
	// Type check is done before proposing mutation, in case schema is not
	// present, some invalid entries might be written initially
	if err := ValidateAndConvert(edge, &su); err != nil {
		return err
	}

	t := time.Now()
	key := x.DataKey(edge.Attr, edge.Entity)
	plist, err := posting.Get(key)
	if dur := time.Since(t); dur > time.Millisecond {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf("GetLru took %v", dur)
		}
	}
	if err != nil {
		return err
	}

	if err = plist.AddMutationWithIndex(ctx, edge, txn); err != nil {
		return err // abort applying the rest of them.
	}
	return nil
}

// This is serialized with mutations, called after applied watermarks catch up
// and further mutations are blocked until this is done.
func runSchemaMutation(ctx context.Context, update *pb.SchemaUpdate, startTs uint64) error {
	if err := runSchemaMutationHelper(ctx, update, startTs); err != nil {
		return err
	}

	return updateSchema(update.Predicate, *update)
}

func runSchemaMutationHelper(ctx context.Context, update *pb.SchemaUpdate, startTs uint64) error {
	n := groups().Node
	if !groups().ServesTablet(update.Predicate) {
		return errUnservedTablet
	}
	if err := checkSchema(update); err != nil {
		return err
	}
	old, ok := schema.State().Get(update.Predicate)
	current := *update
	// Sets only in memory, we will update it on disk only after schema mutations is successful and persisted
	// to disk.
	schema.State().Set(update.Predicate, current)

	// Once we remove index or reverse edges from schema, even though the values
	// are present in db, they won't be used due to validation in work/task.go

	// We don't want to use sync watermarks for background removal, because it would block
	// linearizable read requests. Only downside would be on system crash, stale edges
	// might remain, which is ok.

	// Indexing can't be done in background as it can cause race conditons with new
	// index mutations (old set and new del)
	// We need watermark for index/reverse edge addition for linearizable reads.
	// (both applied and synced watermarks).
	defer glog.Infof("Done schema update %+v\n", update)
	if !ok {
		if current.Directive == pb.SchemaUpdate_INDEX {
			if err := n.rebuildOrDelIndex(ctx, update.Predicate, true, startTs); err != nil {
				return err
			}
		} else if current.Directive == pb.SchemaUpdate_REVERSE {
			if err := n.rebuildOrDelRevEdge(ctx, update.Predicate, true, startTs); err != nil {
				return err
			}
		}

		if current.Count {
			if err := n.rebuildOrDelCountIndex(ctx, update.Predicate, true, startTs); err != nil {
				return err
			}
		}
		return nil
	}

	// schema was present already
	if current.List && !old.List {
		if err := posting.RebuildListType(ctx, update.Predicate, startTs); err != nil {
			return err
		}
	} else if old.List && !current.List {
		return fmt.Errorf("Type can't be changed from list to scalar for attr: [%s]"+
			" without dropping it first.", current.Predicate)
	}

	if needReindexing(old, current) {
		// Reindex if update.Index is true or remove index
		if err := n.rebuildOrDelIndex(ctx, update.Predicate,
			current.Directive == pb.SchemaUpdate_INDEX, startTs); err != nil {
			return err
		}
	} else if needsRebuildingReverses(old, current) {
		// Add or remove reverse edge based on update.Reverse
		if err := n.rebuildOrDelRevEdge(ctx, update.Predicate,
			current.Directive == pb.SchemaUpdate_REVERSE, startTs); err != nil {
			return err
		}
	}

	if current.Count != old.Count {
		if err := n.rebuildOrDelCountIndex(ctx, update.Predicate, current.Count,
			startTs); err != nil {
			return err
		}
	}
	return nil
}

func needsRebuildingReverses(old pb.SchemaUpdate, current pb.SchemaUpdate) bool {
	return (current.Directive == pb.SchemaUpdate_REVERSE) !=
		(old.Directive == pb.SchemaUpdate_REVERSE)
}

func needReindexing(old pb.SchemaUpdate, current pb.SchemaUpdate) bool {
	if (current.Directive == pb.SchemaUpdate_INDEX) != (old.Directive == pb.SchemaUpdate_INDEX) {
		return true
	}
	// if value types has changed
	if current.Directive == pb.SchemaUpdate_INDEX && current.ValueType != old.ValueType {
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

// We commit schema to disk in blocking way, should be ok because this happens
// only during schema mutations or we see a new predicate.
func updateSchema(attr string, s pb.SchemaUpdate) error {
	schema.State().Set(attr, s)
	txn := pstore.NewTransactionAt(1, true)
	defer txn.Discard()
	data, err := s.Marshal()
	x.Check(err)
	if err := txn.Set(x.SchemaKey(attr), data); err != nil {
		return err
	}
	return txn.CommitAt(1, nil)
}

func updateSchemaType(attr string, typ types.TypeID, index uint64) {
	// Don't overwrite schema blindly, acl's might have been set even though
	// type is not present
	s, ok := schema.State().Get(attr)
	if ok {
		s.ValueType = typ.Enum()
	} else {
		s = pb.SchemaUpdate{ValueType: typ.Enum(), Predicate: attr}
	}
	updateSchema(attr, s)
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
		// Check for non-empty posting
		// BitEmptyPosting is also a complete posting,
		// so checking for CompletePosting&BitCompletePosting > 0 would
		// be wrong
		if it.Item().UserMeta()&posting.BitEmptyPosting != posting.BitEmptyPosting {
			return true
		}
	}
	return false
}

func checkSchema(s *pb.SchemaUpdate) error {
	if len(s.Predicate) == 0 {
		return x.Errorf("No predicate specified in schema mutation")
	}

	if s.Directive == pb.SchemaUpdate_INDEX && len(s.Tokenizer) == 0 {
		return x.Errorf("Tokenizer must be specified while indexing a predicate: %+v", s)
	}

	if len(s.Tokenizer) > 0 && s.Directive != pb.SchemaUpdate_INDEX {
		return x.Errorf("Directive must be SchemaUpdate_INDEX when a tokenizer is specified")
	}

	typ := types.TypeID(s.ValueType)
	if typ == types.UidID && s.Directive == pb.SchemaUpdate_INDEX {
		// index on uid type
		return x.Errorf("Index not allowed on predicate of type uid on predicate %s",
			s.Predicate)
	} else if typ != types.UidID && s.Directive == pb.SchemaUpdate_REVERSE {
		// reverse on non-uid type
		return x.Errorf("Cannot reverse for non-uid type on predicate %s", s.Predicate)
	}

	// If schema update has upsert directive, it should have index directive.
	if s.Upsert && len(s.Tokenizer) == 0 {
		return x.Errorf("Index tokenizer is mandatory for: [%s] when specifying @upsert directive",
			s.Predicate)
	}

	t, err := schema.State().TypeOf(s.Predicate)
	if err != nil {
		// No schema previously defined, so no need to do checks about schema conversions.
		return nil
	}

	// schema was defined already
	switch {
	case t.IsScalar() && (t.Enum() == pb.Posting_PASSWORD || s.ValueType == pb.Posting_PASSWORD):
		// can't change password -> x, x -> password
		if t.Enum() != s.ValueType {
			return x.Errorf("Schema change not allowed from %s to %s",
				t.Enum().String(), typ.Enum().String())
		}

	case t.IsScalar() == typ.IsScalar():
		// If old type was list and new type is non-list, we don't allow it until user
		// has data.
		if schema.State().IsList(s.Predicate) && !s.List && hasEdges(s.Predicate, math.MaxUint64) {
			return x.Errorf("Schema change not allowed from [%s] => %s without"+
				" deleting pred: %s", t.Name(), typ.Name(), s.Predicate)
		}

	default:
		// uid => scalar or scalar => uid. Check that there shouldn't be any data.
		if hasEdges(s.Predicate, math.MaxUint64) {
			return x.Errorf("Schema change not allowed from scalar to uid or vice versa"+
				" while there is data for pred: %s", s.Predicate)
		}
	}
	return nil
}

// If storage type is specified, then check compatibility or convert to schema type
// if no storage type is specified then convert to schema type.
func ValidateAndConvert(edge *pb.DirectedEdge, su *pb.SchemaUpdate) error {
	if isDeletePredicateEdge(edge) {
		return nil
	}
	if types.TypeID(edge.ValueType) == types.DefaultID && isStarAll(edge.Value) {
		return nil
	}
	// <s> <p> <o> Del on non list scalar type.
	if edge.ValueId == 0 && !isStarAll(edge.Value) && edge.Op == pb.DirectedEdge_DEL && !su.GetList() {
		return x.Errorf("Please use * with delete operation for non-list type: [%v]", edge.Attr)
	}

	// type checks
	storageType, schemaType := posting.TypeID(edge), types.TypeID(su.ValueType)
	switch {
	case edge.Lang != "" && !su.GetLang():
		return x.Errorf("Attr: [%v] should have @lang directive in schema to mutate edge: [%v]",
			edge.Attr, edge)

	case !schemaType.IsScalar() && !storageType.IsScalar():
		return nil

	case !schemaType.IsScalar() && storageType.IsScalar():
		return x.Errorf("Input for predicate %s of type uid is scalar", edge.Attr)

	case schemaType.IsScalar() && !storageType.IsScalar():
		return x.Errorf("Input for predicate %s of type scalar is uid", edge.Attr)

	// The suggested storage type matches the schema, OK!
	case storageType == schemaType && schemaType != types.DefaultID:
		return nil

	// try to guess a type from the value
	case storageType == schemaType && schemaType == types.DefaultID:
		schemaType, _ = types.TypeForValue(edge.Value)
		if schemaType == types.DefaultID {
			return nil
		}

	// We accept the storage type iff we don't have a schema type and a storage type is specified.
	case schemaType == types.DefaultID:
		schemaType = storageType
	}

	var (
		dst types.Val
		err error
	)

	src := types.Val{Tid: types.TypeID(edge.ValueType), Value: edge.Value}
	// check compatibility of schema type and storage type
	if dst, err = types.Convert(src, schemaType); err != nil {
		return err
	}

	// convert to schema type
	b := types.ValueForType(types.BinaryID)
	if err = types.Marshal(dst, &b); err != nil {
		return err
	}
	edge.ValueType = schemaType.Enum()
	edge.Value = b.Value.([]byte)
	return nil
}

func AssignUidsOverNetwork(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	conn := pl.Get()
	c := pb.NewZeroClient(conn)
	return c.AssignUids(ctx, num)
}

func Timestamps(ctx context.Context, num *pb.Num) (*pb.AssignedIds, error) {
	pl := groups().Leader(0)
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	conn := pl.Get()
	c := pb.NewZeroClient(conn)
	return c.Timestamps(ctx, num)
}

func fillTxnContext(tctx *api.TxnContext, startTs uint64) {
	if txn := posting.Oracle().GetTxn(startTs); txn != nil {
		txn.Fill(tctx)
	}
	// We do not need to fill linread mechanism anymore, because transaction
	// start ts is sufficient to wait for, to achieve lin reads.
}

// proposeOrSend either proposes the mutation if the node serves the group gid or sends it to
// the leader of the group gid for proposing.
func proposeOrSend(ctx context.Context, gid uint32, m *pb.Mutations, chr chan res) {
	res := res{}
	if groups().ServesGroup(gid) {
		node := groups().Node
		// we don't timeout after proposing
		res.err = node.proposeAndWait(ctx, &pb.Proposal{Mutations: m})
		res.ctx = &api.TxnContext{}
		fillTxnContext(res.ctx, m.StartTs)
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

	var tc *api.TxnContext
	c := pb.NewWorkerClient(conn)

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

// populateMutationMap populates a map from group id to the mutation that
// should be sent to that group.
func populateMutationMap(src *pb.Mutations) map[uint32]*pb.Mutations {
	mm := make(map[uint32]*pb.Mutations)
	for _, edge := range src.Edges {
		gid := groups().BelongsTo(edge.Attr)
		mu := mm[gid]
		if mu == nil {
			mu = &pb.Mutations{GroupId: gid}
			mm[gid] = mu
		}
		mu.Edges = append(mu.Edges, edge)
	}
	for _, schema := range src.Schema {
		gid := groups().BelongsTo(schema.Predicate)
		mu := mm[gid]
		if mu == nil {
			mu = &pb.Mutations{GroupId: gid}
			mm[gid] = mu
		}
		mu.Schema = append(mu.Schema, schema)
	}
	if src.DropAll {
		for _, gid := range groups().KnownGroups() {
			mu := mm[gid]
			if mu == nil {
				mu = &pb.Mutations{GroupId: gid}
				mm[gid] = mu
			}
			mu.DropAll = true
		}
	}
	return mm
}

func commitOrAbort(ctx context.Context, startTs, commitTs uint64) error {
	txn := posting.Oracle().GetTxn(startTs)
	if txn == nil {
		return nil
	}
	// Ensures that we wait till prewrite is applied
	return txn.CommitToMemory(commitTs)
}

type res struct {
	err error
	ctx *api.TxnContext
}

// MutateOverNetwork checks which group should be running the mutations
// according to the group config and sends it to that instance.
func MutateOverNetwork(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error) {
	tctx := &api.TxnContext{StartTs: m.StartTs}
	mutationMap := populateMutationMap(m)

	resCh := make(chan res, len(mutationMap))
	for gid, mu := range mutationMap {
		if gid == 0 {
			return tctx, errUnservedTablet
		}
		mu.StartTs = m.StartTs
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
		if res.ctx != nil {
			tctx.Keys = append(tctx.Keys, res.ctx.Keys...)
		}
	}
	close(resCh)
	return tctx, e
}

// CommitOverNetwork makes a proxy call to Zero to commit or abort a transaction.
func CommitOverNetwork(ctx context.Context, tc *api.TxnContext) (uint64, error) {
	pl := groups().Leader(0)
	if pl == nil {
		return 0, conn.ErrNoConnection
	}
	zc := pb.NewZeroClient(pl.Get())
	tctx, err := zc.CommitOrAbort(ctx, tc)
	if err != nil {
		return 0, err
	}
	if tctx.Aborted {
		return 0, y.ErrAborted
	}
	return tctx.CommitTs, nil
}

func (w *grpcWorker) PurgeTs(ctx context.Context,
	payload *api.Payload) (*pb.Num, error) {
	n := &pb.Num{}
	n.Val = posting.Oracle().PurgeTs()
	return n, nil
}

// Mutate is used to apply mutations over the network on other instances.
func (w *grpcWorker) Mutate(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error) {
	txnCtx := &api.TxnContext{}
	if ctx.Err() != nil {
		return txnCtx, ctx.Err()
	}
	if !groups().ServesGroup(m.GroupId) {
		return txnCtx, x.Errorf("This server doesn't serve group id: %v", m.GroupId)
	}

	node := groups().Node
	if rand.Float64() < Config.Tracing {
		var tr trace.Trace
		tr, ctx = x.NewTrace("grpcWorker.Mutate", ctx)
		defer tr.Finish()
	}

	err := node.proposeAndWait(ctx, &pb.Proposal{Mutations: m})
	fillTxnContext(txnCtx, m.StartTs)
	return txnCtx, err
}

func tryAbortTransactions(startTimestamps []uint64) {
	// Aborts if not already committed.
	req := &pb.TxnTimestamps{Ts: startTimestamps}

	err := groups().Node.blockingAbort(req)
	glog.Infof("tryAbortTransactions for %d txns. Error: %+v\n", len(req.Ts), err)
}
