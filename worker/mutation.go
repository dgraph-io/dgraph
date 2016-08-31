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

	"github.com/dgryski/go-farm"
	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/index"
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
func runMutations(ctx context.Context, edges []x.DirectedEdge, op byte, left *Mutations) error {
	for _, edge := range edges {
		if farm.Fingerprint64(
			[]byte(edge.Attribute))%numInstances != instanceIdx {
			return fmt.Errorf("Predicate fingerprint doesn't match this instance")
		}

		key := posting.Key(edge.Entity, edge.Attribute)
		plist := posting.GetOrCreate(key, dataStore)

		// Update indices (frontfill).
		if op == posting.Set && edge.Value != nil {
			index.FrontfillAdd(ctx, edge.Attribute, edge.Entity, string(edge.Value))
		} else if op == posting.Del {
			index.FrontfillDel(ctx, edge.Attribute, edge.Entity)
		}

		// Try adding the mutation.
		if err := plist.AddMutation(ctx, edge, op); err != nil {
			if op == posting.Set {
				left.Set = append(left.Set, edge)
			} else if op == posting.Del {
				left.Del = append(left.Del, edge)
			}
			log.Printf("Error while adding mutation: %v %v", edge, err)
			continue
		}
	}
	return nil
}

// mutate runs the set and delete mutations.
func mutate(ctx context.Context, m *Mutations, left *Mutations) error {
	// Running the set instructions first.
	if err := runMutations(ctx, m.Set, posting.Set, left); err != nil {
		return err
	}
	if err := runMutations(ctx, m.Del, posting.Del, left); err != nil {
		return err
	}
	return nil
}

// runMutate is used to run the mutations on an instance.
func runMutate(ctx context.Context, idx int, m *Mutations,
	replies chan *Payload, che chan error) {
	left := new(Mutations)
	var err error
	// We run them locally if idx == instanceIdx
	if idx == int(instanceIdx) {
		if err = mutate(ctx, m, left); err != nil {
			che <- err
			return
		}
		reply := new(Payload)
		// Encoding and sending back the mutations which were not applied.
		if reply.Data, err = left.Encode(); err != nil {
			che <- err
			return
		}
		replies <- reply
		che <- nil
		return
	}

	// Get a connection from the pool and run mutations over the network.
	pool := pools[idx]
	query := new(Payload)
	query.Data, err = m.Encode()
	if err != nil {
		che <- err
		return
	}

	conn, err := pool.Get()
	if err != nil {
		che <- err
		return
	}
	defer pool.Put(conn)
	c := NewWorkerClient(conn)

	reply, err := c.Mutate(ctx, query)
	if err != nil {
		che <- err
		return
	}
	replies <- reply
	che <- nil
}

// addToMutationArray adds the edges to the appropriate index in the mutationArray,
// taking into account the op(operation) and the attribute.
func addToMutationArray(mutationArray []*Mutations, edges []x.DirectedEdge, op string) {
	for _, edge := range edges {
		idx := farm.Fingerprint64([]byte(edge.Attribute)) % numInstances
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
func MutateOverNetwork(ctx context.Context, m Mutations) (left Mutations, rerr error) {
	mutationArray := make([]*Mutations, numInstances)

	addToMutationArray(mutationArray, m.Set, set)
	addToMutationArray(mutationArray, m.Del, del)

	replies := make(chan *Payload, numInstances)
	errors := make(chan error, numInstances)
	count := 0
	for idx, mu := range mutationArray {
		if mu == nil || (len(mu.Set) == 0 && len(mu.Del) == 0) {
			continue
		}
		count++
		go runMutate(ctx, idx, mu, replies, errors)
	}

	// Wait for all the goroutines to reply back.
	// We return if an error was returned or the parent called ctx.Done()
	for i := 0; i < count; i++ {
		select {
		case err := <-errors:
			if err != nil {
				x.Trace(ctx, "Error while running all mutations: %v", err)
				return left, err
			}
		case <-ctx.Done():
			return left, ctx.Err()
		}
	}
	close(replies)
	close(errors)

	// Any mutations which weren't applied are added to left which is returned.
	for reply := range replies {
		l := new(Mutations)
		if err := l.Decode(reply.Data); err != nil {
			return left, err
		}
		left.Set = append(left.Set, l.Set...)
		left.Del = append(left.Del, l.Del...)
	}
	return left, nil
}
