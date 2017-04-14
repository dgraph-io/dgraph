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
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/protos/graphp"
	"github.com/dgraph-io/dgraph/protos/taskp"
	"github.com/dgraph-io/dgraph/protos/typesp"
	"github.com/dgraph-io/dgraph/protos/workerp"
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
func runMutations(ctx context.Context, edges []*taskp.DirectedEdge) error {
	for _, edge := range edges {
		gid := group.BelongsTo(edge.Attr)
		if !groups().ServesGroup(gid) {
			return x.Errorf("Predicate fingerprint doesn't match this instance")
		}

		rv := ctx.Value("raft").(x.RaftValue)
		x.AssertTruef(rv.Group == gid, "fingerprint mismatch between raft and group conf")

		typ, err := schema.State().TypeOf(edge.Attr)
		x.Checkf(err, "Schema is not present for predicate %s", edge.Attr)

		// Once mutation comes via raft we do best effort conversion
		// Type check is done before proposing mutation, in case schema is not
		// present, some invalid entries might be written initially
		err = validateAndConvert(edge, typ)

		key := x.DataKey(edge.Attr, edge.Entity)
		plist, decr := posting.GetOrCreate(key, gid)
		defer decr()

		if err = plist.AddMutationWithIndex(ctx, edge); err != nil {
			x.Printf("Error while adding mutation: %v %v", edge, err)
			return err // abort applying the rest of them.
		}
	}
	return nil
}

// This is serialized with mutations, called after applied watermarks catch up
// and further mutations are blocked until this is done.
func runSchemaMutations(ctx context.Context, updates []*graphp.SchemaUpdate) error {
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
			if current.Directive == typesp.Schema_INDEX {
				if err := n.rebuildOrDelIndex(ctx, update.Predicate, true); err != nil {
					return err
				}
			} else if current.Directive == typesp.Schema_REVERSE {
				if err := n.rebuildOrDelRevEdge(ctx, update.Predicate, true); err != nil {
					return err
				}
			}
			continue
		}
		// schema was present already
		if needReindexing(old, current) {
			// Reindex if update.Index is true or remove index
			if err := n.rebuildOrDelIndex(ctx, update.Predicate,
				current.Directive == typesp.Schema_INDEX); err != nil {
				return err
			}
		} else if (current.Directive == typesp.Schema_REVERSE) !=
			(old.Directive == typesp.Schema_REVERSE) {
			// Add or remove reverse edge based on update.Reverse
			if err := n.rebuildOrDelRevEdge(ctx, update.Predicate,
				current.Directive == typesp.Schema_REVERSE); err != nil {
				return err
			}
		}
	}
	return nil
}

func needReindexing(old typesp.Schema, current typesp.Schema) bool {
	if (current.Directive == typesp.Schema_INDEX) != (old.Directive == typesp.Schema_INDEX) {
		return true
	}
	// if value types has changed
	if current.Directive == typesp.Schema_INDEX && current.ValueType != old.ValueType {
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

func updateSchema(attr string, s typesp.Schema, raftIndex uint64, group uint32) {
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
		s = typesp.Schema{ValueType: uint32(typ)}
	}
	updateSchema(attr, s, raftIndex, group)
}

func checkSchema(s *graphp.SchemaUpdate) error {
	typ := types.TypeID(s.ValueType)
	if typ == types.UidID && s.Directive == graphp.SchemaUpdate_INDEX {
		// index on uid type
		return x.Errorf("Index not allowed on predicate of type uid on predicate %s",
			s.Predicate)
	} else if typ != types.UidID && s.Directive == graphp.SchemaUpdate_REVERSE {
		// reverse on non-uid type
		return x.Errorf("Cannot reverse for non-uid type on predicate %s", s.Predicate)
	}
	if t, err := schema.State().TypeOf(s.Predicate); err == nil {
		// schema was defined already
		if typ.IsScalar() != t.IsScalar() {
			return x.Errorf("Schema change not allowed from predicate to uid or vice versa")
		}
	}
	return nil
}

// If storage type is specified, then check compatibility or convert to schema type
// if no storage type is specified then convert to schema type
func validateAndConvert(edge *taskp.DirectedEdge, schemaType types.TypeID) error {
	if types.TypeID(edge.ValueType) == types.DefaultID &&
		string(edge.Value) == x.DeleteAll {
		if edge.Op != taskp.DirectedEdge_DEL {
			return x.Errorf("* allowed only with delete operation")
		}
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
func proposeOrSend(ctx context.Context, gid uint32, m *taskp.Mutations, che chan error) {
	if groups().ServesGroup(gid) {
		node := groups().Node(gid)
		che <- node.ProposeAndWait(ctx, &taskp.Proposal{Mutations: m})
		return
	}

	_, addr := groups().Leader(gid)
	pl := pools().get(addr)
	conn, err := pl.Get()
	if err != nil {
		x.TraceError(ctx, err)
		che <- err
		return
	}
	defer pl.Put(conn)

	c := workerp.NewWorkerClient(conn)
	_, err = c.Mutate(ctx, m)
	che <- err
}

// addToMutationArray adds the edges to the appropriate index in the mutationArray,
// taking into account the op(operation) and the attribute.
func addToMutationMap(mutationMap map[uint32]*taskp.Mutations, m *taskp.Mutations) {
	for _, edge := range m.Edges {
		gid := group.BelongsTo(edge.Attr)
		mu := mutationMap[gid]
		if mu == nil {
			mu = &taskp.Mutations{GroupId: gid}
			mutationMap[gid] = mu
		}
		mu.Edges = append(mu.Edges, edge)
	}
	for _, schema := range m.Schema {
		gid := group.BelongsTo(schema.Predicate)
		mu := mutationMap[gid]
		if mu == nil {
			mu = &taskp.Mutations{GroupId: gid}
			mutationMap[gid] = mu
		}
		mu.Schema = append(mu.Schema, schema)
	}
}

// MutateOverNetwork checks which group should be running the mutations
// according to fingerprint of the predicate and sends it to that instance.
func MutateOverNetwork(ctx context.Context, m *taskp.Mutations) error {
	mutationMap := make(map[uint32]*taskp.Mutations)
	addToMutationMap(mutationMap, m)

	errors := make(chan error, len(mutationMap))
	for gid, mu := range mutationMap {
		proposeOrSend(ctx, gid, mu, errors)
	}

	// Wait for all the goroutines to reply back.
	// We return if an error was returned or the parent called ctx.Done()
	for i := 0; i < len(mutationMap); i++ {
		select {
		case err := <-errors:
			if err != nil {
				x.TraceError(ctx, x.Wrapf(err, "Error while running all mutations"))
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	close(errors)

	return nil
}

// Mutate is used to apply mutations over the network on other instances.
func (w *grpcWorker) Mutate(ctx context.Context, m *taskp.Mutations) (*workerp.Payload, error) {
	if ctx.Err() != nil {
		return &workerp.Payload{}, ctx.Err()
	}

	if !groups().ServesGroup(m.GroupId) {
		return &workerp.Payload{}, x.Errorf("This server doesn't serve group id: %v", m.GroupId)
	}
	c := make(chan error, 1)
	node := groups().Node(m.GroupId)
	go func() { c <- node.ProposeAndWait(ctx, &taskp.Proposal{Mutations: m}) }()

	select {
	case <-ctx.Done():
		return &workerp.Payload{}, ctx.Err()
	case err := <-c:
		return &workerp.Payload{}, err
	}
}
