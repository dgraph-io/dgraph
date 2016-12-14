/*
 * Copyright 2016 DGraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package worker

import (
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/group"
	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/task"
	"github.com/dgraph-io/dgraph/x"
)

const (
	set = "set"
	del = "delete"
)

// runMutations goes through all the edges and applies them. It returns the
// mutations which were not applied in left.
func runMutations(ctx context.Context, edges []*task.DirectedEdge) error {
	for _, edge := range edges {
		if !groups().ServesGroup(group.BelongsTo(edge.Attr)) {
			return x.Errorf("Predicate fingerprint doesn't match this instance")
		}

		key := x.DataKey(edge.Attr, edge.Entity)
		plist, decr := posting.GetOrCreate(key) // TODO: Needs to have a node and entry index.
		defer decr()

		if err := plist.AddMutationWithIndex(ctx, edge); err != nil {
			x.Printf("Error while adding mutation: %v %v", edge, err)
			return err // abort applying the rest of them.
		}
	}
	return nil
}

// mutate runs the set and delete mutations.
func mutate(ctx context.Context, m *task.Mutations) error {
	// TODO: Needs to have a node and entry index.
	// Running the set instructions first.
	if err := runMutations(ctx, m.Edges); err != nil {
		return err
	}
	return nil
}

// runMutate is used to run the mutations on an instance.
func proposeOrSend(ctx context.Context, gid uint32, m *task.Mutations, che chan error) {
	if groups().ServesGroup(gid) {
		node := groups().Node(gid)
		che <- node.ProposeAndWait(ctx, &task.Proposal{Mutations: m})
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

	c := NewWorkerClient(conn)
	_, err = c.Mutate(ctx, m)
	che <- err
}

// addToMutationArray adds the edges to the appropriate index in the mutationArray,
// taking into account the op(operation) and the attribute.
func addToMutationMap(mutationMap map[uint32]*task.Mutations, edges []*task.DirectedEdge) {
	for _, edge := range edges {
		gid := group.BelongsTo(edge.Attr)
		mu := mutationMap[gid]
		if mu == nil {
			mu = &task.Mutations{GroupId: gid}
			mutationMap[gid] = mu
		}
		mu.Edges = append(mu.Edges, edge)
	}
}

// MutateOverNetwork checks which group should be running the mutations
// according to fingerprint of the predicate and sends it to that instance.
func MutateOverNetwork(ctx context.Context, m *task.Mutations) error {
	mutationMap := make(map[uint32]*task.Mutations)
	addToMutationMap(mutationMap, m.Edges)

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
func (w *grpcWorker) Mutate(ctx context.Context, m *task.Mutations) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	if !groups().ServesGroup(m.GroupId) {
		return &Payload{}, x.Errorf("This server doesn't serve group id: %v", m.GroupId)
	}
	c := make(chan error, 1)
	node := groups().Node(m.GroupId)
	go func() { c <- node.ProposeAndWait(ctx, &task.Proposal{Mutations: m}) }()

	select {
	case <-ctx.Done():
		return &Payload{}, ctx.Err()
	case err := <-c:
		return &Payload{}, err
	}
}
