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

	"golang.org/x/net/context"

	"github.com/dgraph-io/dgraph/posting"
	"github.com/dgraph-io/dgraph/x"
	"github.com/dgryski/go-farm"
)

type Mutations struct {
	Set []x.DirectedEdge
	Del []x.DirectedEdge
}

func (m *Mutations) Encode() (data []byte, rerr error) {
	var b bytes.Buffer
	enc := gob.NewEncoder(&b)
	rerr = enc.Encode(*m)
	return b.Bytes(), rerr
}

func (m *Mutations) Decode(data []byte) error {
	r := bytes.NewReader(data)
	dec := gob.NewDecoder(r)
	return dec.Decode(m)
}

func mutate(ctx context.Context, m *Mutations, left *Mutations) error {
	// For now, assume it's all only Set instructions.
	for _, edge := range m.Set {
		if farm.Fingerprint64(
			[]byte(edge.Attribute))%numInstances != instanceIdx {

			return fmt.Errorf("predicate fingerprint doesn't match this instance.")
		}

		key := posting.Key(edge.Entity, edge.Attribute)
		plist := posting.GetOrCreate(key, dataStore)
		if err := plist.AddMutation(ctx, edge, posting.Set); err != nil {
			left.Set = append(left.Set, edge)
			log.Printf("Error while adding mutation: %v %v", edge, err)
			continue
		}
	}
	return nil
}

func runMutate(ctx context.Context, idx int, m *Mutations,
	replies chan *Payload, che chan error) {

	left := new(Mutations)
	if idx == int(instanceIdx) {
		che <- mutate(m, left)
		return
	}

	pool := pools[idx]
	var err error
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

func MutateOverNetwork(ctx context.Context,
	edges []x.DirectedEdge) (left []x.DirectedEdge, rerr error) {

	mutationArray := make([]*Mutations, numInstances)
	for _, edge := range edges {
		idx := farm.Fingerprint64([]byte(edge.Attribute)) % numInstances
		mu := mutationArray[idx]
		if mu == nil {
			mu = new(Mutations)
			mutationArray[idx] = mu
		}
		mu.Set = append(mu.Set, edge)
	}

	replies := make(chan *Payload, numInstances)
	errors := make(chan error, numInstances)
	count := 0
	for idx, mu := range mutationArray {
		if mu == nil || len(mu.Set) == 0 {
			continue
		}
		count += 1
		go runMutate(ctx, idx, mu, replies, errors)
	}

	// Wait for all the goroutines to reply back.
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

	for reply := range replies {
		l := new(Mutations)
		if err := l.Decode(reply.Data); err != nil {
			return left, err
		}
		left = append(left, l.Set...)
	}
	return left, nil
}
