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
	"fmt"
	"log"

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

const (
	set = "set"
	del = "delete"
)

// runMutations goes through all the edges and applies them. It returns the
// mutations which were not applied in left.
func runMutations(ctx context.Context, edges []x.DirectedEdge, op byte) error {
	for _, edge := range edges {
		if !groups().ServesGroup(BelongsTo(edge.Attribute)) {
			return fmt.Errorf("Predicate fingerprint doesn't match this instance")
		}

		key := posting.Key(edge.Entity, edge.Attribute)
		plist, decr := posting.GetOrCreate(key, ws.dataStore)
		defer decr()

		if err := plist.AddMutationWithIndex(ctx, edge, op); err != nil {
			log.Printf("Error while adding mutation: %v %v", edge, err)
			return err // abort applying the rest of them.
		}
	}
	return nil
}

// mutate runs the set and delete mutations.
func mutate(ctx context.Context, m *x.Mutations) error {
	// Running the set instructions first.
	if err := runMutations(ctx, m.Set, posting.Set); err != nil {
		return err
	}
	if err := runMutations(ctx, m.Del, posting.Del); err != nil {
		return err
	}
	return nil
}

// runMutate is used to run the mutations on an instance.
func proposeOrSend(ctx context.Context, gid uint32, m *x.Mutations, che chan error) {
	data, err := m.Encode()
	if err != nil {
		che <- err
		return
	}

	if groups().ServesGroup(gid) {
		node := groups().Node(gid)
		che <- node.ProposeAndWait(ctx, mutationMsg, data)
		return
	}

	addr := groups().Leader(gid)
	pl := pools().get(addr)
	conn, err := pl.Get()
	if err != nil {
		x.TraceError(ctx, err)
		che <- err
		return
	}
	defer pl.Put(conn)
	query := new(Payload)
	query.Data = data

	c := NewWorkerClient(conn)
	_, err = c.Mutate(ctx, query)
	che <- err
}

// addToMutationArray adds the edges to the appropriate index in the mutationArray,
// taking into account the op(operation) and the attribute.
func addToMutationMap(mutationMap map[uint32]*x.Mutations, edges []x.DirectedEdge, op string) {
	for _, edge := range edges {
		gid := BelongsTo(edge.Attribute)
		mu := mutationMap[gid]
		if mu == nil {
			mu = new(x.Mutations)
			mu.GroupId = gid
			mutationMap[gid] = mu
		}

		if op == set {
			mu.Set = append(mu.Set, edge)
		} else if op == del {
			mu.Del = append(mu.Del, edge)
		}
	}
}

// MutateOverNetwork checks which group should be running the mutations
// according to fingerprint of the predicate and sends it to that instance.
func MutateOverNetwork(ctx context.Context, m x.Mutations) (rerr error) {
	mutationMap := make(map[uint32]*x.Mutations)

	addToMutationMap(mutationMap, m.Set, set)
	addToMutationMap(mutationMap, m.Del, del)

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
func (w *grpcWorker) Mutate(ctx context.Context, query *Payload) (*Payload, error) {
	if ctx.Err() != nil {
		return &Payload{}, ctx.Err()
	}

	m := new(x.Mutations)
	// Ensure that this can be decoded. This is an optional step.
	if err := m.Decode(query.Data); err != nil {
		return nil, x.Wrapf(err, "While decoding mutation.")
	}

	c := make(chan error, 1)
	node := groups().Node(m.GroupId)
	go func() { c <- node.ProposeAndWait(ctx, mutationMsg, query.Data) }()

	select {
	case <-ctx.Done():
		return &Payload{}, ctx.Err()
	case err := <-c:
		return &Payload{}, err
	}
}
