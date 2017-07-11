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
	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos"
	"github.com/dgraph-io/dgraph/schema"
	"github.com/dgraph-io/dgraph/types"
	"github.com/dgraph-io/dgraph/x"
)

const (
	set = "set"
	del = "delete"
)

// runMutations goes through all the edges and applies them. It returns the
// mutations which were not applied in left.
func runMutations(ctx context.Context, edges []*protos.DirectedEdge) error {
	if tr, ok := trace.FromContext(ctx); ok {
		tr.LazyPrintf("In run mutations")
	}
	for _, edge := range edges {
		gid := group.BelongsTo(edge.Attr)
		if !groups().ServesGroup(gid) {
			return x.Errorf("Predicate fingerprint doesn't match this instance")
		}

		rv := ctx.Value("raft").(x.RaftValue)
		x.AssertTruef(rv.Group == gid, "fingerprint mismatch between raft and group conf")

		typ, err := schema.State().TypeOf(edge.Attr)
		x.Checkf(err, "Schema is not present for predicate %s", edge.Attr)

		if edge.Entity == 0 && bytes.Equal(edge.Value, []byte(x.Star)) {
			if err = posting.DeletePredicate(ctx, edge.Attr); err != nil {
				return err
			}
			continue
		}
		// Once mutation comes via raft we do best effort conversion
		// Type check is done before proposing mutation, in case schema is not
		// present, some invalid entries might be written initially
		err = validateAndConvert(edge, typ)

		key := x.DataKey(edge.Attr, edge.Entity)

		t := time.Now()
		plist, decr := posting.GetOrCreate(key, gid)
		if dur := time.Since(t); dur > time.Millisecond {
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("GetOrCreate took %v", dur)
			}
		}

		if err = plist.AddMutationWithIndex(ctx, edge); err != nil {
			x.Printf("Error while adding mutation: %v %v", edge, err)
			decr()
			return err // abort applying the rest of them.
		}
		decr()
	}
	return nil
}

// This is serialized with mutations, called after applied watermarks catch up
// and further mutations are blocked until this is done.
func runSchemaMutations(ctx context.Context, updates []*protos.SchemaUpdate) error {
	rv := ctx.Value("raft").(x.RaftValue)
	n := groups().Node(rv.Group)
	for _, update := range updates {
		if !groups().ServesGroup(group.BelongsTo(update.Predicate)) {
			return x.Errorf("Predicate fingerprint doesn't match this instance")
		}
		if err := checkSchema(update); err != nil {
			return err
		}
		old, ok := schema.State().Get(update.Predicate)
		current := schema.From(update)
		updateSchema(update.Predicate, current, rv.Index, rv.Group)

		// Once we remove index or reverse edges from schema, even though the values
		// are present in db, they won't be used due to validation in work/task.go
		// Removal can be done in background if we write a scheduler later which ensures
		// that schema mutations are serialized, so that there won't be
		// race conditions between deletion of edges and addition of edges.

		// We don't want to use sync watermarks for background removal, because it would block
		// linearizable read requests. Only downside would be on system crash, stale edges
		// might remain, which is ok.

		// Indexing can't be done in background as it can cause race conditons with new
		// index mutations (old set and new del)
		// We need watermark for index/reverse edge addition for linearizable reads.
		// (both applied and synced watermarks).
		if !ok {
			if current.Directive == protos.SchemaUpdate_INDEX {
				if err := n.rebuildOrDelIndex(ctx, update.Predicate, true); err != nil {
					return err
				}
			} else if current.Directive == protos.SchemaUpdate_REVERSE {
				if err := n.rebuildOrDelRevEdge(ctx, update.Predicate, true); err != nil {
					return err
				}
			}

			if current.Count {
				if err := n.rebuildOrDelCountIndex(ctx, update.Predicate, true); err != nil {
					return err
				}
			}
			continue
		}
		// schema was present already
		if needReindexing(old, current) {
			// Reindex if update.Index is true or remove index
			if err := n.rebuildOrDelIndex(ctx, update.Predicate,
				current.Directive == protos.SchemaUpdate_INDEX); err != nil {
				return err
			}
		} else if needsRebuildingReverses(old, current) {
			// Add or remove reverse edge based on update.Reverse
			if err := n.rebuildOrDelRevEdge(ctx, update.Predicate,
				current.Directive == protos.SchemaUpdate_REVERSE); err != nil {
				return err
			}
		}

		if current.Count != old.Count {
			if err := n.rebuildOrDelCountIndex(ctx, update.Predicate, current.Count); err != nil {
			}
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
		Water:  posting.SyncMarkFor(group),
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

func numEdges(attr string) int {
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.FetchValues = false
	it := pstore.NewIterator(iterOpt)
	defer it.Close()
	pk := x.ParsedKey{
		Attr: attr,
	}
	prefix := pk.DataPrefix()
	count := 0
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		count++
	}
	return count
}

func hasEdges(attr string) bool {
	iterOpt := badger.DefaultIteratorOptions
	iterOpt.FetchValues = false
	it := pstore.NewIterator(iterOpt)
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

func checkSchema(s *protos.SchemaUpdate) error {
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
			// New and old type are both scalar or both are uid. Allow schema change.
			return nil
		}
		// uid => scalar or scalar => uid. Check that there shouldn't be any data.
		if hasEdges(s.Predicate) {
			return x.Errorf("Schema change not allowed from predicate to uid or vice versa"+
				" till you have data for pred: %s", s.Predicate)
		}
	}
	return nil
}

// If storage type is specified, then check compatibility or convert to schema type
// if no storage type is specified then convert to schema type.
func validateAndConvert(edge *protos.DirectedEdge, schemaType types.TypeID) error {
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

// runMutate is used to run the mutations on an instance.
func proposeOrSend(ctx context.Context, gid uint32, m *protos.Mutations, che chan error) {
	if groups().ServesGroup(gid) {
		node := groups().Node(gid)
		// we don't timeout after proposing
		che <- node.ProposeAndWait(ctx, &protos.Proposal{Mutations: m})
		return
	}

	_, addr := groups().Leader(gid)
	pl := pools().get(addr)
	conn, err := pl.Get()
	if err != nil {
		if tr, ok := trace.FromContext(ctx); ok {
			tr.LazyPrintf(err.Error())
		}
		che <- err
		return
	}
	defer pl.Put(conn)

	c := protos.NewWorkerClient(conn)
	ch := make(chan error, 1)
	go func() {
		_, err = c.Mutate(ctx, m)
		ch <- err
	}()
	select {
	case <-ctx.Done():
		che <- ctx.Err()
	case err = <-ch:
		che <- err
	}
}

// addToMutationArray adds the edges to the appropriate index in the mutationArray,
// taking into account the op(operation) and the attribute.
func addToMutationMap(mutationMap map[uint32]*protos.Mutations, m *protos.Mutations) {
	for _, edge := range m.Edges {
		gid := group.BelongsTo(edge.Attr)
		mu := mutationMap[gid]
		if mu == nil {
			mu = &protos.Mutations{GroupId: gid}
			mutationMap[gid] = mu
		}
		mu.Edges = append(mu.Edges, edge)
	}
	for _, schema := range m.Schema {
		gid := group.BelongsTo(schema.Predicate)
		mu := mutationMap[gid]
		if mu == nil {
			mu = &protos.Mutations{GroupId: gid}
			mutationMap[gid] = mu
		}
		mu.Schema = append(mu.Schema, schema)
	}
}

// MutateOverNetwork checks which group should be running the mutations
// according to fingerprint of the predicate and sends it to that instance.
func MutateOverNetwork(ctx context.Context, m *protos.Mutations) error {
	mutationMap := make(map[uint32]*protos.Mutations)
	addToMutationMap(mutationMap, m)

	errors := make(chan error, len(mutationMap))
	for gid, mu := range mutationMap {
		go proposeOrSend(ctx, gid, mu, errors)
	}

	// Wait for all the goroutines to reply back.
	// We return if an error was returned or the parent called ctx.Done()
	var e error
	for i := 0; i < len(mutationMap); i++ {
		err := <-errors
		if err != nil {
			e = err
			if tr, ok := trace.FromContext(ctx); ok {
				tr.LazyPrintf("Error while running all mutations: %+v", err)
			}
		}
	}
	close(errors)

	return e
}

// Mutate is used to apply mutations over the network on other instances.
func (w *grpcWorker) Mutate(ctx context.Context, m *protos.Mutations) (*protos.Payload, error) {
	if ctx.Err() != nil {
		return &protos.Payload{}, ctx.Err()
	}

	if !groups().ServesGroup(m.GroupId) {
		return &protos.Payload{}, x.Errorf("This server doesn't serve group id: %v", m.GroupId)
	}
	node := groups().Node(m.GroupId)
	var tr trace.Trace
	if rand.Float64() < Config.Tracing {
		tr = trace.New("Dgraph", "GrpcMutate")
		defer tr.Finish()
		tr.SetMaxEvents(1000)
		ctx = trace.NewContext(ctx, tr)
	}
	err := node.ProposeAndWait(ctx, &protos.Proposal{Mutations: m})
	return &protos.Payload{}, err
}
