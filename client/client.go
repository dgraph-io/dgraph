/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package client

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/dgraph-io/dgraph/protos/graphp"
)

type Op int

const (
	SET Op = iota
	DEL
)

// Req wraps the graphp.Request so that we can define helper methods for the
// client around it.
type Req struct {
	gr graphp.Request
}

// Request returns the graph request object which is sent to the server to perform
// a query/mutation.
func (req *Req) Request() *graphp.Request {
	return &req.gr
}

func checkNQuad(nq graph.NQuad) error {
	if len(nq.Subject) == 0 {
		return fmt.Errorf("Subject can't be empty")
	}
	if len(nq.Predicate) == 0 {
		return fmt.Errorf("Predicate can't be empty")
	}

	hasVal := nq.ObjectValue != nil
	if len(nq.ObjectId) == 0 && !hasVal {
		return fmt.Errorf("Both objectId and objectValue can't be nil")
	}
	if len(nq.ObjectId) > 0 && hasVal {
		return fmt.Errorf("Only one out of objectId and objectValue can be set")
	}
	return nil
}

// SetQuery sets a query as part of the request.
// Example usage
// req := client.Req{}
// req.SetQuery("{ me(_xid_: alice) { name falls.in } }")
// resp, err := c.Query(context.Background(), req.Request())
// Check response and handle errors
func (req *Req) SetQuery(q string) {
	req.gr.Query = q
}

func (req *Req) addMutation(nq graph.NQuad, op Op) {
	if req.gr.Mutation == nil {
		req.gr.Mutation = new(graph.Mutation)
	}

	if op == SET {
		req.gr.Mutation.Set = append(req.gr.Mutation.Set, &nq)
	} else if op == DEL {
		req.gr.Mutation.Del = append(req.gr.Mutation.Del, &nq)
	}
}

// AddMutation adds a SET/DELETE mutation operation.
//
// Example usage
// req := client.Req{}
// To set a string value
// if err := req.AddMutation(graph.NQuad{
// 	Sub:   "alice",
// 	Pred:  "name",
// 	Value: client.Str("Alice"),
// }, client.SET); err != nil {
// ....
// handle error
// ....
// }

// To set an integer value
// if err := req.AddMutation(graph.NQuad{
// 	Sub:   "alice",
// 	Pred:  "age",
// 	Value: client.Int(13),
// }, client.SET); err != nil {
// ....
// handle error
// ....
// }

// To add a mutation with a DELETE operation
// if err := req.AddMutation(graph.NQuad{
// 	Sub:   "alice",
// 	Pred:  "name",
// 	Value: client.Str("Alice"),
// }, client.DEL); err != nil {
// ....
// handle error
// ....
// }
func (req *Req) AddMutation(nq graph.NQuad, op Op) error {
	if err := checkNQuad(nq); err != nil {
		return err
	}
	req.addMutation(nq, op)
	return nil
}

func (req *Req) size() int {
	if req.gr.Mutation == nil {
		return 0
	}
	return len(req.gr.Mutation.Set) + len(req.gr.Mutation.Del)
}

func (req *Req) reset() {
	req.gr.Query = ""
	req.gr.Mutation.Set = req.gr.Mutation.Set[:0]
	req.gr.Mutation.Del = req.gr.Mutation.Del[:0]
}

type nquadOp struct {
	nq graph.NQuad
	op Op
}

type BatchMutation struct {
	size    int
	pending int

	nquads chan nquadOp
	dc     graph.DgraphClient
	wg     sync.WaitGroup

	// Miscellaneous information to print counters.
	// Num of RDF's sent
	rdfs uint64
	// Num of mutations sent
	mutations uint64
	// To get time elapsed.
	start time.Time
}

func (batch *BatchMutation) request(req *Req) {
	counter := atomic.AddUint64(&batch.mutations, 1)
RETRY:
	_, err := batch.dc.Run(context.Background(), &req.gr)
	if err != nil {
		errString := err.Error()
		// Irrecoverable
		if strings.Contains(errString, "x509") || grpc.Code(err) == codes.Internal {
			log.Fatal(errString)
		}
		fmt.Printf("Retrying req: %d. Error: %v\n", counter, errString)
		time.Sleep(5 * time.Millisecond)
		goto RETRY
	}
	req.reset()
}

func (batch *BatchMutation) makeRequests() {
	req := new(Req)
	for n := range batch.nquads {
		req.addMutation(n.nq, n.op)
		if req.size() == batch.size {
			batch.request(req)
		}
	}
	if req.size() > 0 {
		batch.request(req)
	}
	batch.wg.Done()
}

func NewBatchMutation(ctx context.Context, conn *grpc.ClientConn,
	size int, pending int) *BatchMutation {
	bm := BatchMutation{
		size:    size,
		pending: pending,
		nquads:  make(chan nquadOp, 2*size),
		start:   time.Now(),
		dc:      graph.NewDgraphClient(conn),
	}

	for i := 0; i < pending; i++ {
		bm.wg.Add(1)
		go bm.makeRequests()
	}
	return &bm
}

func (batch *BatchMutation) AddMutation(nq graph.NQuad, op Op) error {
	if err := checkNQuad(nq); err != nil {
		return err
	}
	batch.nquads <- nquadOp{nq: nq, op: op}
	atomic.AddUint64(&batch.rdfs, 1)
	return nil
}

type Counter struct {
	Rdfs      uint64
	Mutations uint64
	Elapsed   time.Duration
}

func (batch *BatchMutation) Counter() Counter {
	return Counter{
		Rdfs:      atomic.LoadUint64(&batch.rdfs),
		Mutations: atomic.LoadUint64(&batch.mutations),
		Elapsed:   time.Since(batch.start),
	}
}

func (batch *BatchMutation) Flush() {
	close(batch.nquads)
	batch.wg.Wait()
}
