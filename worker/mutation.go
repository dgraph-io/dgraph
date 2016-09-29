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
	"bytes"
	"encoding/gob"
	"fmt"
	"log"

	"context"

	"github.com/dgryski/go-farm"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
)

// Mutations stores the directed edges for both the set and delete operations.
type Mutations struct {
	Set []x.DirectedEdge
	Del []x.DirectedEdge
}

const (
	set = "set"
	del = "delete"
)

// Encode gob encodes the mutation which is then sent over to the instance which
// is supposed to run it.
func (m *Mutations) Encode() (data []byte, rerr error) {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	rerr = enc.Encode(*m)
	return b.Bytes(), rerr
}

// Decode decodes the mutation from a byte slice after receiving the byte slice over
// the network.
func (m *Mutations) Decode(data []byte) error {
	r := bytes.NewReader(data)
	dec := gob.NewDecoder(r)
	return dec.Decode(m)
}

// runMutations goes through all the edges and applies them. It returns the
// mutations which were not applied in left.
func runMutations(ctx context.Context, edges []x.DirectedEdge, op byte) error {
	for _, edge := range edges {
		if farm.Fingerprint64(
			[]byte(edge.Attribute))%ws.numGroups != ws.groupId {
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
func mutate(ctx context.Context, m *Mutations) error {
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
func proposeMutation(ctx context.Context, idx int, m *Mutations, che chan error) {
	data, err := m.Encode()
	if err != nil {
		che <- err
		return
	}

	// We run them locally if idx == groupId
	// HACK HACK HACK
	// if idx == int(ws.groupId) {
	if true {
		che <- GetNode().ProposeAndWait(ctx, mutationMsg, data)
		// che <- GetNode().raft.Propose(ctx, data)
		return
	}

	// Get a connection from the pool and run mutations over the network.
	pool := ws.GetPool(idx)
	query := new(Payload)
	query.Data = data

	conn, err := pool.Get()
	if err != nil {
		che <- err
		return
	}
	defer pool.Put(conn)
	c := NewWorkerClient(conn)

	_, err = c.Mutate(ctx, query)
	che <- err
}

// addToMutationArray adds the edges to the appropriate index in the mutationArray,
// taking into account the op(operation) and the attribute.
func addToMutationArray(mutationArray []*Mutations, edges []x.DirectedEdge, op string) {
	for _, edge := range edges {
		// TODO: Determine the right group using rules, instead of modulos.
		idx := farm.Fingerprint64([]byte(edge.Attribute)) % ws.numGroups
		mu := mutationArray[idx]
		if mu == nil {
			mu = new(Mutations)
			mutationArray[idx] = mu
		}

		if op == set {
			mu.Set = append(mu.Set, edge)
		} else if op == del {
			mu.Del = append(mu.Del, edge)
		}
	}
}

// MutateOverNetwork checks which instance should be running the mutations
// according to fingerprint of the predicate and sends it to that instance.
func MutateOverNetwork(ctx context.Context, m Mutations) (rerr error) {
	mutationArray := make([]*Mutations, ws.numGroups)

	addToMutationArray(mutationArray, m.Set, set)
	addToMutationArray(mutationArray, m.Del, del)

	errors := make(chan error, ws.numGroups)
	count := 0
	for idx, mu := range mutationArray {
		if mu == nil || (len(mu.Set) == 0 && len(mu.Del) == 0) {
			continue
		}
		count++
		go proposeMutation(ctx, idx, mu, errors)
	}

	// Wait for all the goroutines to reply back.
	// We return if an error was returned or the parent called ctx.Done()
	for i := 0; i < count; i++ {
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
