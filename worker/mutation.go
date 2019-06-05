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
	"math"
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
	"github.com/pkg/errors"
	otrace "go.opencensus.io/trace"
	"golang.org/x/net/context"
)

var (
	ErrNonExistentTabletMessage = "Requested predicate is not being served by any tablet"
	errNonExistentTablet        = errors.Errorf(ErrNonExistentTabletMessage)
	errUnservedTablet           = errors.Errorf("Tablet isn't being served by this instance")
)

func isStarAll(v []byte) bool {
	return bytes.Equal(v, []byte(x.Star))
}

func isDeletePredicateEdge(edge *pb.DirectedEdge) bool {
	return edge.Entity == 0 && isStarAll(edge.Value)
}

// runMutation goes through all the edges and applies them.
func runMutation(ctx context.Context, edge *pb.DirectedEdge, txn *posting.Txn) error {
	// We shouldn't check whether this Alpha serves this predicate or not. Membership information
	// isn't consistent across the entire cluster. We should just apply whatever is given to us.

	su, ok := schema.State().Get(edge.Attr)
	if edge.Op == pb.DirectedEdge_SET {
		if !ok {
			return errors.Errorf("runMutation: Unable to find schema for %s", edge.Attr)
		}
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
	plist, err := txn.Get(key)
	if dur := time.Since(t); dur > time.Millisecond {
		if span := otrace.FromContext(ctx); span != nil {
			span.Annotatef([]otrace.Attribute{otrace.BoolAttribute("slow-get", true)},
				"GetLru took %s", dur)
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
		// on error, we restore the memory state to be the same as the disk
		maxRetries := 10
		loadErr := x.RetryUntilSuccess(maxRetries, 10*time.Millisecond, func() error {
			return schema.Load(update.Predicate)
		})

		if loadErr != nil {
			glog.Fatalf("failed to load schema after %d retries: %v", maxRetries, loadErr)
		}
		return err
	}

	return updateSchema(update.Predicate, *update)
}

func runSchemaMutationHelper(ctx context.Context, update *pb.SchemaUpdate, startTs uint64) error {
	if tablet, err := groups().Tablet(update.Predicate); err != nil {
		return err
	} else if tablet.GetGroupId() != groups().groupId() {
		return errors.Errorf("Tablet isn't being served by this group. Tablet: %+v", tablet)
	}

	if err := checkSchema(update); err != nil {
		return err
	}
	old, _ := schema.State().Get(update.Predicate)
	current := *update
	// Sets only in memory, we will update it on disk only after schema mutations
	// are successful and  written to disk.
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
	rebuild := posting.IndexRebuild{
		Attr:          update.Predicate,
		StartTs:       startTs,
		OldSchema:     &old,
		CurrentSchema: &current,
	}
	return rebuild.Run(ctx)
}

// updateSchema commits the schema to disk in blocking way, should be ok because this happens
// only during schema mutations or we see a new predicate.
func updateSchema(attr string, s pb.SchemaUpdate) error {
	schema.State().Set(attr, s)
	txn := pstore.NewTransactionAt(1, true)
	defer txn.Discard()
	data, err := s.Marshal()
	x.Check(err)
	err = txn.SetEntry(&badger.Entry{
		Key:      x.SchemaKey(attr),
		Value:    data,
		UserMeta: posting.BitSchemaPosting,
	})
	if err != nil {
		return err
	}
	return txn.CommitAt(1, nil)
}

func createSchema(attr string, typ types.TypeID) {
	// Don't overwrite schema blindly, acl's might have been set even though
	// type is not present
	s, ok := schema.State().Get(attr)
	if ok {
		s.ValueType = typ.Enum()
	} else {
		s = pb.SchemaUpdate{ValueType: typ.Enum(), Predicate: attr}
		// For type UidID, set List to true. This is done because previously
		// all predicates of type UidID were implicitly considered lists.
		if typ == types.UidID {
			s.List = true
		}
	}
	updateSchema(attr, s)
}

func runTypeMutation(ctx context.Context, update *pb.TypeUpdate) error {
	if err := checkType(update); err != nil {
		return err
	}
	current := *update

	schema.State().SetType(update.TypeName, current)
	return updateType(update.TypeName, *update)
}

// We commit schema to disk in blocking way, should be ok because this happens
// only during schema mutations or we see a new predicate.
func updateType(typeName string, t pb.TypeUpdate) error {
	schema.State().SetType(typeName, t)
	txn := pstore.NewTransactionAt(1, true)
	defer txn.Discard()
	data, err := t.Marshal()
	x.Check(err)
	err = txn.SetEntry(&badger.Entry{
		Key:      x.TypeKey(typeName),
		Value:    data,
		UserMeta: posting.BitSchemaPosting,
	})
	if err != nil {
		return err
	}
	return txn.CommitAt(1, nil)
}

func hasEdges(attr string, startTs uint64) bool {
	pk := x.ParsedKey{Attr: attr}
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.PrefetchValues = false
	iterOpt.Prefix = pk.DataPrefix()

	txn := pstore.NewTransactionAt(startTs, false)
	defer txn.Discard()

	it := txn.NewIterator(iterOpt)
	defer it.Close()

	for it.Rewind(); it.Valid(); it.Next() {
		// NOTE: This is NOT correct.
		// An incorrect, but efficient way to quickly check if we have at least one non-empty
		// posting. This does NOT consider those posting lists which can have multiple deltas
		// summing up to an empty posting list. I'm leaving it as it is for now. But, this could
		// cause issues because of this inaccuracy.
		if it.Item().UserMeta()&posting.BitEmptyPosting == 0 {
			return true
		}
	}
	return false
}
func checkSchema(s *pb.SchemaUpdate) error {
	if len(s.Predicate) == 0 {
		return errors.Errorf("No predicate specified in schema mutation")
	}

	if s.Directive == pb.SchemaUpdate_INDEX && len(s.Tokenizer) == 0 {
		return errors.Errorf("Tokenizer must be specified while indexing a predicate: %+v", s)
	}

	if len(s.Tokenizer) > 0 && s.Directive != pb.SchemaUpdate_INDEX {
		return errors.Errorf("Directive must be SchemaUpdate_INDEX when a tokenizer is specified")
	}

	typ := types.TypeID(s.ValueType)
	if typ == types.UidID && s.Directive == pb.SchemaUpdate_INDEX {
		// index on uid type
		return errors.Errorf("Index not allowed on predicate of type uid on predicate %s",
			s.Predicate)
	} else if typ != types.UidID && s.Directive == pb.SchemaUpdate_REVERSE {
		// reverse on non-uid type
		return errors.Errorf("Cannot reverse for non-uid type on predicate %s", s.Predicate)
	}

	// If schema update has upsert directive, it should have index directive.
	if s.Upsert && len(s.Tokenizer) == 0 {
		return errors.Errorf("Index tokenizer is mandatory for: [%s] when specifying @upsert directive",
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
			return errors.Errorf("Schema change not allowed from %s to %s",
				t.Enum(), typ.Enum())
		}

	case t.IsScalar() == typ.IsScalar():
		// If old type was list and new type is non-list, we don't allow it until user
		// has data.
		if schema.State().IsList(s.Predicate) && !s.List && hasEdges(s.Predicate, math.MaxUint64) {
			return errors.Errorf("Schema change not allowed from [%s] => %s without"+
				" deleting pred: %s", t.Name(), typ.Name(), s.Predicate)
		}

	default:
		// uid => scalar or scalar => uid. Check that there shouldn't be any data.
		if hasEdges(s.Predicate, math.MaxUint64) {
			return errors.Errorf("Schema change not allowed from scalar to uid or vice versa"+
				" while there is data for pred: %s", s.Predicate)
		}
	}
	return nil
}

func checkType(t *pb.TypeUpdate) error {
	if len(t.TypeName) == 0 {
		return errors.Errorf("Type name must be specified in type update")
	}

	for _, field := range t.Fields {
		if len(field.Predicate) == 0 {
			return errors.Errorf("Field in type definition must have a name")
		}

		if field.ValueType == pb.Posting_OBJECT && len(field.ObjectTypeName) == 0 {
			return errors.Errorf("Field with value type OBJECT must specify the name of the object type")
		}

		if field.Directive != pb.SchemaUpdate_NONE {
			return errors.Errorf("Field in type definition cannot have a directive")
		}

		if len(field.Tokenizer) > 0 {
			return errors.Errorf("Field in type definition cannot have tokenizers")
		}
	}

	return nil
}

// ValidateAndConvert checks compatibility or converts to the schema type if the storage type is
// specified. If no storage type is specified then it converts to the schema type.
func ValidateAndConvert(edge *pb.DirectedEdge, su *pb.SchemaUpdate) error {
	if isDeletePredicateEdge(edge) {
		return nil
	}
	if types.TypeID(edge.ValueType) == types.DefaultID && isStarAll(edge.Value) {
		return nil
	}

	storageType := posting.TypeID(edge)
	schemaType := types.TypeID(su.ValueType)

	// type checks
	switch {
	case edge.Lang != "" && !su.GetLang():
		return errors.Errorf("Attr: [%v] should have @lang directive in schema to mutate edge: [%v]",
			edge.Attr, edge)

	case !schemaType.IsScalar() && !storageType.IsScalar():
		return nil

	case !schemaType.IsScalar() && storageType.IsScalar():
		return errors.Errorf("Input for predicate %s of type uid is scalar", edge.Attr)

	case schemaType.IsScalar() && !storageType.IsScalar():
		return errors.Errorf("Input for predicate %s of type scalar is uid. Edge: %v", edge.Attr, edge)

	// The suggested storage type matches the schema, OK!
	case storageType == schemaType && schemaType != types.DefaultID:
		return nil

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
	pl := groups().connToZeroLeader()
	if pl == nil {
		return nil, conn.ErrNoConnection
	}

	conn := pl.Get()
	c := pb.NewZeroClient(conn)
	return c.Timestamps(ctx, num)
}

func fillTxnContext(tctx *api.TxnContext, startTs uint64) {
	if txn := posting.Oracle().GetTxn(startTs); txn != nil {
		txn.Fill(tctx, groups().groupId())
	}
	// We do not need to fill linread mechanism anymore, because transaction
	// start ts is sufficient to wait for, to achieve lin reads.
}

// proposeOrSend either proposes the mutation if the node serves the group gid or sends it to
// the leader of the group gid for proposing.
func proposeOrSend(ctx context.Context, gid uint32, m *pb.Mutations, chr chan res) {
	res := res{}
	if groups().ServesGroup(gid) {
		res.ctx = &api.TxnContext{}
		res.err = (&grpcWorker{}).proposeAndWait(ctx, res.ctx, m)
		chr <- res
		return
	}

	pl := groups().Leader(gid)
	if pl == nil {
		res.err = conn.ErrNoConnection
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
func populateMutationMap(src *pb.Mutations) (map[uint32]*pb.Mutations, error) {
	mm := make(map[uint32]*pb.Mutations)
	for _, edge := range src.Edges {
		gid, err := groups().BelongsTo(edge.Attr)
		if err != nil {
			return nil, err
		}

		mu := mm[gid]
		if mu == nil {
			mu = &pb.Mutations{GroupId: gid}
			mm[gid] = mu
		}
		mu.Edges = append(mu.Edges, edge)
	}

	for _, schema := range src.Schema {
		gid, err := groups().BelongsTo(schema.Predicate)
		if err != nil {
			return nil, err
		}

		mu := mm[gid]
		if mu == nil {
			mu = &pb.Mutations{GroupId: gid}
			mm[gid] = mu
		}
		mu.Schema = append(mu.Schema, schema)
	}

	if src.DropOp > 0 {
		for _, gid := range groups().KnownGroups() {
			mu := mm[gid]
			if mu == nil {
				mu = &pb.Mutations{GroupId: gid}
				mm[gid] = mu
			}
			mu.DropOp = src.DropOp
			mu.DropValue = src.DropValue
		}
	}

	// Type definitions are sent to all groups.
	if len(src.Types) > 0 {
		for _, gid := range groups().KnownGroups() {
			mu := mm[gid]
			if mu == nil {
				mu = &pb.Mutations{GroupId: gid}
				mm[gid] = mu
			}
			mu.Types = src.Types
		}
	}

	return mm, nil
}

type res struct {
	err error
	ctx *api.TxnContext
}

// MutateOverNetwork checks which group should be running the mutations
// according to the group config and sends it to that instance.
func MutateOverNetwork(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error) {
	ctx, span := otrace.StartSpan(ctx, "worker.MutateOverNetwork")
	defer span.End()

	tctx := &api.TxnContext{StartTs: m.StartTs}
	mutationMap, err := populateMutationMap(m)
	if err != nil {
		return tctx, err
	}

	resCh := make(chan res, len(mutationMap))
	for gid, mu := range mutationMap {
		if gid == 0 {
			span.Annotatef(nil, "state: %+v", groups().state)
			span.Annotatef(nil, "Group id zero for mutation: %+v", mu)
			return tctx, errNonExistentTablet
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
		}
		if res.ctx != nil {
			tctx.Keys = append(tctx.Keys, res.ctx.Keys...)
			tctx.Preds = append(tctx.Preds, res.ctx.Preds...)
		}
	}
	close(resCh)
	return tctx, e
}

// CommitOverNetwork makes a proxy call to Zero to commit or abort a transaction.
func CommitOverNetwork(ctx context.Context, tc *api.TxnContext) (uint64, error) {
	ctx, span := otrace.StartSpan(ctx, "worker.CommitOverNetwork")
	defer span.End()

	pl := groups().Leader(0)
	if pl == nil {
		return 0, conn.ErrNoConnection
	}
	zc := pb.NewZeroClient(pl.Get())
	tctx, err := zc.CommitOrAbort(ctx, tc)

	if err != nil {
		span.Annotatef(nil, "Error=%v", err)
		return 0, err
	}
	var attributes []otrace.Attribute
	attributes = append(attributes, otrace.Int64Attribute("commitTs", int64(tctx.CommitTs)))
	attributes = append(attributes, otrace.BoolAttribute("committed", tctx.CommitTs > 0))
	span.Annotate(attributes, "")

	if tctx.Aborted || tctx.CommitTs == 0 {
		return 0, y.ErrAborted
	}
	return tctx.CommitTs, nil
}

func (w *grpcWorker) proposeAndWait(ctx context.Context, txnCtx *api.TxnContext,
	m *pb.Mutations) error {
	if x.WorkerConfig.StrictMutations {
		for _, edge := range m.Edges {
			if _, err := schema.State().TypeOf(edge.Attr); err != nil {
				return err
			}
		}
	}

	node := groups().Node
	err := node.proposeAndWait(ctx, &pb.Proposal{Mutations: m})
	fillTxnContext(txnCtx, m.StartTs)
	return err
}

// Mutate is used to apply mutations over the network on other instances.
func (w *grpcWorker) Mutate(ctx context.Context, m *pb.Mutations) (*api.TxnContext, error) {
	ctx, span := otrace.StartSpan(ctx, "worker.Mutate")
	defer span.End()

	txnCtx := &api.TxnContext{}
	if ctx.Err() != nil {
		return txnCtx, ctx.Err()
	}
	if !groups().ServesGroup(m.GroupId) {
		return txnCtx, errors.Errorf("This server doesn't serve group id: %v", m.GroupId)
	}

	return txnCtx, w.proposeAndWait(ctx, txnCtx, m)
}

func tryAbortTransactions(startTimestamps []uint64) {
	// Aborts if not already committed.
	req := &pb.TxnTimestamps{Ts: startTimestamps}

	err := groups().Node.blockingAbort(req)
	glog.Infof("tryAbortTransactions for %d txns. Error: %+v\n", len(req.Ts), err)
}
